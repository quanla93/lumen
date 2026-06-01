#!/usr/bin/env bash
#
# Lumen agent installer.
#
# Usage (one-liner, run on the target machine as root):
#
#   curl -fsSL <hub-url>/install.sh | sudo bash -s -- \
#     --token lum_xxxxxxxxxxxxxxxxxxxxx \
#     --host my-server-name
#
# The hub URL is auto-baked into this script by the hub when it serves
# /install.sh — you don't need to pass --hub unless you want to override
# (e.g. internal IP vs. tunnel URL).
#
# Flags:
#   --token <lum_...>    REQUIRED. Per-host bearer token from Hub UI → Settings → Hosts.
#   --host <name>        Optional. Defaults to $(hostname).
#   --hub <url>          Optional. Defaults to the hub that served this script.
#   --interval <dur>     Optional. Defaults to 5s.
#   --uninstall          Stop service, remove binary + unit file, exit.
#
# This script is idempotent — re-running upgrades the binary in place
# and restarts the service.

set -euo pipefail

HUB_URL="{{ .HubURL }}"
# Detect untemplated placeholder — happens when this script is fetched
# directly from GitHub raw instead of through the hub's /install.sh
# endpoint (which renders {{ .HubURL }}). The flag check below will
# then prompt for --hub instead of silently using the literal string.
case "$HUB_URL" in
  *{{*) HUB_URL="" ;;
esac
AGENT_HOST=""
AGENT_TOKEN=""
AGENT_INTERVAL="5s"
UNINSTALL=0

BIN_PATH="/usr/local/bin/lumen-agent"
UNIT_PATH="/etc/systemd/system/lumen-agent.service"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!! \033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxx \033[0m %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --token)     AGENT_TOKEN="$2"; shift 2 ;;
    --host)      AGENT_HOST="$2"; shift 2 ;;
    --hub)       HUB_URL="$2"; shift 2 ;;
    --interval)  AGENT_INTERVAL="$2"; shift 2 ;;
    --uninstall) UNINSTALL=1; shift ;;
    -h|--help)
      sed -n '2,/^set -euo/p' "$0" | sed 's/^# \{0,1\}//; /^set -euo/d'
      exit 0
      ;;
    *) die "unknown flag: $1 (use --help)" ;;
  esac
done

[ "$(id -u)" -eq 0 ] || die "must be run as root (try: sudo bash -s -- ...)"

uninstall() {
  log "Stopping and disabling lumen-agent"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl disable --now lumen-agent 2>/dev/null || true
  fi
  rm -f "$UNIT_PATH" "$BIN_PATH"
  command -v systemctl >/dev/null 2>&1 && systemctl daemon-reload || true
  log "Removed. Bye."
}

if [ "$UNINSTALL" -eq 1 ]; then
  uninstall
  exit 0
fi

[ -n "$AGENT_TOKEN" ] || die "--token is required. Mint one in Hub UI → Settings → Hosts."
[ -n "$HUB_URL" ]     || die "--hub is required (hub didn't bake one in — pass it explicitly)."

[ -n "$AGENT_HOST" ] || AGENT_HOST="$(hostname)"

case "$(uname -s)" in
  Linux) OS="linux" ;;
  *)     die "unsupported OS: $(uname -s) (only Linux is supported via this installer)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)             die "unsupported arch: $(uname -m). Supported: x86_64, aarch64. (armv7 dropped for faster image builds — open an issue if you need it back.)" ;;
esac

ARTIFACT="lumen-agent-${OS}-${ARCH}"
DOWNLOAD_URL="${HUB_URL%/}/install/${ARTIFACT}"

log "Hub:      $HUB_URL"
log "Host:     $AGENT_HOST"
log "Arch:     $OS/$ARCH"
log "Interval: $AGENT_INTERVAL"

log "Downloading $ARTIFACT"
TMP="$(mktemp)"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$DOWNLOAD_URL" -o "$TMP" || die "download failed from $DOWNLOAD_URL"
elif command -v wget >/dev/null 2>&1; then
  wget -q -O "$TMP" "$DOWNLOAD_URL" || die "download failed from $DOWNLOAD_URL"
else
  die "need curl or wget"
fi

log "Installing to $BIN_PATH"
install -m 0755 "$TMP" "$BIN_PATH"
rm -f "$TMP"

# Persistent buffer dir for the on-disk offline queue. The agent
# writes a small bbolt file here when the hub is unreachable so
# frames replay after reconnect. Root-owned because the agent runs
# as root (needed for /proc + docker.sock).
BUFFER_DIR="/var/lib/lumen-agent"
log "Ensuring buffer dir $BUFFER_DIR"
install -d -m 0750 "$BUFFER_DIR"

if ! command -v systemctl >/dev/null 2>&1; then
  warn "systemd not detected — binary installed at $BIN_PATH but no service registered."
  warn "Run it manually with:"
  warn "  LUMEN_HUB_URL='$HUB_URL' LUMEN_AGENT_HOST='$AGENT_HOST' LUMEN_AGENT_TOKEN='$AGENT_TOKEN' LUMEN_AGENT_BUFFER_PATH=$BUFFER_DIR/buffer.db $BIN_PATH"
  exit 0
fi

log "Writing $UNIT_PATH"
cat > "$UNIT_PATH" <<UNIT
[Unit]
Description=Lumen agent
Documentation=https://github.com/quanla93/lumen
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=LUMEN_HUB_URL=$HUB_URL
Environment=LUMEN_AGENT_HOST=$AGENT_HOST
Environment=LUMEN_AGENT_TOKEN=$AGENT_TOKEN
Environment=LUMEN_AGENT_INTERVAL=$AGENT_INTERVAL
Environment=LUMEN_AGENT_BUFFER_PATH=$BUFFER_DIR/buffer.db
ExecStart=$BIN_PATH
Restart=always
RestartSec=5s

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
# Carve out the buffer dir so ProtectSystem=strict still lets the
# agent persist its offline queue across restarts.
ReadWritePaths=$BUFFER_DIR

[Install]
WantedBy=multi-user.target
UNIT

chmod 0600 "$UNIT_PATH"  # token lives here — keep it root-only readable

log "Reloading systemd and starting lumen-agent"
systemctl daemon-reload
systemctl enable --now lumen-agent

sleep 2
if systemctl is-active --quiet lumen-agent; then
  log "lumen-agent is running."
  log "Tail logs with:  journalctl -u lumen-agent -f"
else
  warn "lumen-agent failed to start. Inspect with:"
  warn "  systemctl status lumen-agent"
  warn "  journalctl -u lumen-agent -n 50 --no-pager"
  exit 1
fi
