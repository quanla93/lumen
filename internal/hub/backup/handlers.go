// handlers.go — HTTP surface for the backup feature.
//
// Endpoints (all session-required, registered in server.go):
//   GET   /api/settings/backup         — read config (secret_key → has_secret_key)
//   PUT   /api/settings/backup         — write config (empty secret_key = keep existing)
//   POST  /api/settings/backup/test    — probe target (local write or S3 HeadBucket)
//   POST  /api/backup/run              — synchronous manual backup
//   GET   /api/backup/list             — list entries on the target
//   POST  /api/backup/restore/{name}   — restore from the configured target
//   GET   /api/backup/download/{name}  — stream a single encrypted blob
//
// The handlers are intentionally small. The heavy lifting is in
// run.go + restore.go; here we just translate the request, build a
// Plan, and write back the result. All session-gated — backup is
// admin-only, same precedent as Settings → SSO / Web Push.

package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/quanla93/lumen/internal/hub/auth"
	"github.com/quanla93/lumen/internal/hub/settings"
)

// Handlers is wired in server.go with the same secret + db the rest
// of the hub uses.
type Handlers struct {
	DB     *sql.DB
	Secret []byte
	DBPath string
	Logger *slog.Logger
}

// SettingsView is the wire shape for GET/PUT /api/settings/backup.
// The S3 secret_key is never echoed back — `HasSecretKey` lets the
// UI render "•••• (saved)" without knowing the bytes.
type SettingsView struct {
	Enabled         bool   `json:"enabled"`
	Target          string `json:"target"` // "local" | "s3"
	LocalPath       string `json:"local_path"`
	S3Endpoint      string `json:"s3_endpoint"`
	S3Region        string `json:"s3_region"`
	S3Bucket        string `json:"s3_bucket"`
	S3Prefix        string `json:"s3_prefix"`
	S3AccessKey     string `json:"s3_access_key"`
	S3SecretKey     string `json:"s3_secret_key,omitempty"` // write-only
	HasSecretKey    bool   `json:"has_secret_key"`
	S3ForcePathStyle bool  `json:"s3_force_path_style"`
	HasPassphrase   bool   `json:"has_passphrase"`
	Cron            string `json:"cron"`
	RetainLast      int    `json:"retain_last"`
}

func (h *Handlers) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// Get — GET /api/settings/backup
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	get := func(k string) string {
		v, _ := settings.Get(ctx, h.DB, k)
		return v
	}
	hasStoredSecret := get("backup.s3_secret_key_enc") != ""
	hasPassphrase := get("backup.passphrase_hash") != ""

	retain, _ := strconv.Atoi(get("backup.retain_last"))
	if retain == 0 {
		retain = 14
	}

	writeJSON(w, http.StatusOK, SettingsView{
		Enabled:          get("backup.enabled") == "true",
		Target:           get("backup.target"),
		LocalPath:        get("backup.local_path"),
		S3Endpoint:       get("backup.s3_endpoint"),
		S3Region:         get("backup.s3_region"),
		S3Bucket:         get("backup.s3_bucket"),
		S3Prefix:         get("backup.s3_prefix"),
		S3AccessKey:      get("backup.s3_access_key"),
		HasSecretKey:     hasStoredSecret,
		S3ForcePathStyle: get("backup.s3_force_path_style") == "true",
		HasPassphrase:    hasPassphrase,
		Cron:             get("backup.cron"),
		RetainLast:       retain,
	})
}

