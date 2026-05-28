import { useState } from "react";
import { authApi, ApiError, type User } from "@/lib/api";
import { CenterCard, Field, FieldInput, PrimaryButton, ErrorText } from "@/components/CenterCard";
import { useI18n } from "@/i18n/useI18n";

export function RegisterForm({ onSuccess }: { onSuccess: (user: User) => void }) {
  const { t } = useI18n();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (password !== confirm) {
      setError(t("auth.passwordsMismatch"));
      return;
    }
    setBusy(true);
    try {
      const u = await authApi.register(username, password);
      onSuccess(u);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <CenterCard
      title={t("auth.createAdminTitle")}
      subtitle={t("auth.createAdminSubtitle")}
    >
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("auth.username")}>
          <FieldInput
            type="text"
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            minLength={3}
            maxLength={32}
            pattern="[A-Za-z0-9_.\-]+"
            required
            autoFocus
          />
        </Field>
        <Field label={t("auth.passwordMin")}>
          <FieldInput
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            minLength={8}
            required
          />
        </Field>
        <Field label={t("auth.confirmPassword")}>
          <FieldInput
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            minLength={8}
            required
          />
        </Field>
        {error && <ErrorText message={error} />}
        <PrimaryButton disabled={busy} className="w-full">
          {busy ? t("auth.creating") : t("auth.createAdminAndSignIn")}
        </PrimaryButton>
      </form>
    </CenterCard>
  );
}
