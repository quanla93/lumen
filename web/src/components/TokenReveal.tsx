import { useState } from "react";
import { GhostButton } from "@/components/CenterCard";
import { useI18n } from "@/i18n/useI18n";

function dockerReachableHubUrl(hubUrl: string): string {
  try {
    const u = new URL(hubUrl);
    if (u.hostname === "localhost" || u.hostname === "127.0.0.1") {
      u.hostname = "host.docker.internal";
    }
    return u.toString().replace(/\/$/, "");
  } catch {
    return hubUrl;
  }
}

/** One-shot token display — shows the plaintext + copy button + the
 * preferred one-liner install command + a fallback manual snippet.
 * Sticks around in the parent's state until the parent removes it
 * (the token CAN'T be fetched again from the hub). */
export function TokenReveal({
  hostName,
  token,
  onDismiss,
}: {
  hostName: string;
  token: string;
  onDismiss: () => void;
}) {
  const { t } = useI18n();
  const [copied, setCopied] = useState<"token" | "oneliner" | "docker" | "env" | null>(null);
  const hubUrl =
    typeof window !== "undefined"
      ? `${window.location.protocol}//${window.location.host}`
      : "https://your-hub.example.com";

  const dockerHubUrl = dockerReachableHubUrl(hubUrl);
  const safeHostName = hostName.replace(/[^A-Za-z0-9_.-]/g, "-");

  const oneLiner = `curl -fsSL ${hubUrl}/install.sh | sudo bash -s -- --token ${token} --host ${hostName}`;
  const agentImage = "ghcr.io/quanla93/lumen-agent:latest";
  const dockerRun = `docker run -d --pull always --name lumen-agent-${safeHostName} --restart unless-stopped -e LUMEN_HUB_URL=${dockerHubUrl} -e LUMEN_AGENT_TOKEN=${token} -e LUMEN_AGENT_HOST=${hostName} -e LUMEN_AGENT_INTERVAL=5s -v /var/run/docker.sock:/var/run/docker.sock:ro --user 0:0 ${agentImage}`;
  const envSnippet = `LUMEN_HUB_URL=${hubUrl}\nLUMEN_AGENT_TOKEN=${token}\nLUMEN_AGENT_HOST=${hostName}`;

  async function copy(text: string, which: "token" | "oneliner" | "docker" | "env") {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(which);
      setTimeout(() => setCopied(null), 1500);
    } catch {
      // clipboard API may be unavailable on non-HTTPS — user can manually copy
    }
  }

  return (
    <div className="rounded-lg border border-[color:var(--color-warn)]/30 bg-[color:var(--color-warn)]/5 p-4">
      <div className="flex items-start justify-between gap-3 mb-3">
        <div>
          <h3 className="text-sm font-semibold">
            {t("token.title", { host: hostName })}
          </h3>
          <p className="text-xs text-[color:var(--color-muted)] mt-0.5">
            {t("token.description")}
          </p>
        </div>
        <GhostButton onClick={onDismiss}>{t("common.dismiss")}</GhostButton>
      </div>

      <div className="flex items-center gap-2 mb-4">
        <code className="flex-1 font-mono text-xs bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded px-2 py-1.5 overflow-x-auto whitespace-nowrap">
          {token}
        </code>
        <GhostButton onClick={() => copy(token, "token")}>
          {copied === "token" ? t("common.copied") : t("common.copy")}
        </GhostButton>
      </div>

      <div className="mb-4">
        <div className="flex items-center justify-between mb-1">
          <p className="text-xs font-semibold">{t("token.installTitle")}</p>
          <GhostButton onClick={() => copy(oneLiner, "oneliner")}>
            {copied === "oneliner" ? t("common.copied") : t("common.copy")}
          </GhostButton>
        </div>
        <pre className="text-xs font-mono bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded p-3 overflow-x-auto">{oneLiner}</pre>
        <p className="text-xs text-[color:var(--color-muted)] mt-2">
          {t("token.installDescription")}
        </p>
      </div>

      <div className="mb-4">
        <div className="flex items-center justify-between mb-1">
          <p className="text-xs font-semibold">{t("token.dockerTitle")}</p>
          <GhostButton onClick={() => copy(dockerRun, "docker")}>
            {copied === "docker" ? t("common.copied") : t("common.copy")}
          </GhostButton>
        </div>
        <pre className="text-xs font-mono bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded p-3 overflow-x-auto whitespace-pre-wrap">{dockerRun}</pre>
        <p className="text-xs text-[color:var(--color-muted)] mt-2">
          {t("token.dockerDescription")}
        </p>
      </div>

      <details>
        <summary className="text-xs text-[color:var(--color-muted)] cursor-pointer hover:text-[color:var(--color-fg)]">
          {t("token.manualTitle")}
        </summary>
        <div className="mt-2">
          <div className="flex items-center justify-between mb-1">
            <p className="text-xs">{t("token.manualDescription")}</p>
            <GhostButton onClick={() => copy(envSnippet, "env")}>
              {copied === "env" ? t("common.copied") : t("common.copy")}
            </GhostButton>
          </div>
          <pre className="text-xs font-mono bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded p-3 overflow-x-auto">{envSnippet}</pre>
        </div>
      </details>
    </div>
  );
}
