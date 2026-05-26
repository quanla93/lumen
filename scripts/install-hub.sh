#!/usr/bin/env bash
#
# Lumen hub installer (native binary, systemd service).
#
# Use this when you want to run the hub as a regular Linux service
# instead of in Docker. Works on Debian / Ubuntu / Alpine / RHEL — any
# systemd-based distro. Tested specifically on Proxmox LXC containers
# (Debian + Ubuntu templates).
#
# WORKFLOW
#
#   # 1. On a build machine (Mac/Linux with Go + pnpm):
#   make release-hub-tarball ARCH=linux-amd64
#   # → produces dist/lumen-hub-linux-amd64.tar.gz
#
#   # 2. Copy the tarball to the target LXC / Linux host:
#   scp dist/lumen-hub-linux-amd64.tar.gz root@my-lxc:/tmp/
#
#   # 3. On the target, as root:
#   cd /tmp && tar xf lumen-hub-linux-amd64.tar.gz
#   cd lumen-hub-linux-amd64
#   sudo ./install-hub.sh
#
#   # 4. Open http://<this-host>:8090 and log in with admin / lumenadmin
#   #    (the default; change LUMEN_HUB_ADMIN_PASSWORD in /etc/lumen/hub.env
#   #    before running install.sh if you want a different bootstrap value).
#
# FLAGS
#
#   --uninstall   Stop service, remove binary + unit. KEEPS /etc/lumen and
#                 /var/lib/lumen (your SQLite DB + config) — re-running
#                 install.sh on a later tarball is then an in-place upgrade.
#   --purge       Like --uninstall but also nukes /etc/lumen and /var/lib/lumen.
#                 IRREVERSIBLE (drops the DB).
#
# This script is idempotent — re-run on a newer tarball to upgrade the
# binary in place; /etc/lumen/hub.env is preserved.

set -euo pipefail

UNINSTALL=0
PURGE=0

# Paths the installer touches. Keep in sync with docs/install/hub-binary.md.
BIN_PATH="/usr/local/bin/lumen-hub"
UNIT_PATH="/etc/systemd/system/lumen-hub.service"
ENV_PATH="/etc/lumen/hub.env"
DATA_DIR="/var/lib/lumen"
ETC_DIR="/etc/lumen"
SVC_USER="lumen"
SVC_GROUP="lumen"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!! \033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxx \033[0m %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --uninstall) UNINSTALL=1; shift ;;
    --purge)     PURGE=1; UNINSTALL=1; shift ;;
    -h|--help)
      sed -n '2,/^set -euo/p' "$0" | sed 's/^# \{0,1\}//; /^set -euo/d'
      exit 0
      ;;
    *) die "unknown flag: $1 (use --help)" ;;
  esac
done

[ "$(id -u)" -eq 0 ] || die "must be run as root (try: sudo ./install-hub.sh)"

# ─── uninstall path ──────────────────────────────────────────────────────────
if [ "$UNINSTALL" -eq 1 ]; then
  log "Stopping and disabling lumen-hub"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl disable --now lumen-hub 2>/dev/null || true
  fi
  rm -f "$UNIT_PATH" "$BIN_PATH"
  command -v systemctl >/dev/null 2>&1 && systemctl daemon-reload || true
  if [ "$PURGE" -eq 1 ]; then
    warn "PURGE: removing $ETC_DIR and $DATA_DIR"
    rm -rf "$ETC_DIR" "$DATA_DIR"
    userdel "$SVC_USER" 2>/dev/null || true
  else
    log "Kept $ETC_DIR and $DATA_DIR (use --purge to also wipe these)"
  fi
  log "Removed. Bye."
  exit 0
fi

# ─── platform check ──────────────────────────────────────────────────────────
case "$(uname -s)" in
  Linux) OS="linux" ;;
  *)     die "unsupported OS: $(uname -s) (this installer only supports Linux)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l|armv7)  ARCH="armv7" ;;
  *)             die "unsupported arch: $(uname -m). Supported: x86_64, aarch64, armv7l." ;;
esac

SRC_BINARY="$(dirname "$(readlink -f "$0")")/lumen-hub"
SRC_UNIT="$(dirname "$(readlink -f "$0")")/lumen-hub.service"
SRC_ENV="$(dirname "$(readlink -f "$0")")/hub.env.example"

