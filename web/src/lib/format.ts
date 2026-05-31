// Shared number / byte / throughput formatters. Extracted from HostDetail
// so HostCard (and future surfaces) can share the same display rules.

export function formatBytes(b: number): string {
  if (!b) return "0 B";
  const abs = Math.abs(b);
  if (abs >= 1024 * 1024 * 1024) return `${(b / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
  if (abs >= 1024 * 1024)        return `${(b / (1024 * 1024)).toFixed(1)} MiB`;
  if (abs >= 1024)               return `${(b / 1024).toFixed(0)} KiB`;
  return `${b.toFixed(0)} B`;
}

export function formatBps(bps: number): string {
  const abs = Math.abs(bps);
  if (abs >= 1_000_000_000) return `${(bps / 1_000_000_000).toFixed(2)} GB/s`;
  if (abs >= 1_000_000)     return `${(bps / 1_000_000).toFixed(2)} MB/s`;
  if (abs >= 1_000)         return `${(bps / 1_000).toFixed(1)} kB/s`;
  return `${bps.toFixed(0)} B/s`;
}
