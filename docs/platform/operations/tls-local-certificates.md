# TLS for local appliances — removed

- **Status:** Removed (graveyard). Superseded in v0.5.1.
- **Replaced by:** [tailscale-serve.md](./tailscale-serve.md) (HTTPS opt-in), [tailscale-remote-access.md](./tailscale-remote-access.md)
- **Rationale:** [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md)

This page used to describe the shipped local-TLS path: a first-boot local CA, a
self-signed server certificate for `velocity.local`, and nginx terminating HTTPS on
port 443. **That path no longer exists.** It was removed in v0.5.1.

## What ships now

- The Go server binds **`:80` directly** as the non-root `velocity` user (via
  `CAP_NET_BIND_SERVICE` in the systemd unit). No nginx, no reverse proxy.
- The LAN URL is **`http://velocity.local`** — plain HTTP, portless.
- There is **no bundled TLS**: no local CA, no self-signed certificate, no `/ca.crt`
  download, no browser warning to click through.
- **HTTPS is an opt-in via Tailscale Serve**, which provisions a genuine
  browser-trusted Let's Encrypt certificate for the device's `*.ts.net` name. See
  [tailscale-serve.md](./tailscale-serve.md) for the three-command quickstart.

## Why it was removed

The self-signed CA flow asked every first-time user to either click through a browser
warning or download and trust a `.crt` file — the worst dialog box in the project. The
device serves no PII by architecture (see [TENETS.md](../../../TENETS.md)), so plain
HTTP on a home LAN is an accepted trade-off, and the padlock becomes a Tailscale-shaped
opt-in for the operators who want it. Removing nginx and the CA also collapsed a
deployment stage and a recurring systemd unit, and trimmed the image.

The full reasoning, the threat-model trade-offs, and the original design it replaced
live in [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md). The
original local-CA design is preserved in this file's git history.