[ -f "$SRC_BINARY" ] || die "lumen-hub binary not found next to install-hub.sh ($SRC_BINARY).
   This installer expects to live inside the release tarball — see --help."
[ -f "$SRC_UNIT" ]   || die "lumen-hub.service not found next to install-hub.sh ($SRC_UNIT)."

# ─── service user ────────────────────────────────────────────────────────────
if ! id "$SVC_USER" >/dev/null 2>&1; then
  log "Creating system user $SVC_USER"
  # Use --system + --no-create-home + --shell /usr/sbin/nologin so the user
  # has no login surface. busybox adduser (Alpine) uses different flags.
  if command -v useradd >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin \
      --home-dir "$DATA_DIR" "$SVC_USER"
  else
    # Alpine / busybox
    adduser -S -D -H -h "$DATA_DIR" -s /sbin/nologin "$SVC_USER"
  fi
else
  log "User $SVC_USER already exists"
fi

# ─── dirs ────────────────────────────────────────────────────────────────────
install -d -m 0750 -o "$SVC_USER" -g "$SVC_GROUP" "$DATA_DIR"
install -d -m 0750 -o root        -g "$SVC_GROUP" "$ETC_DIR"

# ─── env file (idempotent — preserve existing customizations) ────────────────
if [ ! -f "$ENV_PATH" ]; then
  log "Writing initial $ENV_PATH (random LUMEN_HUB_SECRET, default admin password)"
  SECRET="$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p -c 64)"
  if [ -f "$SRC_ENV" ]; then
    # Use the bundled template, substitute the random secret.
    sed "s|^LUMEN_HUB_SECRET=.*|LUMEN_HUB_SECRET=$SECRET|" "$SRC_ENV" > "$ENV_PATH"
  else
    # Fallback: write a minimal env inline.
    cat > "$ENV_PATH" <<ENV
# Lumen hub environment — managed by install-hub.sh on first install,
# preserved on upgrade. Edit freely; \`systemctl restart lumen-hub\`
# to apply.

LUMEN_HUB_ADDR=:8090
LUMEN_HUB_DEV=false
LUMEN_HUB_DB_PATH=$DATA_DIR/lumen.db
LUMEN_HUB_SECRET=$SECRET
LUMEN_HUB_RETENTION_WINDOW=24h
LUMEN_HUB_RETENTION_INTERVAL=1h
LUMEN_HUB_ADMIN_USERNAME=admin
LUMEN_HUB_ADMIN_PASSWORD=lumenadmin
ENV
  fi
  chmod 0640 "$ENV_PATH"
  chown root:"$SVC_GROUP" "$ENV_PATH"
else
  log "Preserving existing $ENV_PATH (delete it manually if you want a reset)"
fi

# ─── binary ──────────────────────────────────────────────────────────────────
log "Installing binary → $BIN_PATH"
install -m 0755 "$SRC_BINARY" "$BIN_PATH"

# ─── systemd unit ────────────────────────────────────────────────────────────
if ! command -v systemctl >/dev/null 2>&1; then
  warn "systemd not detected. Binary is installed at $BIN_PATH; start it manually:"
  warn "  sudo -u $SVC_USER env \$(cat $ENV_PATH | xargs) $BIN_PATH"
  exit 0
fi

log "Writing $UNIT_PATH"
install -m 0644 "$SRC_UNIT" "$UNIT_PATH"

log "Reloading systemd and starting lumen-hub"
systemctl daemon-reload
systemctl enable --now lumen-hub

sleep 2
if systemctl is-active --quiet lumen-hub; then
  log "lumen-hub is running."
  PORT="$(grep -E '^LUMEN_HUB_ADDR=' "$ENV_PATH" | sed 's/.*://' | tr -d '\"')"
  log "Open  http://$(hostname -f 2>/dev/null || hostname):${PORT:-8090}"
  log "Logs  journalctl -u lumen-hub -f"
  log "Env   $ENV_PATH"
else
  warn "lumen-hub failed to start. Inspect with:"
  warn "  systemctl status lumen-hub"
  warn "  journalctl -u lumen-hub -n 50 --no-pager"
  exit 1
fi
