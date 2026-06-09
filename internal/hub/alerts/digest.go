// digest.go — RFC 0004 §"Digest" feature.
//
// Per-channel digest_window operator opt-in. FormatDigestBody
// renders the combined body that any single-shot channel can ship
// (ntfy / email / discord / webhook / slack / telegram) when the
// dispatcher flushes a buffered window.

package alerts

import (
	"fmt"
	"strings"
	"time"
)

// FormatDigestBody renders a multi-event digest as a human-facing
// string suitable for any single-shot channel (email body, Slack
// fallback, ntfy message). Each event becomes a bullet so the
// operator can scan; the header counts events + the window length
// so the channel-form copy can say "5 alerts in last 5m" and have
// the rendering back it up.
//
// We read DigestWindow off the first event (all events in one
// digest share the same window — the dispatcher groups by
// (channel, window) so a single flush is one window's worth of
// events). Pass a Notification with DigestWindow="" for
// "single-shot, no buffering" — the header still prints but
// says "in last 0s".
func FormatDigestBody(events []Notification) string {
	if len(events) == 0 {
		return ""
	}
	window := 0 * time.Second
	if d, err := ParseDigestWindow(events[0].DigestWindow); err == nil {
		window = d
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d alert(s) in last %s:\n", len(events), window)
	for _, n := range events {
		// Reuse the per-event FormatMessage so the wording stays
		// consistent with single-shot notifications. The resolved
		// branch keeps the "back below threshold" phrasing the
		// test pins.
		fmt.Fprintf(&b, "- %s\n", FormatMessage(n))
	}
	return strings.TrimRight(b.String(), "\n")
}
