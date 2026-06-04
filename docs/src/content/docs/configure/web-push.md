---
title: Web Push notifications
description: "Get browser notifications when Lumen alerts fire. Per-browser opt-in; works on desktop + Android. iOS Safari needs the PWA installed."
sidebar:
  order: 8
---

Web Push delivers alert notifications straight to your browser — system-level desktop or Android notifications, no Telegram bot, no email server. One Lumen "Web Push" channel can fan out to multiple browsers (your laptop + your phone), and dead subscriptions (browser uninstalled, permission revoked) are cleaned up automatically the next time the hub tries to send.

## Compatibility

- ✅ **Desktop Chrome, Edge, Firefox, Brave, Opera** — works out of the box.
- ✅ **Android Chrome / Firefox** — works out of the box.
- ✅ **iOS Safari 16.4+** — works **only after the user installs Lumen as a PWA** (Share → Add to Home Screen, then open from the home screen icon and subscribe from there). This is an Apple restriction, not a Lumen one.
- ❌ **iOS Safari without PWA install** — Apple does not expose the Push API to in-browser tabs.

In all browsers the subscribing user has to grant the notification permission. There is no "auto-enable" path — that's also a browser policy.

## Set up

1. Sign in as admin and open **Alerts → Channels → New channel**.
2. Set **Type** to **web_push**, give it a name (e.g. "browsers"), pick a min severity, and **Save**. The channel saves with zero browsers attached — you'll add them next.
3. Reopen the channel from the channel list. The config section now shows a **Subscribe this browser** button.
4. Click it. The browser asks for notification permission (this is the only time it asks); grant it.
5. The button now reports the saved subscription under "Subscribed browsers". Visit the same page from your phone (or another laptop) and repeat to fan out.
6. Wire the channel to a rule the same way you wire any other channel: open the rule, tick the box.

## What gets sent

Each fired alert produces one notification with:

- **Title** — `[FIRING|RESOLVED] <severity> — <rule name>`
- **Body** — the alert message (the same text the rule sets), or a default `<metric> on <host>: value=… threshold=…` when blank.
- **Tag** — `rule:<id>` so a re-fire replaces the previous notification instead of stacking.
- **Click action** — focuses an existing Lumen tab if one is open; otherwise opens a new one at `/`.

The service worker that handles the push lives at `/sw.js` and is bundled with the hub — there's nothing to install.

## VAPID — the cryptographic identity

Web Push requires the hub to sign every push with a stable ECDH key pair so the browser's push service trusts the sender. Lumen generates that pair **the first time you open the Web Push tab** and stores it in the `settings` table. **Do not** rotate `LUMEN_HUB_SECRET` after the pair is generated: the VAPID private key is AES-GCM-encrypted at rest with a key derived from `LUMEN_HUB_SECRET`, so rotating it locks Lumen out of its own key and forces every browser to re-subscribe.

The VAPID JWT also carries a `sub` claim — a contact for the push service to reach if delivery starts misbehaving. Lumen defaults to `mailto:admin@example.invalid`; override it via `PUT /api/alerts/web-push/subject` with a real `mailto:` or `https://` URL. (A future Settings UI will surface this; for now it's an API-only knob.)

## Manage subscriptions

Open **Alerts → Channels → <your web_push channel>** to see:

- One row per subscribed browser, labelled with the user agent the browser sent.
- The push service host (`fcm.googleapis.com` for Chrome/Edge, `updates.push.services.mozilla.com` for Firefox, etc.). Admin-only.
- A trash button to drop a subscription manually.

The hub also self-prunes: any push that comes back with HTTP **404 Gone** or **410 Gone** (the browser uninstalled or revoked the permission) deletes the row inline so subsequent fan-outs skip it.

## Troubleshooting

**"Notification permission denied" after I clicked Subscribe**
: The browser remembers a prior **deny**. Open the site's lock-icon menu → Permissions → Notifications → Reset. Then click Subscribe again.

**Subscribed OK but no notifications when alerts fire**
: Run a test ping from **Channels → … → Test**. If the test arrives and a real alert doesn't, the rule isn't wired to this channel (check the rule's channel list) or the alert is being suppressed by a `min_severity` filter on the channel. If the test also doesn't arrive, see the next item.

**Test ping returns OK but no notification appears**
: Some browsers throttle or hide pushes when the page is focused. Switch to another window and re-run the test. If still nothing, the OS-level notification permission may be off (macOS: System Settings → Notifications → Chrome/Firefox/Safari).

**iOS Safari shows "Subscribe" but does nothing**
: You're in a regular Safari tab, not the PWA. Add the site to the home screen, open it from the icon, then subscribe from there.

**"VAPID keys not generated yet"**
: The hub hasn't bootstrapped its pair. Open the Web Push tab once — the GET endpoint generates it on demand.

**Pushes stop after I changed `LUMEN_HUB_SECRET`**
: That's expected — the VAPID private key is encrypted with the old secret. Either restore the old `LUMEN_HUB_SECRET`, or accept the loss: delete the `webpush.*` rows in `settings`, then have every browser re-subscribe.

## Privacy

The push service (FCM, Mozilla, Apple) sees the **endpoint** + the **encrypted payload only**. They do not see the alert content; that's encrypted client-side using the browser's `p256dh` public key, decrypted only inside the user's browser. Lumen never sends user metrics through these services — only the small alert envelope.
