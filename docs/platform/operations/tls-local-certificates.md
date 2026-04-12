# TLS for Local Appliances

- **Status:** Implemented
- **Branch:** `copilot/complete-phase-1-image`
- **Components:** Go server, systemd service, image build

Local TLS certificate setup for velocity.report devices, using a first-boot local CA to provide HTTPS on `.local` domains without requiring internet access or a public certificate authority.

## Problem

velocity.report devices serve a web interface on the local network.
Browsers increasingly distrust plain HTTP — marking pages as insecure,
blocking mixed content, and restricting APIs like the clipboard. A user
visiting `http://velocity.local:8080` sees a warning label before they
see any data.

The device uses the `.local` mDNS hostname. No public Certificate
Authority will issue a certificate for `.local` domains — they are
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

## Chosen Approach: First-Boot Local CA

Each device generates its own CA and server certificate on first boot.
The user trusts the CA once and receives no further browser warnings.

### Certificate Chain

```
velocity.report Local CA (ECDSA P-256, 10-year validity)
  └── velocity.local server cert (ECDSA P-256, 825-day validity)
        SANs: DNS:velocity.local, DNS:localhost, IP:127.0.0.1
```

### Why a Local CA Rather Than a Bare Self-Signed Cert

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

### Certificate Generation

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

### nginx Reverse Proxy

nginx terminates TLS on port 443 and proxies all requests to the Go
server on `localhost:8080`. This keeps TLS concerns out of the
application, lets the Go server remain a plain HTTP server, and
preserves direct access on port 8080 for local processes (PDF
generator, diagnostics).

nginx serves the CA certificate at `GET /ca.crt` for browser trust
setup. After trusting the CA once, all future server certificate
renewals are accepted silently.

### Go Server

The Go server listens on `:8080` (plain HTTP). It has no TLS
awareness — all HTTPS is handled by nginx.

When running in development (no nginx), the server is accessed
directly at `http://localhost:8080`.

### Renewal

The 825-day server certificate will eventually expire. The generation
script runs on every boot and checks expiry, regenerating when less
than 24 hours remain. Because the same CA signs the new cert, browsers
that have trusted the CA do not need to re-accept anything.

The CA certificate has a 10-year validity. Replacing it requires the
user to re-trust — this is a deliberate tradeoff. Ten years is longer
than the expected field life of a Raspberry Pi 4.

## Alternatives Considered

| Option                                 | Decision                                                                        |
| -------------------------------------- | ------------------------------------------------------------------------------- |
| Let's Encrypt / ACME                   | Rejected: requires a public domain and internet access. Violates local-first.   |
| Tailscale HTTPS                        | Rejected: adds a dependency and requires Tailscale account. Not for all users.  |
| mkcert at build time                   | Rejected: bakes a shared CA into every image. One leak compromises all devices. |
| Reverse proxy (nginx, chosen approach) | Tradeoff: extra binary, extra config, extra failure mode.                       |
| Stay on plain HTTP                     | Rejected: browser warnings erode trust and block modern web APIs.               |
| Bare self-signed cert                  | Rejected: renewal forces user to re-accept. CA approach is strictly better.     |

## User Trust Setup

After first boot, the user:

1. Navigates to `https://velocity.local/` and accepts the initial
   browser warning (one time only if they skip CA installation).
2. Downloads the CA certificate at `https://velocity.local/ca.crt`.
3. Installs it in their OS/browser trust store:
   - **macOS:** Double-click `velocity-ca.crt` → Keychain Access →
     set "Always Trust" for SSL.
   - **Windows:** Double-click → Install Certificate → Local Machine →
     Trusted Root Certification Authorities.
   - **Linux/Firefox:** Settings → Certificates → Import →
     select `velocity-ca.crt`.
4. Subsequent visits show a green padlock. No further action needed
   until the CA expires (10 years).

## Security Considerations

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