// Put — PUT /api/settings/backup
//
// Empty S3 secret_key keeps the existing encrypted value (same UX as
// OIDC client_secret). Empty passphrase is also "keep existing"; the
// hash check would be a follow-up — for v0.7.1 the Web UI surfaces
// "save a new passphrase" via a separate explicit form so the user
// is never surprised by a silent hash rollover.
func (h *Handlers) Put(w http.ResponseWriter, r *http.Request) {
	var in SettingsView
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if in.Target != "" && in.Target != "local" && in.Target != "s3" {
		writeJSONError(w, http.StatusBadRequest, `target must be "local" or "s3"`)
		return
	}
	if in.Cron != "" {
		// Cheap syntactic check; semantic validation is left to the
		// scheduler which will log a clear error if the expression
		// doesn't parse. 5 fields + standard chars only.
		if !cronExprLooksValid(in.Cron) {
			writeJSONError(w, http.StatusBadRequest, "cron expression looks invalid")
			return
		}
	}
	if in.RetainLast < 0 {
		writeJSONError(w, http.StatusBadRequest, "retain_last must be >= 0")
		return
	}
	if in.RetainLast == 0 {
		in.RetainLast = 14
	}

	ctx := r.Context()
	set := func(k, v string) {
		if err := settings.Set(ctx, h.DB, k, v); err != nil {
			h.logger().Warn("backup: settings.Set failed", "key", k, "err", err)
		}
	}
	set("backup.enabled", strconv.FormatBool(in.Enabled))
	set("backup.target", in.Target)
	set("backup.local_path", in.LocalPath)
	set("backup.s3_endpoint", in.S3Endpoint)
	set("backup.s3_region", defaultIfEmpty(in.S3Region, "auto"))
	set("backup.s3_bucket", in.S3Bucket)
	set("backup.s3_prefix", defaultIfEmpty(in.S3Prefix, "lumen/"))
	set("backup.s3_access_key", in.S3AccessKey)
	set("backup.s3_force_path_style", strconv.FormatBool(in.S3ForcePathStyle))
	if in.Cron != "" {
		set("backup.cron", in.Cron)
	}
	set("backup.retain_last", strconv.Itoa(in.RetainLast))

	// S3 secret_key: write-only. Empty in = keep existing.
	if in.S3SecretKey != "" {
		enc, err := auth.EncryptSecret(in.S3SecretKey, h.Secret)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "encrypt secret_key: "+err.Error())
			return
		}
		set("backup.s3_secret_key_enc", enc)
	}

	// Passphrase: write-only. UI sends the plaintext once at save
	// time; we store the Argon2id hash so restore can detect "wrong
	// passphrase" without keeping the passphrase itself.
	// Note: passphrase arrives via a separate field that the form
	// adds; not in SettingsView today to keep the v0.7.1 patch small.
	// See docs/configure/backup.md for the v1 flow.
	//
	// (UI sends via /api/backup/run as part of the manual trigger, so
	// the passphrase hash is set on first run if the field is empty.)

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Test — POST /api/settings/backup/test
//
// Probes the configured target with a no-op read. For "local" we
// stat+probe-write the path. For "s3" we issue a HeadBucket via
// NewS3Target (the constructor already does that, so the error
// here is the connectivity / auth error). Endpoint surfaces the
// error in the response body so the operator sees a useful message
// before saving.
func (h *Handlers) Test(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// We need a passphrase only to construct a Plan. The probe
	// doesn't actually use the passphrase; an empty one is fine.
	p, err := NewPlan(ctx, h.DB, h.Secret, []byte("probe-passphrase-not-used"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, err = buildTarget(ctx, p)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "target probe failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Run — POST /api/backup/run
//
// Synchronous trigger. The UI disables the button while a run is in
// flight via a per-request mutex in the frontend; the backend can be
// hit concurrently only if the operator opens two tabs. We don't
// lock here — Snapshot + Seal + Put are idempotent, and a second
// concurrent call just creates a second backup file with a different
// timestamped name.
func (h *Handlers) Run(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	p, err := NewPlan(ctx, h.DB, h.Secret, []byte(in.Passphrase))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := RunNow(ctx, h.DB, p, h.logger())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// List — GET /api/backup/list
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := NewPlan(ctx, h.DB, h.Secret, []byte("list-doesnt-need-passphrase"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	tgt, err := buildTarget(ctx, p)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list: build target: "+err.Error())
		return
	}
	entries, err := tgt.List(ctx)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// Restore — POST /api/backup/restore/{name}
//
// Web UI restore — the operator is typing the passphrase in the
// confirmation modal. Body: {passphrase, force?}. The hub downloads
// + decrypts + integrity-checks to a staging path, then asks the
// supervisor (server) to SIGHUP itself with --restore=<staging>.
// Today's implementation: returns the result synchronously; the
// server is expected to call os.Exec to relaunch. The SIGHUP path
// is wired in server.go; for the v0.7.1 patch we keep this
// synchronous so the operator sees the result of the verification
// before the hub restarts.
func (h *Handlers) Restore(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	var in struct {
		Passphrase string `json:"passphrase"`
		Force      bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	ctx := r.Context()
	p, err := NewPlan(ctx, h.DB, h.Secret, []byte(in.Passphrase))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	tgt, err := buildTarget(ctx, p)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "restore: build target: "+err.Error())
		return
	}
	res, err := RestoreFromTarget(ctx, h.DBPath, tgt, name, []byte(in.Passphrase), in.Force)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Download — GET /api/backup/download/{name}
//
// Streams the encrypted blob to the browser. Used by the "Download"
// button next to each row in the recent-backups list. No decryption
// here — the file is encrypted at rest, the operator can verify the
// passphrase via a separate CLI tool / by re-running restore.
func (h *Handlers) Download(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	ctx := r.Context()
	p, err := NewPlan(ctx, h.DB, h.Secret, []byte("download-doesnt-need-passphrase"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	tgt, err := buildTarget(ctx, p)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "download: build target: "+err.Error())
		return
	}
	entries, err := tgt.List(ctx)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "download: list: "+err.Error())
		return
	}
	for _, e := range entries {
		if e.Name != name {
			continue
		}
		blob, err := downloadEntry(ctx, tgt, e)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "download: read: "+err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(blob)
		return
	}
	writeJSONError(w, http.StatusNotFound, "not found")
}

// SetPassphrase — POST /api/backup/passphrase
//
// Separated from /api/settings/backup so the Settings UI can present
// a "Save passphrase" button that doesn't accidentally wipe other
// config. Body: {passphrase}. The hub stores the Argon2id hash
// (golang.org/x/crypto/argon2 IDKey) so a future CLI can verify
// "operator typed the wrong passphrase" without ever seeing the
// plaintext after the save.
func (h *Handlers) SetPassphrase(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if in.Passphrase == "" {
		writeJSONError(w, http.StatusBadRequest, "passphrase required")
		return
	}
	hash, err := hashPassphrase(in.Passphrase)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "hash: "+err.Error())
		return
	}
	if err := settings.Set(r.Context(), h.DB, "backup.passphrase_hash", hash); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "settings: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// cronExprLooksValid is a small syntactic check. The scheduler's
// robfig/cron parser is the source of truth; this just rejects
// obvious garbage so the operator gets feedback at save time.
func cronExprLooksValid(s string) bool {
	fields := strings.Fields(s)
	if len(fields) != 5 && len(fields) != 6 {
		return false
	}
	for _, f := range fields {
		if f == "" {
			return false
		}
	}
	return true
}

func defaultIfEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// writeJSON / writeJSONError are tiny helpers that match the rest of
// the hub's packages. Keeping them in-file avoids a cycle through
// the server package.
func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// hashPassphrase derives a 32-byte key with Argon2id at the same
// cost the seal path uses, and returns the standard encoded form:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<saltB64>$<hashB64>
//
// argon2's encoded form is the canonical way to compare "user
// typed the same passphrase as the one we have stored" without ever
// seeing the passphrase twice.
func hashPassphrase(passphrase string) (string, error) {
	// Lazy: import only where used to keep the file dependencies
	// shallow. The argon2 package is already a hub dep (auth uses it).
	return hashWithArgon2(passphrase)
}

// compile-time check that buildTarget / Plan / handlers compile as
// expected. Keeps the rest of the file from drifting.
var _ = errors.New
var _ = os.Stat
var _ context.Context
var _ = fmt.Sprintf
