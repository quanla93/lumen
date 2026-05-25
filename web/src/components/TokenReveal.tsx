import { useState } from "react";
import { GhostButton } from "@/components/CenterCard";

/** One-shot token display — shows the plaintext + copy button + agent
 * install snippet. Sticks around in the parent's state until the parent
 * removes it (the token CAN'T be fetched again from the hub). */
export function TokenReveal({
  hostName,
  token,
  onDismiss,
}: {
  hostName: string;
  token: string;
  onDismiss: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const hubUrl =
    typeof window !== "undefined"
      ? `${window.location.protocol}//${window.location.host}`
      : "https://your-hub.example.com";

  const envSnippet = `LUMEN_HUB_URL=${hubUrl}\nLUMEN_AGENT_TOKEN=${token}\nLUMEN_AGENT_HOST=${hostName}`;

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard API may be unavailable on non-HTTPS — user can manually copy
    }
  }

  return (
    <div className="rounded-lg border border-[color:var(--color-warn)]/30 bg-[color:var(--color-warn)]/5 p-4">
      <div className="flex items-start justify-between gap-3 mb-3">
        <div>
          <h3 className="text-sm font-semibold">
            Token for <span className="font-mono">{hostName}</span> — shown once
          </h3>
          <p className="text-xs text-[color:var(--color-muted)] mt-0.5">
            Copy it now. The hub stores only a hash — if you lose it, rotate the token.
          </p>
        </div>
        <GhostButton onClick={onDismiss}>Dismiss</GhostButton>
      </div>

      <div className="flex items-center gap-2 mb-4">
        <code className="flex-1 font-mono text-xs bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded px-2 py-1.5 overflow-x-auto whitespace-nowrap">
          {token}
        </code>
        <GhostButton onClick={() => copy(token)}>
          {copied ? "Copied!" : "Copy"}
        </GhostButton>
      </div>

      <div>
        <p className="text-xs text-[color:var(--color-muted)] mb-1">
          Set these on the host you want to monitor, then run the agent:
        </p>
        <pre className="text-xs font-mono bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded p-3 overflow-x-auto">{envSnippet}</pre>
        <p className="text-xs text-[color:var(--color-muted)] mt-2">
          The agent loads <code>.env</code> from its working directory; either
          paste these into an <code>.env</code> file or export them as env vars.
        </p>
      </div>
    </div>
  );
}
