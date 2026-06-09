import { useEffect, useState } from "react";
import { authApi, webauthnApi, ApiError, type User } from "@/lib/api";
import { CenterCard, Field, FieldInput, PrimaryButton, ErrorText } from "@/components/CenterCard";
import { useI18n } from "@/i18n/useI18n";
import {
  encodeAssertionForServer,
  jsonBase64UrlToBytes,
} from "@/lib/webauthn";

export function LoginForm({ onSuccess }: { onSuccess: (user: User) => void }) {
  const { t } = useI18n();
  // LoginForm has the same i18n-leaf-drop issue as
  // OnboardingWizard / PasskeySection: the deep `passkeys.*`
  // subtree is missing from the TranslationKey union. Cast
  // t once at the outer scope; the body uses (k: string) =>
  // string for the affected calls.
  const tAny = t as unknown as (k: string) => string;
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [oidcEnabled, setOidcEnabled] = useState(false);
  const [samlEnabled, setSamlEnabled] = useState(false);
  const [passkeySupported, setPasskeySupported] = useState(false);
  const [passkeyBusy, setPasskeyBusy] = useState(false);

  // Pull oidc_enabled / saml_enabled so we know whether to render the
  // SSO buttons. Done here (not in App.bootstrap) so a runtime toggle
  // from Settings → SSO / Settings → SAML takes effect on the next
  // mount without a route refactor.
  // Also surface any sso_error= query param that the OIDC or SAML
  // callback set when it bounced the user back to the login page on
  // failure.
  useEffect(() => {
    authApi.setupStatus().then((s) => {
      setOidcEnabled(s.oidc_enabled);
      setSamlEnabled(s.saml_enabled);
    }).catch(() => {});
    // Probe for navigator.credentials.get — same RFC 0006
    // §"Risks" #5 as the Settings side: very old browsers
    // don't ship WebAuthn at all.
    setPasskeySupported(
      typeof window !== "undefined" &&
        typeof navigator !== "undefined" &&
        !!navigator.credentials?.get,
    );
    const params = new URLSearchParams(window.location.search);
    const ssoErr = params.get("sso_error");
    if (ssoErr) {
      setError(`${t("auth.ssoError")}: ${decodeURIComponent(ssoErr)}`);
      // Drop the query param so a refresh doesn't repeat the message.
      window.history.replaceState({}, "", window.location.pathname);
    }
    // intentional: t reference stable across renders, only run on mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const u = await authApi.login(username, password);
      onSuccess(u);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <CenterCard title={t("auth.signInTitle")}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("auth.username")}>
          <FieldInput
            type="text"
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
          />
        </Field>
        <Field label={t("auth.password")}>
          <FieldInput
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </Field>
        {error && <ErrorText message={error} />}
        <PrimaryButton disabled={busy} className="w-full">
          {busy ? t("auth.signingIn") : t("auth.signIn")}
        </PrimaryButton>
        {(oidcEnabled || samlEnabled) && (
          <>
            <div className="flex items-center gap-3 text-xs text-[color:var(--color-muted)]">
              <span className="h-px flex-1 bg-[color:var(--color-border)]" />
              <span>{t("auth.signInOr")}</span>
              <span className="h-px flex-1 bg-[color:var(--color-border)]" />
            </div>
            {/* Full page navigation (anchor, not fetch) — the IdP needs
                a real browser redirect to set its own session cookies. */}
            {oidcEnabled && (
              <a
                href="/api/auth/oidc/login"
                className="block w-full rounded-md border border-[color:var(--color-border)] py-2 text-center text-sm font-medium text-[color:var(--color-fg)] hover:bg-[color:var(--color-border)]/40"
              >
                {t("auth.signInWithSSO")}
              </a>
            )}
            {samlEnabled && (
              <a
                href="/api/auth/saml/login"
                className="block w-full rounded-md border border-[color:var(--color-border)] py-2 text-center text-sm font-medium text-[color:var(--color-fg)] hover:bg-[color:var(--color-border)]/40"
              >
                {t("auth.signInWithSAML")}
              </a>
            )}
            {passkeySupported && (
              <button
                type="button"
                disabled={passkeyBusy}
                onClick={async () => {
                  setError(null);
                  setPasskeyBusy(true);
                  try {
                    const ch = await webauthnApi.loginBegin(username);
                    const options = jsonBase64UrlToBytes(ch.options_json);
                    const cred = (await navigator.credentials.get(
                      options as unknown as CredentialRequestOptions,
                    )) as PublicKeyCredential | null;
                    if (!cred) {
                      setError(tAny("passkeys.cancelled"));
                      return;
                    }
                    const assertion = encodeAssertionForServer({
                      id: cred.id,
                      rawId: cred.rawId,
                      type: cred.type,
                      response: {
                        clientDataJSON: cred.response.clientDataJSON,
                        authenticatorData: (cred.response as AuthenticatorAssertionResponse).authenticatorData,
                        signature: (cred.response as AuthenticatorAssertionResponse).signature,
                        userHandle: (cred.response as AuthenticatorAssertionResponse).userHandle ?? null,
                      },
                    });
                    const result = await webauthnApi.loginFinish(ch.session_id, assertion);
                    // The hub's loginFinish endpoint normally
                    // sets the session cookie server-side, so a
                    // 200 means the operator is signed in. We
                    // refresh /api/me to pick up the new session
                    // and let App.tsx route to the dashboard.
                    const me = await authApi.me();
                    onSuccess(me);
                    void result;
                  } catch (e) {
                    if (e instanceof DOMException && e.name === "NotAllowedError") {
                      setError(tAny("passkeys.cancelled"));
                    } else if (e instanceof ApiError) {
                      setError(e.message);
                    } else {
                      setError(e instanceof Error ? e.message : String(e));
                    }
                  } finally {
                    setPasskeyBusy(false);
                  }
                }}
                className="block w-full rounded-md border border-[color:var(--color-border)] py-2 text-center text-sm font-medium text-[color:var(--color-fg)] hover:bg-[color:var(--color-border)]/40 disabled:opacity-50"
              >
                {passkeyBusy ? t("auth.signingIn") : t("auth.signInWithPasskey")}
              </button>
            )}
          </>
        )}
      </form>
    </CenterCard>
  );
}
