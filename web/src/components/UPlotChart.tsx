import { useEffect, useRef } from "react";
import uPlot, { type AlignedData, type Options } from "uplot";

// UPlotChart is a thin React wrapper around uPlot. We control resize via a
// ResizeObserver because uPlot itself doesn't track its container's width —
// it only respects the explicit width passed at construction or via
// setSize().
export function UPlotChart({
  data,
  options,
  className,
}: {
  data: AlignedData;
  options: Omit<Options, "width" | "height">;
  className?: string;
}) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const plotRef = useRef<uPlot | null>(null);

  // Construct uPlot once; tear down on unmount.
  useEffect(() => {
    const wrap = wrapRef.current;
    if (!wrap) return;
    const width = wrap.clientWidth || 600;
    const height = wrap.clientHeight || 200;
    const plot = new uPlot(
      { ...options, width, height } as Options,
      data,
      wrap,
    );
    plotRef.current = plot;

    const ro = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry || !plotRef.current) return;
      const w = Math.floor(entry.contentRect.width);
      const h = Math.floor(entry.contentRect.height) || height;
      if (w > 0) plotRef.current.setSize({ width: w, height: h });
    });
    ro.observe(wrap);

    return () => {
      ro.disconnect();
      plot.destroy();
      plotRef.current = null;
    };
    // We deliberately want to construct uPlot only ONCE per chart instance.
    // Updates flow through setData / a fresh series array via the other
    // useEffect below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Push new data into the existing plot when it changes.
  useEffect(() => {
    if (plotRef.current) {
      plotRef.current.setData(data);
    }
  }, [data]);

  return <div ref={wrapRef} className={className} />;
}
