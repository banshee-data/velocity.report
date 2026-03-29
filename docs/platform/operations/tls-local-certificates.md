# TLS for Local Appliances

- **Status:** Implemented
- **Branch:** `copilot/complete-phase-1-image`
- **Components:** Go server, systemd service, image build

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

| Constraint                | Reason                                                     |
| ------------------------- | ---------------------------------------------------------- |
| No public CA for `.local` | `.local` is reserved for mDNS, excluded from public PKI    |
| No internet required      | Local-first appliance; may never see the public internet   |
| No reverse proxy          | Single-binary deployment; adding nginx is avoidable burden |
| No user SSH setup         | Target audience is neighbourhood advocates, not sysadmins  |
| Apple 825-day cert limit  | macOS/iOS reject server certs valid longer than 825 days   |
| Unique per device         | Compromising one device must not affect others             |

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

`velocity-generate-tls.sh` runs as `ExecStartPre` in the systemd
service unit. It is idempotent:

1. If `server.crt` exists and is valid for at least 24 hours → exit.
2. Otherwise, generate CA key + cert, server key + cert, clean up CSR.

Files are written to `/var/lib/velocity-report/tls/`:

| File         | Permissions | Purpose                           |
| ------------ | ----------- | --------------------------------- |
| `ca.key`     | 600         | CA private key (never leaves box) |
| `ca.crt`     | 644         | CA certificate (downloadable)     |
| `server.key` | 600         | Server private key                |
| `server.crt` | 644         | Server certificate                |

### Go Server

`internal/api/server.go` accepts `--tls-cert` and `--tls-key` flags.
When both files exist and load successfully:

- Serves HTTPS via `ListenAndServeTLS` with TLS 1.2 minimum.
- Serves the CA certificate at `GET /ca.crt` for browser trust setup.

When certificates are absent or fail to load:

- Falls back to plain HTTP. The server logs the reason.

This ensures the binary works in both production (RPi with certs) and
development (laptop, no certs, `--listen :8080`).

### Port

Default listen address changed from `:8080` to `:443`. The systemd
unit grants `CAP_NET_BIND_SERVICE` via `AmbientCapabilities` so the
`velocity` user can bind privileged ports without running as root.

For local development, pass `--listen :8080` to avoid needing
privilege.

### Renewal

The 825-day server certificate will eventually expire. The generation
script checks expiry on every service start and regenerates when less
than 24 hours remain. Because the same CA signs the new cert, browsers
that have trusted the CA do not need to re-accept anything.

The CA certificate has a 10-year validity. Replacing it requires the
user to re-trust — this is a deliberate tradeoff. Ten years is longer
than the expected field life of a Raspberry Pi 4.

## Alternatives Considered

| Alternative           | Rejected because                                                      |
| --------------------- | --------------------------------------------------------------------- |
| Let's Encrypt / ACME  | Requires a public domain and internet access. Violates local-first.   |
| Tailscale HTTPS       | Adds a dependency and requires Tailscale account. Not for all users.  |
| mkcert at build time  | Bakes a shared CA into every image. One leak compromises all devices. |
| Reverse proxy (nginx) | Extra binary, extra config, extra failure mode. Avoidable burden.     |
| Stay on plain HTTP    | Browser warnings erode trust and block modern web APIs.               |
| Bare self-signed cert | Renewal forces user to re-accept. CA approach is strictly better.     |

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
