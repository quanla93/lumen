import { useState } from "react";
import { authApi, ApiError, type User } from "@/lib/api";
import { CenterCard, Field, FieldInput, PrimaryButton, ErrorText } from "@/components/CenterCard";

export function LoginForm({ onSuccess }: { onSuccess: (user: User) => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

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
    <CenterCard title="Sign in to Lumen">
      <form onSubmit={submit} className="space-y-4">
        <Field label="Username">
          <FieldInput
            type="text"
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
          />
        </Field>
        <Field label="Password">
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
          {busy ? "Signing in…" : "Sign in"}
        </PrimaryButton>
      </form>
    </CenterCard>
  );
}
