# TLS for local appliances

- **Status:** ~~Implemented~~ **Superseded** by [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md)
- **Branch:** `copilot/complete-phase-1-image`
- **Components:** Go server, systemd service, image build

> **Superseded.** The local-CA + nginx + self-signed TLS approach described below is being removed. The replacement: serve plain HTTP on `:80` for LAN access (`http://velocity.local`) and let users opt into HTTPS via Tailscale Serve (real Let's Encrypt cert on `<host>.<tailnet>.ts.net`). See [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md) for the rationale, the user-impact matrix, and the migration plan. The strikethrough sections below describe the decisions that have been reversed; they are kept for historical context and to make the drift visible.

~~Local TLS certificate setup for velocity.report devices, using a first-boot local CA to provide HTTPS on `.local` domains without requiring internet access or a public certificate authority.~~

## Problem

velocity.report devices serve a web interface on the local network.
Browsers increasingly distrust plain HTTP: marking pages as insecure,
blocking mixed content, and restricting APIs like the clipboard. A user
visiting `http://velocity.local:8080` sees a warning label before they
see any data.

The device uses the `.local` mDNS hostname. No public Certificate
Authority will issue a certificate for `.local` domains: they are
reserved for link-local multicast DNS and explicitly excluded from the
Web PKI trust model (CA/Browser Forum Baseline Requirements § 7.1.4.2).

## Constraints

| Constraint                | Reason                                                    |
| ------------------------- | --------------------------------------------------------- |
| No public CA for `.local` | `.local` is reserved for mDNS, excluded from public PKI   |
| No internet required      | Local-first appliance; may never see the public internet  |
| No user SSH setup         | Target audience is neighbourhood advocates, not sysadmins |
| Apple 825-day cert limit  | macOS/iOS reject server certs valid longer than 825 days  |
| Unique per device         | Compromising one device must not affect others            |

## ~~Chosen~~ Superseded approach: first-boot local CA

> The CA + per-device server cert was implemented and shipped, then reversed. The first-time-trust dialog (browser warning → download CA → install in OS trust store) was the worst onboarding step in the project, and the security benefit on a LAN-only appliance with no PII (per [TENETS.md](../../../TENETS.md)) did not justify it. The replacement plan is in [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md).

~~Each device generates its own CA and server certificate on first boot.
The user trusts the CA once and receives no further browser warnings.~~

### Certificate chain

```
velocity.report Local CA (ECDSA P-256, 10-year validity)
  └── velocity.local server cert (ECDSA P-256, 825-day validity)
        SANs: DNS:velocity.local, DNS:localhost, IP:127.0.0.1
```

### Why a local CA rather than a bare self-signed cert

A bare self-signed server certificate would technically work, but:

- Browsers show a full interstitial warning on every new certificate.
  With a CA, the user trusts the CA once and all future server certs
  (including renewals) are accepted silently.
- Certificate renewal without a CA forces the user to re-accept.
  With a CA, the renewal script signs a new server cert and the
  browser never notices.
- The CA certificate can be downloaded at `/ca.crt` and installed
  in the OS trust store. This is a one-time operation.

### Why ECDSA P-256

- Smaller keys and faster handshakes than RSA on ARM hardware.
- Universally supported by modern browsers and Go's `crypto/tls`.
- No dependency on external libraries.

## Implementation

### Certificate generation

`velocity-generate-tls.sh` runs as a systemd oneshot service
(`velocity-generate-tls.service`) before nginx starts on first boot.
It is idempotent:

1. If `server.crt` exists and is valid for at least 24 hours → exit.
2. If `ca.key`/`ca.crt` are missing or the CA is expiring → generate new CA.
3. Generate server key + cert signed by the (existing or new) CA, clean up CSR.

Files are written to `/var/lib/velocity-report/tls/`:

| File         | Permissions | Purpose                           |
| ------------ | ----------- | --------------------------------- |
| `ca.key`     | 600         | CA private key (never leaves box) |
| `ca.crt`     | 644         | CA certificate (downloadable)     |
| `server.key` | 600         | Server private key                |
| `server.crt` | 644         | Server certificate                |

### ~~nginx reverse proxy~~ (removed)

> nginx is being removed. The Go server now binds `:80` directly via `AmbientCapabilities=CAP_NET_BIND_SERVICE` in the systemd unit. See [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md).

~~nginx terminates TLS on port 443 and proxies all requests to the Go
server on `localhost:8080`. This keeps TLS concerns out of the
application, lets the Go server remain a plain HTTP server, and
preserves direct access on port 8080 for local processes (PDF
generator, diagnostics).~~

~~nginx serves the CA certificate at `GET /ca.crt` for browser trust
setup. After trusting the CA once, all future server certificate
renewals are accepted silently.~~

### Go server

~~The Go server listens on `:8080` (plain HTTP). It has no TLS
awareness: all HTTPS is handled by nginx.~~

The Go server binds `:80` directly in production (LAN), and `:8080` in development (`make dev-go`). No TLS in the Go path. HTTPS is delivered by Tailscale Serve when the user opts in.

~~When running in development (no nginx), the server is accessed
directly at `http://localhost:8080`.~~

### ~~Renewal~~ (no longer applies)

> No server cert, no CA, nothing to renew. Tailscale Serve manages its own Let's Encrypt cert lifecycle for users who opt in.

~~The 825-day server certificate will eventually expire. The generation
script runs on every boot and checks expiry, regenerating when less
than 24 hours remain. Because the same CA signs the new cert, browsers
that have trusted the CA do not need to re-accept anything.~~

~~The CA certificate has a 10-year validity. Replacing it requires the
user to re-trust: this is a deliberate tradeoff. Ten years is longer
than the expected field life of a Raspberry Pi 4.~~

## Alternatives considered

> The decision row for "Stay on plain HTTP" has been reversed; "Tailscale HTTPS" is now the recommended (opt-in) HTTPS path rather than the rejected one. The earlier reasoning is preserved with strikethroughs to make the drift legible.

| Option                                                  | Decision                                                                                                                                                                                                                                                                                                                             |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Let's Encrypt / ACME (direct)                           | Rejected: requires a public domain and internet access. Violates local-first.                                                                                                                                                                                                                                                        |
| ~~Tailscale HTTPS~~ Tailscale Serve                     | ~~Rejected: adds a dependency and requires Tailscale account. Not for all users.~~ **Now the recommended opt-in HTTPS path.** Real Let's Encrypt cert via the user's tailnet, no warnings, no bundled state. Not enabled by default; documented in [tailscale-remote-access-guide.md](../../plans/tailscale-remote-access-guide.md). |
| mkcert at build time                                    | Rejected: bakes a shared CA into every image. One leak compromises all devices.                                                                                                                                                                                                                                                      |
| ~~Reverse proxy (nginx, chosen approach)~~              | ~~Tradeoff: extra binary, extra config, extra failure mode.~~ **Removed** — see [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md).                                                                                                                                                                            |
| ~~Stay on plain HTTP~~ HTTP on LAN, HTTPS via Tailscale | ~~Rejected: browser warnings erode trust and block modern web APIs.~~ **Now the chosen approach.** No browser features we use are gated on secure context; LAN eavesdropping reveals only aggregate telemetry already shown on a wall display; PII is excluded by tenet.                                                             |
| Bare self-signed cert                                   | Rejected: renewal forces user to re-accept. CA approach was strictly better at the time, but both are now moot.                                                                                                                                                                                                                      |

## ~~User trust setup~~ (removed)

> No CA installation, no browser warning. New flow:
>
> 1. Navigate to `http://velocity.local/` (port 80, no TLS). No warning, no padlock.
> 2. _(Optional)_ For HTTPS over WAN, install Tailscale and run `tailscale serve --bg http://localhost:80`. Visit `https://<host>.<tailnet>.ts.net/` from any peer; valid Let's Encrypt cert.
>
> Field devices that already have a velocity-CA installed in their browser trust stores can leave it; it does no harm and will stop being used when the device upgrades past nginx.

~~After first boot, the user:~~

~~1. Navigates to `https://velocity.local/` and accepts the initial
browser warning (one time only if they skip CA installation).~~
~~2. Downloads the CA certificate at `https://velocity.local/ca.crt`.~~
~~3. Installs it in their OS/browser trust store:

- **macOS:** Double-click `velocity-ca.crt` → Keychain Access →
  set "Always Trust" for SSL.
- **Windows:** Double-click → Install Certificate → Local Machine →
  Trusted Root Certification Authorities.
- **Linux/Firefox:** Settings → Certificates → Import →
  select `velocity-ca.crt`.~~
  ~~4. Subsequent visits show a green padlock. No further action needed
  until the CA expires (10 years).~~

## Security considerations

> The bullets below described the security posture under the local-CA approach. They are mostly inapplicable now. The summary of the new posture: LAN traffic is plain HTTP and visible to anyone on the LAN; the device serves no PII per tenet; HTTPS is available to any user who installs Tailscale.

- ~~**CA key never leaves the device.** It exists only at
  `/var/lib/velocity-report/tls/ca.key` with 600 permissions.~~
- ~~**Each device has a unique CA.** Trusting one device's CA does not
  grant trust to any other device.~~
- ~~**The CA can only sign for the SANs in its server certificate.**
  It is not configured as a general-purpose CA, but browsers treat any
  trusted CA as capable of signing for any domain. This is an inherent
  limitation of the trust model. Since the device is local-only and
  the CA key is restricted, the practical risk is low.~~
- ~~**TLS 1.2 minimum.** TLS 1.0 and 1.1 are not accepted.~~
- **No client certificates.** The device is on a local network behind
  the user's own router. Network-level access control is sufficient.
- **No TLS on LAN by default.** Plain HTTP on `:80`. LAN attacker can read or MITM dashboard traffic. Threat accepted: telemetry is non-PII.
- **HTTPS path is Tailscale-mediated.** When enabled, end-to-end via WireGuard + a real Let's Encrypt cert; no per-device CA, no shared secret in the image.
