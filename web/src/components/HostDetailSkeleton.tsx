// HostDetailSkeleton — Suspense fallback for the lazy-loaded
// HostDetail bundle. Mirrors the visual structure of the real
// host detail page (header strip + metric grid placeholder) so
// the perceived layout doesn't jump when the bundle resolves.
//
// Kept dependency-free (no react-grid-layout, no uPlot) so it
// lands in the entry chunk without dragging the heavy deps
// in with it. The CSS uses the same tokens as the rest of
// the app (surface, border, rounded) so the skeleton blends in.

import { ArrowLeft, Cpu, MemoryStick, HardDrive } from "lucide-react";

export function HostDetailSkeleton({ onBack }: { onBack: () => void }) {
  return (
    <div className="flex flex-col gap-4" aria-busy="true" aria-live="polite">
      <div className="flex items-center gap-2 text-sm text-surface-fg-subtle">
        <button
          type="button"
          onClick={onBack}
          className="inline-flex items-center gap-1.5 rounded-md border border-surface-border bg-surface-0 px-2 py-1 hover:bg-surface-1"
        >
          <ArrowLeft size={14} strokeWidth={1.75} />
          <span>Back</span>
        </button>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {[
          { icon: Cpu, label: "CPU" },
          { icon: MemoryStick, label: "RAM" },
          { icon: HardDrive, label: "Disk" },
        ].map(({ icon: Icon, label }) => (
          <div
            key={label}
            className="rounded-lg border border-surface-border bg-surface-0 p-3"
          >
            <div className="flex items-center gap-2 text-xs uppercase tracking-wide text-surface-fg-subtle">
              <Icon size={12} strokeWidth={1.75} aria-hidden="true" />
              {label}
            </div>
            <div
              className="mt-2 h-6 w-24 animate-pulse rounded bg-surface-2"
              aria-hidden="true"
            />
          </div>
        ))}
      </div>
      <div
        className="h-64 animate-pulse rounded-lg border border-surface-border bg-surface-0"
        aria-hidden="true"
      />
    </div>
  );
}
