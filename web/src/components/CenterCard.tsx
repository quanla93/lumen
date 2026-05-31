import type { ReactNode } from "react";
import { LogoMark } from "@/components/Logo";
import { AppButton } from "@/components/ui";

/** Centered single-column container used by Login + Register pages. */
export function CenterCard({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
}) {
  return (
    <main className="min-h-screen flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-sm rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-6 shadow-sm">
        <div className="mb-4 flex items-center justify-center text-[color:var(--lumen-teal)]">
          <LogoMark size={44} />
        </div>
        <h1 className="text-xl font-semibold tracking-tight text-center">{title}</h1>
        {subtitle && (
          <p className="mt-1 text-sm text-[color:var(--color-muted)] text-center">{subtitle}</p>
        )}
        <div className="mt-5">{children}</div>
      </div>
    </main>
  );
}

/** Small reusable input styled like the rest of the dashboard. */
export function FieldInput(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={`w-full rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[color:var(--color-accent)] ${props.className ?? ""}`}
    />
  );
}

export function Field({
  label,
  children,
  className = "",
}: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <label className={`block ${className}`}>
      <span className="block text-xs uppercase tracking-wide text-[color:var(--color-muted)] mb-1">
        {label}
      </span>
      {children}
    </label>
  );
}

/** Primary action button. */
export function PrimaryButton(props: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return <AppButton variant="primary" type="submit" {...props} />;
}

/** Secondary / ghost button. */
export function GhostButton(props: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return <AppButton variant="secondary" {...props} className={`py-1.5 ${props.className ?? ""}`} />;
}

/** Inline error banner. */
export function ErrorText({ message }: { message: string }) {
  return (
    <p className="text-sm text-[color:var(--color-danger)]" role="alert">
      {message}
    </p>
  );
}
