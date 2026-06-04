# RFC 0004 — Notification quality bundle

- **Status**: Draft
- **Sprint**: Phase 8 Sprint 4
- **Effort**: 3 days (digest 1d + share link 1d + Slack 0.5d + multi-recipient 0.5d)

## Motivation

Four small wins that compound:

- **Digest / grouping** — an alert storm (e.g. ISP blip on the hub) currently produces N×channels notifications. One digest per channel per window is far more actionable.
- **Per-host share link** — sharing a single host's read-only view with a teammate without minting them a Lumen account.
- **Slack-native channel** — webhook works today but renders ugly; native Block Kit (color, fields, action button) is the Beszel-quality bar.
- **Multi-recipient email** — single recipient is a friction point operators routinely complain about.

None of these are large; together they make Lumen's notification surface feel polished.

## Scope

### Digest
**In**: Per-channel `digest_window` setting; dispatcher buffers events; flushes one combined notification per window.

**Out**: Cross-channel digest. Per-rule digest. Smart "this storm is over" detection.

### Per-host share link
**In**: Mint a token bound to one host with expiry. Public `/host/{token}` route renders read-only host detail (charts, recent events, no settings).

**Out**: Token rotation. Public share for multiple hosts in one link. Permanent links (expiry always required).

### Slack-native channel
**In**: New `slack` channel type. Block Kit message with color-coded severity, host/metric/value fields, "View in Lumen" action button.

**Out**: Slack OAuth (still uses Incoming Webhook URL). Threading. Reactions.

### Multi-recipient email
**In**: `to_addr` accepts comma-separated; SMTP `RCPT TO:` loop; per-address validation.

**Out**: BCC/CC distinction (all are RCPTs). Per-recipient body templating.

## Design

### Digest

`internal/hub/alerts/channels.go` — `ChannelConfig` gains:
```go
DigestWindow string `json:"digest_window,omitempty"` // "0", "1m", "5m", "15m", "1h"
```

`internal/hub/alerts/dispatcher.go` — when claiming a delivery row:
1. Parse channel's `DigestWindow`; `0` or empty = today's behaviour.
2. With a window: hold the row for up-to-window time, BUT also flush early once N≥10 rows accumulate to avoid silent drops.
3. On flush, render a combined `Notification` with `Message = "5 alerts in last 5m:\n- host=foo metric=cpu value=92...\n- ..."`. Pass it through normal type-specific dispatch.

Validation: `digest_window ∈ {"", "0", "1m", "5m", "15m", "1h"}`. Document trade-off (latency vs spam) in the channel form UI.

### Per-host share link

Migration:
```sql
CREATE TABLE host_share_tokens (
    token       TEXT PRIMARY KEY,         -- 32-byte random, base64url
    host_id     INTEGER NOT NULL,
    expires_at  DATETIME NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    created_by  INTEGER,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX idx_share_token_expires ON host_share_tokens(expires_at);
```

Endpoints:
- `POST /api/hosts/{id}/share` — body `{ttl_hours, label}`. Returns `{token, url, expires_at}`.
- `GET /api/hosts/{id}/shares` — list current valid + recently expired.
- `DELETE /api/share/{token}` — revoke.
- `GET /api/public/host/{token}` — unauthenticated; returns the host's read-only payload (current snapshot + last 1 h metrics + recent alert events). 404 on expired/revoked.

Frontend: Host detail header gets a "Share" button → opens popover (TTL select, label input, generated URL with copy button + current valid shares list). Public route at `/h/{token}` mirrors the existing Host detail component in a read-only mode (hide silence button + edit-layout button).

Background sweep: a goroutine deletes expired tokens once per hour (or piggybacks on the existing retention loop).

### Slack-native channel

`AllowedChannelTypes` gains `"slack"`. `validateChannel` for `slack` requires `url` matching `https://hooks.slack.com/services/...` (loose regex, document the format).

`dispatchSlack` (in `notify.go`):
```go
payload := map[string]any{
    "blocks": []any{
        map[string]any{
            "type": "header",
            "text": map[string]any{
                "type": "plain_text",
                "text": fmt.Sprintf("[%s] %s — %s", state, severity, ruleName),
            },
        },
        map[string]any{
            "type": "section",
            "fields": []any{
                map[string]any{"type": "mrkdwn", "text": "*Host:*\n" + host},
                map[string]any{"type": "mrkdwn", "text": "*Metric:*\n" + metric},
                map[string]any{"type": "mrkdwn", "text": fmt.Sprintf("*Value:*\n%.2f", value)},
                map[string]any{"type": "mrkdwn", "text": fmt.Sprintf("*Threshold:*\n%.2f", threshold)},
            },
        },
        map[string]any{
            "type": "context",
            "elements": []any{
                map[string]any{"type": "mrkdwn", "text": "Color: " + severityColor(severity)},
            },
        },
    },
}
```

Plus an `attachments[0].color` for the colored side bar (Slack legacy but still rendered) — green=resolved, orange=warning, red=critical.

### Multi-recipient email

`channels.go` validate `email` — `to_addr` may contain comma-separated addresses, each validated by `looksLikeEmail`.

`dispatchEmail` — split on comma, build the SMTP envelope with all `RCPT TO:` lines, single `DATA` with `To: a, b, c` header.

## Risks

| Risk | Mitigation |
|---|---|
| Digest hides a critical until window flushes | Document. Critical severity defaults to `digest_window=0` (no buffering) unless explicitly opted in by the operator. |
| Share token URL pasted in public channel = leak | Mandatory expiry; mandatory label; list of active shares in UI so admin can revoke. No "permanent share" knob. |
| Slack rate limits | Webhook URLs are per-channel; ~1 msg/s soft limit. Acceptable for monitoring scale. Document. |
| Multi-recipient email fan-out makes the SMTP server unhappy | Cap at 20 recipients per channel. Reject save above. |
| Public host endpoint leaks tag values via the snapshot envelope | Strip tags + system metadata from the public payload (same redaction as `/api/public/status`). |

## Testing

- `digest_test.go` — buffer N rows, flush triggers at window expiry; flush triggers early at N=10; resolved+firing for the same rule collapse cleanly.
- `share_token_test.go` — mint, fetch, expire, revoke; expired tokens swept.
- `slack_test.go` — fixture body POSTs match the Block Kit JSON snapshot.
- `email_multi_test.go` — 3 recipients → 3 RCPT TO + single DATA.

## Docs deliverables

- `docs/configure/notification-digest.md` — when to use, trade-off, recommended windows per severity.
- `docs/configure/host-share.md` — security model, recommended TTL, revocation.
- Updates to existing `docs/configure/alerts.md` for the new `slack` channel + multi-recipient email.
- CHANGELOG + ACTION_PLAN tick (these 4 items belong to the same Phase 8 sprint).

## Open questions

1. Should the digest preserve per-event metadata for the email/Slack body? Proposed: yes, render each event as a bullet so the operator can scan.
2. Public share for status page hosts — should the host need `public_visible=true` first? Proposed: no — share is per-link, orthogonal to the global status page. Document the difference.
3. Slack action button URL — does the hub know its public URL? Use `hubURLFromRequest()` at *test* time only; at dispatch time read from a new optional setting `hub.public_url` (else fall back to `https://localhost`).
