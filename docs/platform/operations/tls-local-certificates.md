# TLS for local appliances

- **Status:** Implemented
- **Branch:** `copilot/complete-phase-1-image`
- **Components:** Go server, systemd service, image build

This page documents the TLS path that ships today: a first-boot local CA, a server certificate for `velocity.local`, and nginx terminating HTTPS on port 443.

Future simplification work is tracked in [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md). That plan is proposed, not current runtime behaviour.

Local TLS certificate setup for velocity.report devices, using a first-boot local CA to provide HTTPS on `.local` domains without requiring internet access or a public certificate authority.

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

## Chosen approach: first-boot local CA

Each device generates its own CA and server certificate on first boot.
The user may trust the CA once and receive no further browser warnings,
or use the browser's one-off certificate exception flow described in the
setup guide.

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

### nginx reverse proxy

nginx terminates TLS on port 443 and proxies all requests to the Go
server on `localhost:8080`. This keeps TLS concerns out of the
application, lets the Go server remain a plain HTTP server, and
preserves direct access on port 8080 for local processes (PDF
generator, diagnostics).

nginx serves the CA certificate at `GET /ca.crt` for browser trust
setup. After trusting the CA once, all future server certificate
renewals are accepted silently.

### Go server

The Go server listens on `:8080` (plain HTTP). It has no TLS
awareness: all HTTPS is handled by nginx.

When running in development (no nginx), the server is accessed
directly at `http://localhost:8080`.

The proposed direct-bind `:80` model lives in [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md). It has not landed in the shipped image or the default server runtime.

### Renewal

The 825-day server certificate will eventually expire. The generation
script runs on every boot and checks expiry, regenerating when less
than 24 hours remain. Because the same CA signs the new cert, browsers
that have trusted the CA do not need to re-accept anything.

The CA certificate has a 10-year validity. Replacing it requires the
user to re-trust: this is a deliberate tradeoff. Ten years is longer
than the expected field life of a Raspberry Pi 4.

## Alternatives considered

This table records the alternatives considered when the local-CA approach
was implemented. Later plans revisit some of these tradeoffs, but the
runtime described on this page is still the shipped system.

| Option                                 | Decision                                                                                                                                                                            |
| -------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Let's Encrypt / ACME (direct)          | Rejected: requires a public domain and internet access. Violates local-first.                                                                                                       |
| Tailscale HTTPS / Tailscale Serve      | Rejected for the shipped local-TLS path: adds a dependency and requires a Tailscale account. Revisited in [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md). |
| mkcert at build time                   | Rejected: bakes a shared CA into every image. One leak compromises all devices.                                                                                                     |
| Reverse proxy (nginx, chosen approach) | Tradeoff: extra binary, extra config, extra failure mode. Chosen for the shipped runtime because it keeps TLS out of the Go server and supports `https://velocity.local`.           |
| HTTP on LAN, HTTPS via Tailscale       | Rejected for the shipped runtime: browser warnings erode trust and block modern web APIs. Revisited in [deploy-nginx-removal-plan.md](../../plans/deploy-nginx-removal-plan.md).    |
| Bare self-signed cert                  | Rejected: renewal forces user to re-accept. CA approach was strictly better at the time, but both are now moot.                                                                     |

## User trust setup

After first boot, the user can either use the browser's certificate
exception flow for `https://velocity.local/`, or install the local CA to
remove future warnings.

Optional CA-install flow:

1. Navigate to `https://velocity.local/` and accept the initial browser warning.
2. Download the CA certificate at `https://velocity.local/ca.crt`.
3. Install it in the OS or browser trust store:

- **macOS:** Double-click `velocity-ca.crt` → Keychain Access →
  set "Always Trust" for SSL.
- **Windows:** Double-click → Install Certificate → Local Machine →
  Trusted Root Certification Authorities.
- **Linux/Firefox:** Settings → Certificates → Import →
  select `velocity-ca.crt`.

4. Subsequent visits show a trusted HTTPS connection until the CA expires (10 years).

## Security considerations

- **CA key never leaves the device.** It exists only at
  `/var/lib/velocity-report/tls/ca.key` with 600 permissions.
- **Each device has a unique CA.** Trusting one device's CA does not
  grant trust to any other device.
- **The CA can only sign for the SANs in its server certificate.**
  It is not configured as a general-purpose CA, but browsers treat any
  trusted CA as capable of signing for any domain. This is an inherent
  limitation of the trust model. Since the device is local-only and
  the CA key is restricted, the practical risk is low.
- **TLS 1.2 minimum.** TLS 1.0 and 1.1 are not accepted.
- **No client certificates.** The device is on a local network behind
  the user's own router. Network-level access control is sufficient.
- **Remote access is optional and separate.** Tools such as Tailscale may add their own identity and network controls, but they are not part of the local-TLS mechanism described here.
