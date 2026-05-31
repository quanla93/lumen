import { useState } from "react";
import { GhostButton } from "@/components/CenterCard";
import { copyToClipboard } from "@/lib/clipboard";
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

/** One-shot token display — shows the generated Docker Compose agent setup
 * plus the plaintext token for manual fallback. Sticks around in the parent's
 * state until the parent removes it (the token CAN'T be fetched again from the hub). */
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
  const [copied, setCopied] = useState<"token" | "compose" | "commands" | null>(null);
  const hubUrl =
    typeof window !== "undefined"
      ? `${window.location.protocol}//${window.location.host}`
      : "https://your-hub.example.com";

  const dockerHubUrl = dockerReachableHubUrl(hubUrl);
  const safeHostName = hostName.replace(/[^A-Za-z0-9_.-]/g, "-");

  const agentImage = "ghcr.io/quanla93/lumen-agent:latest";
  const compose = `services:
  lumen-agent:
    image: ${agentImage}
    container_name: lumen-agent-${safeHostName}
    restart: unless-stopped
    user: "0:0"
    environment:
      LUMEN_HUB_URL: "${dockerHubUrl}"
      LUMEN_AGENT_TOKEN: "${token}"
      LUMEN_AGENT_HOST: "${hostName}"
      LUMEN_AGENT_INTERVAL: "5s"
      LUMEN_AGENT_BUFFER_PATH: "/data/buffer.db"
      LUMEN_AGENT_BUFFER_MAX_AGE: "24h"
      LUMEN_AGENT_BUFFER_DRAIN: "10"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - lumen-agent-data:/data

volumes:
  lumen-agent-data:
`;
  const composeCommands = `sudo mkdir -p /opt/lumen-agent
cd /opt/lumen-agent
# Save the generated docker-compose.yml in this directory, then start the agent:
sudo docker compose up -d
sudo docker compose logs -f

# Future updates:
sudo docker compose pull
sudo docker compose up -d`;

  async function copy(text: string, which: "token" | "compose" | "commands") {
    if (await copyToClipboard(text)) {
      setCopied(which);
      setTimeout(() => setCopied(null), 1500);
    }
  }

  function downloadCompose() {
    const blob = new Blob([compose], { type: "text/yaml" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "docker-compose.yml";
    a.click();
    URL.revokeObjectURL(url);
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

      <div className="mb-4">
        <div className="flex items-center justify-between gap-2 mb-1">
          <p className="text-xs font-semibold">{t("token.composeCommandsTitle")}</p>
          <GhostButton onClick={() => copy(composeCommands, "commands")}>
            {copied === "commands" ? t("common.copied") : t("common.copy")}
          </GhostButton>
        </div>
        <pre className="text-xs font-mono bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded p-3 overflow-x-auto whitespace-pre-wrap">{composeCommands}</pre>
        <p className="text-xs text-[color:var(--color-muted)] mt-2">
          {t("token.composeCommandsDescription")}
        </p>
      </div>

      <div className="mb-4">
        <div className="flex items-center justify-between gap-2 mb-1">
          <p className="text-xs font-semibold">{t("token.composeTitle")}</p>
          <div className="flex items-center gap-2">
            <GhostButton onClick={downloadCompose}>{t("common.download")}</GhostButton>
            <GhostButton onClick={() => copy(compose, "compose")}>
              {copied === "compose" ? t("common.copied") : t("common.copy")}
            </GhostButton>
          </div>
        </div>
        <pre className="text-xs font-mono bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded p-3 overflow-x-auto whitespace-pre-wrap">{compose}</pre>
        <p className="text-xs text-[color:var(--color-muted)] mt-2">
          {t("token.composeDescription")}
        </p>
      </div>

      <div>
        <div className="flex items-center justify-between gap-2 mb-1">
          <p className="text-xs font-semibold">{t("token.manualTokenTitle")}</p>
          <GhostButton onClick={() => copy(token, "token")}>
            {copied === "token" ? t("common.copied") : t("common.copy")}
          </GhostButton>
        </div>
        <code className="block font-mono text-xs bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded px-2 py-1.5 overflow-x-auto whitespace-nowrap">
          {token}
        </code>
      </div>
    </div>
  );
}
