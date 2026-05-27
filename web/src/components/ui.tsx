import type { ButtonHTMLAttributes, ComponentPropsWithoutRef, ReactNode } from "react";
import { TONE_CLASS, type StatusTone } from "@/lib/status";

type SurfaceProps = ComponentPropsWithoutRef<"div"> & {
  as?: "div" | "section";
  padded?: boolean;
};

export function Surface({ as = "div", padded = true, className = "", children, ...props }: SurfaceProps) {
  const Tag = as;
  return (
    <Tag
      {...props}
      className={`rounded-2xl border border-[color:var(--color-border)] bg-[color:var(--color-card)] shadow-sm ${padded ? "p-5" : ""} ${className}`}
    >
      {children}
    </Tag>
  );
}

export function EmptyState({ title, detail, className = "" }: { title: string; detail?: ReactNode; className?: string }) {
  return (
    <Surface padded={false} className={`border-dashed p-10 text-center ${className}`}>
      <p className="text-lg font-medium">{title}</p>
      {detail && <p className="mx-auto mt-2 max-w-md text-sm text-[color:var(--color-muted)]">{detail}</p>}
    </Surface>
  );
}

export function StatusPill({ tone, children }: { tone: StatusTone; children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-1 text-xs text-[color:var(--color-muted)]">
      <span aria-hidden className={`inline-block h-2 w-2 rounded-full ${TONE_CLASS[tone]}`} />
      {children}
    </span>
  );
}

export function AppButton({ variant = "secondary", className = "", ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "ghost" | "danger" }) {
  const base = "inline-flex items-center justify-center rounded-md px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)]";
  const styles = {
    primary: "bg-[color:var(--color-fg)] text-[color:var(--color-bg)] hover:opacity-90",
    secondary: "border border-[color:var(--color-border)] bg-[color:var(--color-card)] hover:bg-[color:var(--color-border)]",
    ghost: "text-[color:var(--color-muted)] hover:bg-[color:var(--color-border)] hover:text-[color:var(--color-fg)]",
    danger: "border border-[color:var(--color-danger)] text-[color:var(--color-danger)] hover:bg-[color:var(--color-danger)] hover:text-[color:var(--color-bg)]",
  };
  return <button type="button" {...props} className={`${base} ${styles[variant]} ${className}`} />;
}
