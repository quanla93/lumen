---
title: Use the web UI
description: A guided tour of the Lumen dashboard, host detail page, settings, and common operator workflows.
sidebar:
  order: 1
---

The Lumen web UI is the fastest way to check fleet health, drill into one host, and manage agents. It is served by the hub, so there is no separate frontend service to deploy.

## Open the dashboard

After the hub is running, open the hub URL in your browser:

```text
http://<hub-host>:8090
```

On first run, Lumen asks you to create the admin account. After that, sign in with that account to reach the dashboard.

## Dashboard at a glance

The dashboard summarizes the current state of every host that has sent metrics to the hub.

| Area | What it means |
|---|---|
| WebSocket status | Shows whether the browser is receiving live updates from `/api/stream`. |
| Hosts | Total number of known hosts and how many are currently online. |
| Stale | Hosts that have not reported recently. Check the agent service or network if this is non-zero. |
| Average CPU / RAM | Fleet-wide average from the latest snapshots. |
| Search | Filters the host cards by host name. |
| Host cards | Show current CPU, RAM, disk, load, and recent activity for each monitored host. |

Click any host card to open its detail page.

## Add a host

1. Go to **Settings → Hosts**.
2. Enter a stable host name, for example `pve-01`, `lxc-postgres`, or `nas`.
3. Click **Create**.
4. Copy the `lum_...` token immediately. The token is only shown once.
5. Install or configure the agent on that machine with the copied token.

For installation commands, see [Add agents](/how-to/add-agents/).

## Read a host detail page

The host detail page combines live values with historical charts.

- **Status**: `Up` means the host is reporting now. `Stale` means the hub has seen it before but not recently. `Waiting` means no metrics have arrived yet.
- **System line**: shows the primary IP or hostname, operating system, and uptime when the agent reports them.
- **Time range**: switch charts between `1h`, `6h`, and `24h`.
- **Per-core CPU**: appears when the agent reports per-core data.
- **Charts**: CPU, RAM, disk, load average, network throughput, disk I/O, and temperature when available.
- **Containers**: appears when the agent can read Docker container state from the target host.

Charts refresh automatically. The live snapshot updates over WebSocket, while historical chart data refreshes periodically.

## Manage tokens and agents

Use **Settings → Hosts** for token and host lifecycle tasks.

| Action | Use it when |
|---|---|
| Rotate token | A token was lost, exposed, or copied incorrectly. The old token stops working immediately. |
| Delete host | You no longer want the hub to accept or display that host. |
| Create host | You are onboarding a new monitored machine. |

Do not create a new host just to update an agent binary. For updates, keep the same host and token; see [Update Docker agents](/how-to/update-agents/) if you run agents with Docker.

## Configure runtime behavior

The **Settings** page contains several sub-tabs:

- **Hosts**: create hosts, rotate tokens, and delete hosts.
- **Account**: view the signed-in user and change the password.
- **Runtime**: change the agent collection interval shown by the UI.
- **Retention**: control how long raw metrics are kept.
- **Downsample**: configure bucket size and hot/archive windows for lower-resolution history.
- **Logs**: preview log-management behavior and limits.

After saving runtime, retention, or downsample settings, verify that agents and charts still update at the expected cadence.

## Change language or theme

Use the header controls to switch language and theme. The dashboard is designed for both desktop and mobile widths, so the same web UI can be used from a phone browser.

## Troubleshooting

**The dashboard shows no hosts**
: Create a host in **Settings → Hosts**, then start an agent with that host's token. The first card appears after the agent's next collection tick.

**A host is stale**
: Check that the agent service is running, the hub URL is reachable from the target, and the token has not been rotated.

**WebSocket status is disconnected or error**
: Confirm the browser can reach the hub and that any reverse proxy forwards WebSocket upgrades to `/api/stream`.

**Charts have live values but little history**
: Wait for more collection ticks, then try a shorter range such as `1h`. If history remains empty, check retention and downsample settings.

**Container data is missing**
: The agent only reports containers when it can read Docker state on the target. For Docker agents, make sure the Docker socket is mounted read-only as documented in the install guide.
