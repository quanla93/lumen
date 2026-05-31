---
title: Connect external agents (no hub domain)
description: Reach a hub that has no public domain from agents on remote networks (other VPS, branch office, NAT). Tailscale, Cloudflare Tunnel, and self-signed/LAN-only paths.
sidebar:
  order: 4
---

Lumen agents push outbound. They never accept inbound connections, so
the network problem is one-way: **every agent must resolve and reach
`LUMEN_HUB_URL`**. If the hub has a public domain + TLS, you're done —
follow [Hub (Docker Compose)](/install/hub-compose/) or
[Hub (binary)](/install/hub-binary/) and skip this page.

This page is for the harder case: the hub has **no public domain**
(on-prem, NAT, no public IP, no Let's Encrypt cert) and agents live
**outside the hub's local network** (other VPS, branch office, customer
site). Three paths in order of recommendation:

| Path | When | Cost |
|---|---|---|
| **[Tailscale](#a-tailscale-recommended)** | Mixed personal/customer fleets, fastest setup | Free up to 100 devices, magic DNS |
| **[Cloudflare Tunnel](#b-cloudflare-tunnel)** | You control a domain on Cloudflare; want public URL without exposing hub | Free, needs a CF account |
| **[Self-signed cert + LAN IP](#c-self-signed-cert--lan-ip)** | Fully offline customer, agent reachable on LAN/VPN-of-customer's-choice | Manual CA install per agent |

The three are **not exclusive** — pick per-agent. A hub can run
Tailscale, Cloudflare Tunnel, and bare HTTPS at the same time.

---

## A. Tailscale (recommended)

[Tailscale](https://tailscale.com) is a zero-config mesh VPN built on
WireGuard. Hub and every agent join the same tailnet; agents reach the
hub at a stable `100.x.y.z` address with no port-forwarding, no DNS, no
public IP on either side.

### 1. Install Tailscale on the hub

Pick the form matching how the hub runs:

**Hub on bare Linux / VM**

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
# Auth in browser, then:
tailscale ip -4
# e.g. 100.64.10.5  — this is the address agents will use
```

**Hub via Docker Compose** — add a Tailscale sidecar that shares the
network namespace with the hub container:

```yaml
services:
  tailscale-hub:
    image: tailscale/tailscale:latest
    hostname: lumen-hub
    environment:
      TS_AUTHKEY: tskey-auth-CHANGE_ME
      TS_STATE_DIR: /var/lib/tailscale
      TS_USERSPACE: "false"
    volumes:
      - tailscale-hub-state:/var/lib/tailscale
      - /dev/net/tun:/dev/net/tun
    cap_add: [NET_ADMIN, SYS_MODULE]
    restart: unless-stopped

  lumen-hub:
    image: ghcr.io/quanla93/lumen-hub:latest
    network_mode: service:tailscale-hub   # share the tailnet IP
    depends_on: [tailscale-hub]
    # … rest of hub config

volumes:
  tailscale-hub-state:
```

Mint the `TS_AUTHKEY` at <https://login.tailscale.com/admin/settings/keys>.
Treat it like a password — anyone with it can join your tailnet.

### 2. Install Tailscale on each agent

Same one-liner on the agent VM:

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up --authkey tskey-auth-CHANGE_ME
```

If the agent itself runs in Docker, use the same sidecar pattern with
`network_mode: service:tailscale-agent`.

### 3. Point the agent at the hub's tailnet IP

In the agent's compose file or systemd env:

```yaml
LUMEN_HUB_URL: "http://100.64.10.5:8090"
```

Plain HTTP is fine here — Tailscale already encrypts the link with
WireGuard. No certs, no domain, no port-forwarding.

### MagicDNS variant

If MagicDNS is on in your tailnet, use the hostname instead of the IP:

```yaml
LUMEN_HUB_URL: "http://lumen-hub:8090"
```

Stable across hub-tailnet-IP changes.

> [Headscale](https://github.com/juanfont/headscale) is a self-hosted
> Tailscale control plane. Same shape; swap the install URL and login
> server. Recommended only if you already self-host one.

---

## B. Cloudflare Tunnel

Cloudflare's `cloudflared` opens an **outbound** connection from the
hub to Cloudflare's edge. Agents reach the hub at
`https://lumen.your-domain.com` — no port on the hub is exposed to the
internet, but you get a real domain + a real cert for free.

You need: a domain on Cloudflare (free plan is fine).

### 1. Create the tunnel

```bash
# On the hub machine
curl -L --output cloudflared.deb \
  https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb
sudo dpkg -i cloudflared.deb

cloudflared tunnel login           # opens browser, pick your domain
cloudflared tunnel create lumen
# → writes ~/.cloudflared/<UUID>.json
```

### 2. Route it

```bash
cloudflared tunnel route dns lumen lumen.your-domain.com
```

This creates a CNAME in your Cloudflare DNS pointing to the tunnel.

### 3. Run it

```bash
cloudflared tunnel run --url http://127.0.0.1:8090 lumen
# or as a service:
sudo cloudflared service install
```

Verify:

```bash
curl https://lumen.your-domain.com/healthz
# ok
```

### 4. Point agents at the public URL

```yaml
LUMEN_HUB_URL: "https://lumen.your-domain.com"
```

Cloudflare terminates TLS at the edge; the hub speaks plaintext to
`cloudflared` on `127.0.0.1`. Agents see a valid public cert; no extra
trust setup needed.

> Make sure the hub does **not** also listen on `0.0.0.0:8090` — bind
> to `127.0.0.1:8090` so the tunnel is the only reachable surface.

---

## C. Self-signed cert + LAN IP

Use this when the customer's environment is fully offline (no internet
out from the agent) or already has its own VPN/private network you want
to reuse. Lumen's agent uses Go's default HTTPS client — no insecure
flag, no SSL bypass. You must install your CA cert into the **system
trust store** on every agent machine.

### 1. Generate a CA and a hub cert

One-time on any workstation:

```bash
# Generate a self-signed CA (10-year)
openssl req -x509 -nodes -newkey rsa:4096 -days 3650 \
  -keyout lumen-ca.key -out lumen-ca.crt \
  -subj "/CN=Lumen Internal CA"

# Generate the hub server cert, signed by that CA.
# SAN must list every name/IP an agent will use to reach the hub.
cat >hub.cnf <<EOF
[req]
distinguished_name = dn
req_extensions = ext
prompt = no
[dn]
CN = lumen-hub.lan
[ext]
subjectAltName = @alt
[alt]
DNS.1 = lumen-hub.lan
DNS.2 = lumen-hub
IP.1  = 192.168.1.10
EOF

openssl req -nodes -newkey rsa:2048 -keyout hub.key -out hub.csr -config hub.cnf
openssl x509 -req -in hub.csr -CA lumen-ca.crt -CAkey lumen-ca.key -CAcreateserial \
  -out hub.crt -days 825 -sha256 -extensions ext -extfile hub.cnf
```

Replace `192.168.1.10` with the LAN IP the agents will use.

### 2. Put a TLS reverse proxy in front of the hub

The hub itself listens plaintext on `:8090`; terminate TLS in nginx or
Caddy. Caddy minimal config:

```caddy
{
  auto_https off
}

lumen-hub.lan:443 192.168.1.10:443 {
  tls /etc/caddy/hub.crt /etc/caddy/hub.key
  reverse_proxy 127.0.0.1:8090
}
```

Same shape as [Hub (binary) § Putting HTTPS in front](/install/hub-binary/#putting-https-in-front).

### 3. Install the CA on every agent

The agent does standard cert validation. Trust the CA system-wide:

```bash
# Debian/Ubuntu
sudo cp lumen-ca.crt /usr/local/share/ca-certificates/
sudo update-ca-certificates

# RHEL/Fedora/Alma
sudo cp lumen-ca.crt /etc/pki/ca-trust/source/anchors/
sudo update-ca-trust

# Alpine
sudo cp lumen-ca.crt /usr/local/share/ca-certificates/
sudo update-ca-certificates
```

For the Docker-Compose agent, mount the CA into the container:

```yaml
services:
  lumen-agent:
    image: ghcr.io/quanla93/lumen-agent:latest
    volumes:
      - ./lumen-ca.crt:/usr/local/share/ca-certificates/lumen-ca.crt:ro
    environment:
      LUMEN_HUB_URL: "https://lumen-hub.lan"
      # …
    # `update-ca-certificates` runs at image build, not at start, so
    # for Docker the simpler path is mounting into /etc/ssl/certs:
    # - ./lumen-ca.crt:/etc/ssl/certs/lumen-ca.crt:ro
```

### 4. Point the agent at the hub

Use the same name/IP that appears in the cert SAN:

```yaml
LUMEN_HUB_URL: "https://lumen-hub.lan"
# or
LUMEN_HUB_URL: "https://192.168.1.10"
```

If the agent log shows `x509: certificate signed by unknown authority`,
the CA isn't trusted — re-check step 3.

> **Why no `LUMEN_HUB_INSECURE_SKIP_VERIFY`?** Bypassing cert validation
> turns "HTTPS" into "HTTP with extra steps". The agent token rides in
> the `Authorization` header on every request; a MITM with an
> insecure-skip agent steals the token immediately. If you don't want
> to manage a CA, use **Tailscale** instead — Lumen explicitly does not
> ship that flag.

---

## Verify

After any of the three paths, the smoke test is the same:

```bash
# from the agent machine
curl -s "$LUMEN_HUB_URL/healthz"
# → ok

# then start the agent and watch logs
docker compose logs -f lumen-agent
# expect: msg=ingested cpu=… ram=… disk=…
```

The host should appear on the dashboard within one `agent_interval`.

---

## Troubleshooting

**`dial tcp: lookup lumen-hub.lan: no such host`**
: DNS not resolving. For Tailscale, enable MagicDNS. For LAN, add an
  entry to `/etc/hosts` on the agent, or use the IP directly.

**`x509: certificate signed by unknown authority`**
: Self-signed-cert path only. The CA isn't in the agent's system trust
  store. Re-run `update-ca-certificates` (or distro equivalent), and
  for Docker make sure the CA is mounted into `/etc/ssl/certs/` (the
  base image doesn't re-run `update-ca-certificates` at start).

**`x509: certificate is valid for X, not Y`**
: The hostname/IP the agent is using isn't in the cert's
  `subjectAltName`. Re-issue with all the names/IPs the agent will use.

**Cloudflare Tunnel works on a laptop but agent gets `530`**
: The tunnel name in DNS no longer matches the running tunnel
  (recreated tunnel?). Re-run `cloudflared tunnel route dns lumen
  lumen.your-domain.com`.

**Tailscale IP changes**
: It can after long downtime. Use MagicDNS hostnames in
  `LUMEN_HUB_URL`, not raw `100.x.y.z` IPs.

**Hub is also exposed publicly by accident**
: Bind to `127.0.0.1:8090` (Compose / binary) so only the tunnel or the
  Tailscale interface can reach it.
