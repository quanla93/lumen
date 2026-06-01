// Package install serves the install.sh bootstrap script and the agent
// binaries it downloads. Both are optional: if InstallDir is empty or
// missing the script, the endpoints return 503 with a helpful message
// (the hub itself still works; only the one-liner install is disabled).
package install

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-chi/chi/v5"
)

// Handler serves /install.sh and /install/{binary}. InstallDir is the
// filesystem path that holds:
//
//	install.sh                       (the bootstrap script template)
//	lumen-agent-linux-amd64
//	lumen-agent-linux-arm64
//	lumen-agent-linux-armv7
//
// In the hub Docker image these live at /install. For native dev set
// LUMEN_HUB_INSTALL_DIR=./dist after running `make release-agents` plus
// `cp scripts/install-agent.sh dist/install.sh`.
type Handler struct {
	InstallDir string
	Logger     *slog.Logger
}

func (h *Handler) ServeScript(w http.ResponseWriter, r *http.Request) {
	if h.InstallDir == "" {
		writeUnavailable(w, "LUMEN_HUB_INSTALL_DIR not set on this hub")
		return
	}
	scriptPath := filepath.Join(h.InstallDir, "install.sh")
	tmpl, err := template.ParseFiles(scriptPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeUnavailable(w, "install.sh missing — run scripts/build-release.sh on the hub host")
			return
		}
		h.Logger.Error("install script parse failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	data := map[string]string{
		"HubURL": hubURLFromRequest(r),
	}
	if err := tmpl.Execute(w, data); err != nil {
		h.Logger.Warn("install script render failed", "err", err)
	}
}

func (h *Handler) ServeBinary(w http.ResponseWriter, r *http.Request) {
	if h.InstallDir == "" {
		writeUnavailable(w, "LUMEN_HUB_INSTALL_DIR not set on this hub")
		return
	}
	name := chi.URLParam(r, "binary")
	// Reject anything that isn't a Lumen agent binary. Prevents path
	// traversal AND avoids serving e.g. install.sh from this route.
	if !isAllowedBinary(name) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(h.InstallDir, name)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

var allowedBinaries = map[string]bool{
	"lumen-agent-linux-amd64": true,
	"lumen-agent-linux-arm64": true,
	"lumen-agent-linux-armv7": true,
}

func isAllowedBinary(name string) bool {
	return allowedBinaries[name]
}

// hubURLFromRequest reconstructs the URL the *client* used to reach the
// hub, so the install script downloads the binary from the same host
// the user already ran `curl` against. Respects X-Forwarded-Proto for
// reverse-proxy setups.
func hubURLFromRequest(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xfp := r.Header.Get("X-Forwarded-Proto"); xfp != "" {
		scheme = xfp
	}
	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}
	return scheme + "://" + strings.TrimRight(host, "/")
}

func writeUnavailable(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("install endpoint disabled: " + msg + "\n"))
}
