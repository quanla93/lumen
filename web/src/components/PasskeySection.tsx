// PasskeySection.tsx — Sprint 6 / RFC 0006 Settings → Account
// subsection.
//
// Renders the operator's list of passkeys with a "Add passkey"
// button. The flow uses navigator.credentials.create() to mint a
// new credential; the hub server stores it.
//
// The "Add passkey" button is hidden when navigator.credentials
// is unavailable (very old browser) — RFC 0006 §"Risks" #5.
//
// Delete refuses via server-side guard (refuses to remove the
// last passkey if the user has no password). On 409 we surface
// the server's localized error.

import { useEffect, useState } from "react";
import { webauthnApi, type Passkey } from "@/lib/api";
import { ApiError } from "@/lib/api";
import {
  encodeAttestationForServer,
  jsonBase64UrlToBytes,
} from "@/lib/webauthn";
import { PrimaryButton, GhostButton, ErrorText, Field, FieldInput } from "@/components/CenterCard";
import { useI18n } from "@/i18n/useI18n";

// The `passkeys.*` subtree is a deep-nested extension to
// `settings.*`; the LeafKeys walk in src/i18n/types.ts drops
// it from the TranslationKey union at the recursion-depth
// limit. We cast t at the component's outer scope so the
// body uses (k: string) => string. Same workaround as
// OnboardingWizard — see Sprint 5 memory.
type TFunc = (k: string, params?: Record<string, string | number>) => string;

export function PasskeySection() {
  const { t } = useI18n();
  const tt = t as unknown as TFunc;
  const [creds, setCreds] = useState<Passkey[] | null>(null);
  const [supported, setSupported] = useState<boolean | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [showAdd, setShowAdd] = useState(false);

  useEffect(() => {
    // navigator.credentials may be undefined in private windows
    // or very old browsers. Probe once on mount.
    setSupported(
      typeof window !== "undefined" &&
        typeof navigator !== "undefined" &&
        !!navigator.credentials?.create,
    );
    refresh();
  }, []);

  async function refresh() {
    try {
      const { credentials } = await webauthnApi.list();
      setCreds(credentials);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }

  async function add(label: string) {
    setBusy(true);
    setErr(null);
    try {
      const ch = await webauthnApi.registerBegin(label);
      const options = jsonBase64UrlToBytes(ch.options_json);
      // Cast through unknown — the browser-typed options object
      // matches our decoded shape but tsc doesn't see it.
      const cred = (await navigator.credentials.create(
        options as unknown as CredentialCreationOptions,
      )) as PublicKeyCredential | null;
      if (!cred) {
        setErr(tt("passkeys.cancelled"));
        return;
      }
      // cred.response is the base AuthenticatorResponse. The
      // .create() call always returns AuthenticatorAttestationResponse
      // (the .attestationObject field lives only on that subtype),
      // so we cast through unknown to read it.
      const att = cred.response as unknown as AuthenticatorAttestationResponse;
      const attestation = encodeAttestationForServer({
        id: cred.id,
        rawId: cred.rawId,
        type: cred.type,
        response: {
          clientDataJSON: att.clientDataJSON,
          attestationObject: att.attestationObject,
        },
      });
      await webauthnApi.registerFinish(ch.session_id, attestation);
      await refresh();
      setShowAdd(false);
    } catch (e) {
      // NotAllowedError = operator dismissed the browser prompt.
      if (e instanceof DOMException && e.name === "NotAllowedError") {
        setErr(tt("passkeys.cancelled"));
      } else if (e instanceof ApiError) {
        setErr(e.message);
      } else {
        setErr(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setBusy(false);
    }
  }

  async function del(id: number) {
    if (!window.confirm(tt("passkeys.deleteConfirm"))) return;
    setBusy(true);
    setErr(null);
    try {
      await webauthnApi.delete(id);
      await refresh();
    } catch (e) {
      if (e instanceof ApiError) {
        setErr(e.message);
      } else {
        setErr(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setBusy(false);
    }
  }

  if (supported === false) {
    return (
      <section>
        <h3 className="text-sm font-semibold tracking-tight mb-2">{tt("passkeys.title")}</h3>
        <p className="text-xs text-[color:var(--color-muted)]">{tt("passkeys.unsupported")}</p>
      </section>
    );
  }

  return (
    <section>
      <h3 className="text-sm font-semibold tracking-tight mb-2">{tt("passkeys.title")}</h3>
      <p className="text-xs text-[color:var(--color-muted)] mb-3">{tt("passkeys.description")}</p>

      {err && <ErrorText message={err} />}

      {creds === null ? (
        <p className="text-xs text-[color:var(--color-muted)]">{tt("passkeys.loading")}</p>
      ) : creds.length === 0 ? (
        <p className="text-xs text-[color:var(--color-muted)]">{tt("passkeys.empty")}</p>
      ) : (
        <ul className="divide-y divide-[color:var(--color-border)] rounded-md border border-[color:var(--color-border)] mb-3">
          {creds.map((c) => (
            <li key={c.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
              <div>
                <p className="font-medium">{c.label || tt("passkeys.unlabeled")}</p>
                <p className="text-[11px] text-[color:var(--color-muted)]">
                  {tt("passkeys.signCount", { n: c.sign_count })}{" "}
                  {c.last_used_at ? "· " + tt("passkeys.lastUsed", { ts: c.last_used_at }) : ""}
                </p>
              </div>
              <GhostButton onClick={() => del(c.id)} disabled={busy}>
                {t("common.delete")}
              </GhostButton>
            </li>
          ))}
        </ul>
      )}

      {showAdd ? (
        <AddForm
          busy={busy}
          onSubmit={add}
          onCancel={() => {
            setShowAdd(false);
            setErr(null);
          }}
        />
      ) : (
        <PrimaryButton onClick={() => setShowAdd(true)} disabled={busy}>
          {tt("passkeys.add")}
        </PrimaryButton>
      )}
    </section>
  );
}

function AddForm({
  busy,
  onSubmit,
  onCancel,
}: {
  busy: boolean;
  onSubmit: (label: string) => void;
  onCancel: () => void;
}) {
  const { t } = useI18n();
  const tt = t as unknown as TFunc;
  const [label, setLabel] = useState("");
  return (
    <div className="space-y-3 rounded-md border border-[color:var(--color-border)] p-3">
      <Field label={tt("passkeys.labelLabel")}>
        <FieldInput
          type="text"
          autoFocus
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          placeholder={tt("passkeys.labelPlaceholder")}
        />
      </Field>
      <div className="flex items-center justify-end gap-2">
        <GhostButton onClick={onCancel}>{t("common.cancel")}</GhostButton>
        <PrimaryButton onClick={() => onSubmit(label.trim())} disabled={busy || !label.trim()}>
          {tt("passkeys.create")}
        </PrimaryButton>
      </div>
    </div>
  );
}
