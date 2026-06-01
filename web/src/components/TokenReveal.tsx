import { useState } from "react";
import { GhostButton } from "@/components/CenterCard";
import { SegmentedControl } from "@/components/ui";
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
  const [copied, setCopied] = useState<"token" | "compose" | "commands" | "binary" | "binaryGithub" | null>(null);
  const [method, setMethod] = useState<"docker" | "binary">("docker");
  const hubUrl =
    typeof window !== "undefined"
      ? `${window.location.protocol}//${window.location.host}`
      : "https://your-hub.example.com";

  const dockerHubUrl = dockerReachableHubUrl(hubUrl);
  const safeHostName = hostName.replace(/[^A-Za-z0-9_.-]/g, "-");

  // Binary install one-liner. The hub's /install.sh endpoint bakes
  // HUB_URL into the script (templated from request Host header), so
  // the operator only needs to pass --token + --host. install-agent.sh
  // detects arch, downloads the matching binary from /install/<name>,
  // drops a systemd unit, enables it.
  const binaryCommand =
    `curl -fsSL ${hubUrl}/install.sh | sudo bash -s -- \\\n` +
    `    --token ${token} \\\n` +
    `    --host ${hostName}`;

  // Same script fetched from the public GitHub repo — for cases where
  // the target LXC can reach github.com but can't (yet) reach the hub
  // URL (firewall, split DNS, etc.). Script's `{{ .HubURL }}` template
  // isn't rendered when fetched from raw, so --hub is mandatory here.
  const binaryGithubCommand =
    `curl -fsSL https://raw.githubusercontent.com/quanla93/lumen/main/scripts/install-agent.sh | sudo bash -s -- \\\n` +
    `    --hub ${hubUrl} \\\n` +
    `    --token ${token} \\\n` +
    `    --host ${hostName}`;

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

  async function copy(text: string, which: "token" | "compose" | "commands" | "binary" | "binaryGithub") {
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
        <SegmentedControl
          value={method}
          onChange={setMethod}
          options={[
            { value: "docker", label: t("token.methodDocker") },
            { value: "binary", label: t("token.methodBinary") },
          ]}
          ariaLabel={t("token.installTitle")}
        />
      </div>

      {method === "docker" && (
        <>
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
        </>
      )}

      {method === "binary" && (
        <>
          <div className="mb-4">
            <div className="flex items-center justify-between gap-2 mb-1">
              <p className="text-xs font-semibold">{t("token.binaryTitle")}</p>
              <GhostButton onClick={() => copy(binaryCommand, "binary")}>
                {copied === "binary" ? t("common.copied") : t("common.copy")}
              </GhostButton>
            </div>
            <pre className="text-xs font-mono bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded p-3 overflow-x-auto whitespace-pre-wrap">{binaryCommand}</pre>
            <p className="text-xs text-[color:var(--color-muted)] mt-2">
              {t("token.binaryDescription")}
            </p>
            <p className="text-xs text-[color:var(--color-muted)] mt-1">
              {t("token.binaryRequirements")}
            </p>
          </div>
          <div className="mb-4">
            <div className="flex items-center justify-between gap-2 mb-1">
              <p className="text-xs font-semibold">{t("token.binaryGithubTitle")}</p>
              <GhostButton onClick={() => copy(binaryGithubCommand, "binaryGithub")}>
                {copied === "binaryGithub" ? t("common.copied") : t("common.copy")}
              </GhostButton>
            </div>
            <pre className="text-xs font-mono bg-[color:var(--color-card)] border border-[color:var(--color-border)] rounded p-3 overflow-x-auto whitespace-pre-wrap">{binaryGithubCommand}</pre>
            <p className="text-xs text-[color:var(--color-muted)] mt-2">
              {t("token.binaryGithubDescription")}
            </p>
          </div>
        </>
      )}

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
