// Inline copy of brand/logo-monochrome.svg — three concentric arcs + a
// center dot ("pulse of light"). Uses currentColor so the surrounding
// `color:` rule picks the tint; wrap in a span with text-teal-* (or any
// CSS var) to colorize. Keeps the SVG portable and bundles into the
// embedded web assets without an extra HTTP fetch.

export function LogoMark({
  size = 24,
  className = "",
  ariaLabel = "Lumen",
}: {
  size?: number;
  className?: string;
  ariaLabel?: string;
}) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 64 64"
      width={size}
      height={size}
      fill="none"
      role="img"
      aria-label={ariaLabel}
      className={className}
    >
      <path
        d="M 12 32 a 20 20 0 0 1 40 0"
        stroke="currentColor" strokeWidth="3" strokeLinecap="round" opacity="0.35"
      />
      <path
        d="M 18 32 a 14 14 0 0 1 28 0"
        stroke="currentColor" strokeWidth="3" strokeLinecap="round" opacity="0.65"
      />
      <path
        d="M 24 32 a 8 8 0 0 1 16 0"
        stroke="currentColor" strokeWidth="3" strokeLinecap="round"
      />
      <circle cx="32" cy="32" r="4" fill="currentColor" />
    </svg>
  );
}

// LumenWordmark pairs the mark with the "Lumen" label. The teal class on
// the wrapper colors the mark; the label inherits the parent's text color
// so it stays high-contrast in any theme.
export function LumenWordmark({
  size = 24,
  className = "",
}: { size?: number; className?: string }) {
  return (
    <span className={`inline-flex items-center gap-2 ${className}`}>
      <span className="text-[color:var(--lumen-teal)]">
        <LogoMark size={size} />
      </span>
      <span className="font-semibold tracking-tight">Lumen</span>
    </span>
  );
}
