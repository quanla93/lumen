import {
  forwardRef,
  type ButtonHTMLAttributes,
  type ComponentPropsWithoutRef,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";
import * as RadixPopover from "@radix-ui/react-popover";
import * as RadixTooltip from "@radix-ui/react-tooltip";
import * as RadixSwitch from "@radix-ui/react-switch";
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
      className={`rounded-2xl border border-[color:var(--color-border)] bg-[color:var(--color-card)] shadow-[var(--shadow-1)] ${padded ? "p-5" : ""} ${className}`}
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

type AppButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
};

export const AppButton = forwardRef<HTMLButtonElement, AppButtonProps>(
  function AppButton({ variant = "secondary", className = "", ...props }, ref) {
    const base = "inline-flex items-center justify-center rounded-md px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)]";
    const styles = {
      primary: "bg-[color:var(--color-fg)] text-[color:var(--color-bg)] hover:opacity-90",
      secondary: "border border-[color:var(--color-border)] bg-[color:var(--color-card)] hover:bg-[color:var(--color-border)]",
      ghost: "text-[color:var(--color-muted)] hover:bg-[color:var(--color-border)] hover:text-[color:var(--color-fg)]",
      danger: "border border-[color:var(--color-danger)] text-[color:var(--color-danger)] hover:bg-[color:var(--color-danger)] hover:text-[color:var(--color-bg)]",
    };
    return <button ref={ref} type="button" {...props} className={`${base} ${styles[variant]} ${className}`} />;
  },
);

/* ------------------------------------------------------------------
 * PR1 primitives — additive. AppButton stays as-is; new callsites use
 * these. All hit areas ≥40×40, focus rings via accent token.
 * ------------------------------------------------------------------ */

type IconButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & { label: string };

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  function IconButton({ label, className = "", children, ...props }, ref) {
    return (
      <button
        ref={ref}
        type="button"
        aria-label={label}
        title={label}
        {...props}
        className={`inline-flex h-10 w-10 items-center justify-center rounded-md text-[color:var(--color-muted)] transition-colors hover:bg-[color:var(--color-border)] hover:text-[color:var(--color-fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)] disabled:opacity-50 ${className}`}
      >
        {children}
      </button>
    );
  },
);

export function Chip({
  tone = "muted",
  className = "",
  children,
}: {
  tone?: StatusTone;
  className?: string;
  children: ReactNode;
}) {
  const toneBg: Record<StatusTone, string> = {
    ok: "lumen-bg-ok-soft",
    warn: "lumen-bg-warn-soft",
    danger: "lumen-bg-danger-soft",
    muted: "bg-[color:var(--color-bg)]",
  };
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border border-[color:var(--color-border)] px-2.5 py-0.5 text-[11px] font-medium ${toneBg[tone]} ${className}`}
    >
      <span aria-hidden className={`h-1.5 w-1.5 rounded-full ${TONE_CLASS[tone]}`} />
      {children}
    </span>
  );
}

export function Tag({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={`inline-flex items-center rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-[color:var(--color-muted)] ${className}`}
    >
      {children}
    </span>
  );
}

export type SegmentOption<T extends string> = { value: T; label: ReactNode; disabled?: boolean };

export function SegmentedControl<T extends string>({
  value,
  onChange,
  options,
  size = "md",
  className = "",
  ariaLabel,
}: {
  value: T;
  onChange: (v: T) => void;
  options: ReadonlyArray<SegmentOption<T>>;
  size?: "sm" | "md";
  className?: string;
  ariaLabel: string;
}) {
  const padding = size === "sm" ? "px-2 py-1 text-xs" : "px-3 py-1.5 text-sm";
  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      className={`inline-flex items-center gap-0.5 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-0.5 ${className}`}
    >
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            role="radio"
            aria-checked={active}
            disabled={opt.disabled}
            onClick={() => onChange(opt.value)}
            className={`rounded-[5px] ${padding} font-medium transition-colors disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)] ${
              active
                ? "bg-[color:var(--color-card)] text-[color:var(--color-fg)] shadow-[var(--shadow-1)]"
                : "text-[color:var(--color-muted)] hover:text-[color:var(--color-fg)]"
            }`}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

export function Switch({
  checked,
  onCheckedChange,
  ariaLabel,
  disabled,
}: {
  checked: boolean;
  onCheckedChange: (v: boolean) => void;
  ariaLabel: string;
  disabled?: boolean;
}) {
  return (
    <RadixSwitch.Root
      checked={checked}
      onCheckedChange={onCheckedChange}
      aria-label={ariaLabel}
      disabled={disabled}
      className="relative inline-flex h-6 w-10 shrink-0 cursor-pointer items-center rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] transition-colors data-[state=checked]:bg-[color:var(--color-accent)] data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)]"
    >
      <RadixSwitch.Thumb className="block h-4 w-4 translate-x-1 rounded-full bg-[color:var(--color-card)] shadow-[var(--shadow-1)] transition-transform data-[state=checked]:translate-x-5" />
    </RadixSwitch.Root>
  );
}

export function NumberInput({
  value,
  onChange,
  className = "",
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "type"> & {
  value: number | string;
  onChange: (v: string) => void;
}) {
  return (
    <input
      type="number"
      inputMode="numeric"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      {...props}
      className={`w-24 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2.5 py-1.5 text-sm tabular-nums transition-colors focus-visible:border-[color:var(--color-accent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)]/40 disabled:opacity-50 ${className}`}
    />
  );
}

/* ----- Tooltip & Popover are thin Radix wrappers so app code uses the
 *       same DX without each call site repeating styling/positioning. */

export function TooltipProvider({ children }: { children: ReactNode }) {
  return (
    <RadixTooltip.Provider delayDuration={200} skipDelayDuration={300}>
      {children}
    </RadixTooltip.Provider>
  );
}

export function Tooltip({
  content,
  children,
  side = "top",
}: {
  content: ReactNode;
  children: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
}) {
  return (
    <RadixTooltip.Root>
      <RadixTooltip.Trigger asChild>{children}</RadixTooltip.Trigger>
      <RadixTooltip.Portal>
        <RadixTooltip.Content
          side={side}
          sideOffset={6}
          className="z-50 max-w-xs rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2.5 py-1.5 text-xs text-[color:var(--color-fg)] shadow-[var(--shadow-2)]"
        >
          {content}
          <RadixTooltip.Arrow className="fill-[color:var(--color-card)]" />
        </RadixTooltip.Content>
      </RadixTooltip.Portal>
    </RadixTooltip.Root>
  );
}

export function Popover({
  trigger,
  children,
  side = "bottom",
  align = "end",
  sideOffset = 8,
  open,
  onOpenChange,
}: {
  trigger: ReactNode;
  children: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
  sideOffset?: number;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  return (
    <RadixPopover.Root open={open} onOpenChange={onOpenChange}>
      <RadixPopover.Trigger asChild>{trigger}</RadixPopover.Trigger>
      <RadixPopover.Portal>
        <RadixPopover.Content
          side={side}
          align={align}
          sideOffset={sideOffset}
          collisionPadding={12}
          avoidCollisions={true}
          className="z-40 w-[min(92vw,22rem)] rounded-xl border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-4 text-sm shadow-[var(--shadow-3)]"
        >
          {children}
        </RadixPopover.Content>
      </RadixPopover.Portal>
    </RadixPopover.Root>
  );
}
