import { useEffect, useState } from "react";
import { authApi, ApiError, type User } from "@/lib/api";
import { CenterCard, Field, FieldInput, PrimaryButton, ErrorText } from "@/components/CenterCard";
import { useI18n } from "@/i18n/useI18n";

export function LoginForm({ onSuccess }: { onSuccess: (user: User) => void }) {
  const { t } = useI18n();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [oidcEnabled, setOidcEnabled] = useState(false);

  // Pull oidc_enabled so we know whether to render the SSO button. Done
  // here (not in App.bootstrap) so a runtime toggle from Settings → SSO
  // takes effect on the next mount without a route refactor.
  // Also surface any sso_error= query param that the OIDC callback set
  // when it bounced the user back to the login page on failure.
  useEffect(() => {
    authApi.setupStatus().then((s) => setOidcEnabled(s.oidc_enabled)).catch(() => {});
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
        {oidcEnabled && (
          <>
            <div className="flex items-center gap-3 text-xs text-[color:var(--color-muted)]">
              <span className="h-px flex-1 bg-[color:var(--color-border)]" />
              <span>{t("auth.signInOr")}</span>
              <span className="h-px flex-1 bg-[color:var(--color-border)]" />
            </div>
            {/* Full page navigation (anchor, not fetch) — the IdP needs
                a real browser redirect to set its own session cookies. */}
            <a
              href="/api/auth/oidc/login"
              className="block w-full rounded-md border border-[color:var(--color-border)] py-2 text-center text-sm font-medium text-[color:var(--color-fg)] hover:bg-[color:var(--color-border)]/40"
            >
              {t("auth.signInWithSSO")}
            </a>
          </>
        )}
      </form>
    </CenterCard>
  );
}
