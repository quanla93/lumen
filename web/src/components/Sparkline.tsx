/** Dependency-free SVG sparkline. Renders a polyline of `values` scaled
 * to fit the viewBox; values are expected in [0,100] (CPU%, RAM%, etc.).
 * Width and height are in CSS pixels; the SVG uses preserveAspectRatio
 * "none" so the trace stretches to fill any container width via CSS. */
export function Sparkline({
  values,
  width = 80,
  height = 18,
  className,
}: {
  values: number[];
  width?: number;
  height?: number;
  className?: string;
}) {
  if (values.length < 2) {
    return (
      <svg
        viewBox={`0 0 ${width} ${height}`}
        width={width}
        height={height}
        preserveAspectRatio="none"
        aria-hidden
        className={className}
      />
    );
  }

  const step = width / (values.length - 1);
  const points = values
    .map((v, i) => {
      const clamped = Math.max(0, Math.min(100, v));
      const x = (i * step).toFixed(2);
      const y = (height - (clamped / 100) * height).toFixed(2);
      return `${x},${y}`;
    })
    .join(" ");

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      width={width}
      height={height}
      preserveAspectRatio="none"
      aria-hidden
      className={className}
    >
      <polyline
        points={points}
        fill="none"
        stroke="currentColor"
        strokeWidth={1.25}
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
