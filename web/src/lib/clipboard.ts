// copyToClipboard wraps the Clipboard API with a legacy execCommand
// fallback so copy buttons keep working when the dashboard is loaded
// over plain HTTP on a LAN IP (e.g. http://192.168.x.y:8090). The
// modern navigator.clipboard API requires a secure context (HTTPS or
// localhost) — outside that, it's either undefined or throws, and
// homelab users would silently get nothing on the clipboard.
//
// document.execCommand("copy") is marked deprecated but still ships
// in every browser as of 2026; Grafana / Vault / Gitea use the same
// fallback for the same reason. When the operator upgrades to HTTPS
// (Tailscale Serve, reverse proxy with cert, etc.) the modern path
// transparently takes over and this fallback becomes dead code.
export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Permissions blocked, user gesture missing, or some other
      // browser-specific veto — fall through to the legacy path
      // rather than reporting failure straight away.
    }
  }

  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  // Position off-screen so the textarea never flashes into view
  // between append and remove.
  ta.style.position = "fixed";
  ta.style.top = "0";
  ta.style.left = "0";
  ta.style.opacity = "0";
  ta.style.pointerEvents = "none";
  document.body.appendChild(ta);
  try {
    ta.select();
    ta.setSelectionRange(0, text.length);
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    document.body.removeChild(ta);
  }
}
