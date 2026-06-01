import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";
import * as RadixDialog from "@radix-ui/react-dialog";
import { AlertTriangle, X } from "lucide-react";
import { AppButton } from "@/components/ui";
import { useI18n } from "@/i18n/useI18n";

// In-app replacement for window.confirm() — Radix AlertDialog under the
// hood, styled to match the rest of the UI. Imperative Promise API
// minimizes the diff at callsites: `await confirm({...})` swaps in for
// `window.confirm(...)` with a one-line shape change.

type ConfirmOptions = {
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  // destructive=true paints the confirm button danger-red. Use for
  // delete/rotate flows where the action can't be undone.
  destructive?: boolean;
};

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>;

const ConfirmCtx = createContext<ConfirmFn | null>(null);

export function useConfirm(): ConfirmFn {
  const fn = useContext(ConfirmCtx);
  if (!fn) throw new Error("useConfirm must be used inside <ConfirmProvider>");
  return fn;
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [opts, setOpts] = useState<ConfirmOptions | null>(null);
  // Promise resolver kept in a ref so re-renders don't drop it.
  const resolverRef = useRef<((v: boolean) => void) | null>(null);

  const confirm = useCallback<ConfirmFn>((next) => {
    setOpts(next);
    setOpen(true);
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
    });
  }, []);

  function resolve(answer: boolean) {
    resolverRef.current?.(answer);
    resolverRef.current = null;
    setOpen(false);
  }

  return (
    <ConfirmCtx.Provider value={confirm}>
      {children}
      <RadixDialog.Root open={open} onOpenChange={(o) => { if (!o) resolve(false); }}>
        <RadixDialog.Portal>
          <RadixDialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm data-[state=open]:animate-in data-[state=open]:fade-in-0" />
          <RadixDialog.Content
            className="fixed left-1/2 top-1/2 z-50 w-[min(92vw,28rem)] -translate-x-1/2 -translate-y-1/2 rounded-xl border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-5 shadow-[var(--shadow-3)]"
          >
            <div className="flex items-start gap-3">
              {opts?.destructive && (
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-[color:var(--color-danger)]/12 text-[color:var(--color-danger)]">
                  <AlertTriangle size={18} strokeWidth={1.75} />
                </div>
              )}
              <div className="min-w-0 flex-1">
                <RadixDialog.Title className="text-base font-semibold text-[color:var(--color-fg)]">
                  {opts?.title}
                </RadixDialog.Title>
                <RadixDialog.Description asChild>
                  <div className="mt-1.5 text-sm text-[color:var(--color-muted)]">
                    {opts?.message}
                  </div>
                </RadixDialog.Description>
              </div>
              <RadixDialog.Close asChild>
                <button
                  type="button"
                  aria-label={t("common.dismiss")}
                  className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-[color:var(--color-muted)] hover:bg-[color:var(--color-border)] hover:text-[color:var(--color-fg)]"
                >
                  <X size={14} strokeWidth={1.75} />
                </button>
              </RadixDialog.Close>
            </div>
            <div className="mt-5 flex items-center justify-end gap-2">
              <AppButton variant="secondary" onClick={() => resolve(false)}>
                {opts?.cancelLabel ?? t("common.cancel")}
              </AppButton>
              <AppButton
                variant={opts?.destructive ? "danger" : "primary"}
                onClick={() => resolve(true)}
                autoFocus
              >
                {opts?.confirmLabel ?? t("common.confirm")}
              </AppButton>
            </div>
          </RadixDialog.Content>
        </RadixDialog.Portal>
      </RadixDialog.Root>
    </ConfirmCtx.Provider>
  );
}
