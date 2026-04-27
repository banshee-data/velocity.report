# Drop nginx; HTTP-on-:80 locally; Tailscale Serve for HTTPS

- **Document Version:** 1.0
- **Status:** Proposed
- **Layers:** Image build, systemd, Go server, docs
- **Canonical:** [tls-local-certificates.md](../platform/operations/tls-local-certificates.md)
- **Related:** [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md), [tls-local-certificates.md](../platform/operations/tls-local-certificates.md), [tailscale-remote-access-guide.md](./tailscale-remote-access-guide.md)
- **Supersedes:** the local-CA + nginx + self-signed TLS approach in [tls-local-certificates.md](../platform/operations/tls-local-certificates.md)

---

## Context

Today nginx exists for one reason: TLS termination on `https://velocity.local`. The Go server already does everything else (routing, static files, SSE). The self-signed CA flow asks every first-time user to either click through a browser warning or download and trust a `.crt` file — the worst dialog box in the project.

Removing nginx + the CA collapses a deployment stage and a recurring systemd unit. The cost is paid in user expectations: HTTP not HTTPS unless the user opts into Tailscale.

This plan is consistent with the existing rejection of Tailscale Serve as a _bundled_ HTTPS solution in [tls-local-certificates.md](../platform/operations/tls-local-certificates.md). The change here is that we also stop bundling self-signed TLS — we ship plain HTTP on the LAN and treat HTTPS as a Tailscale-shaped extra.

## Proposed architecture

```
Before:                                       After:
Browser ─https─> nginx:443 ─> go:8080         Browser ─http──> go:80                                (LAN)
                                              Browser ─https─> tailscale:443 ─> go:80               (Tailnet)
```

- Go server binds `:80` directly on the Pi via `AmbientCapabilities=CAP_NET_BIND_SERVICE` in the systemd unit. No root, no `setcap` on the binary, survives upgrades. Same `velocity` system user.
- `:8080` is removed as the production default. The `--listen` flag remains for `make dev-go`.
- `velocity.local` mDNS hostname unchanged. Local URL becomes `http://velocity.local` — portless, as desired.
- `https://<host>.<tailnet>.ts.net` works via `tailscale serve --bg http://localhost:80`. Real Let's Encrypt cert. No warnings.

## What the user loses without Tailscale

| Concern                                     | Today (nginx)                   | After (HTTP-only)                  | Severity                                                                 |
| ------------------------------------------- | ------------------------------- | ---------------------------------- | ------------------------------------------------------------------------ |
| Browser padlock on LAN                      | Yellow → green after CA install | None (plain HTTP)                  | Cosmetic + perception                                                    |
| Site-permission APIs (camera/mic/clipboard) | Granted under HTTPS origin      | Some APIs gated to secure contexts | None today — we use no secure-context APIs                               |
| SSE / `/events`                             | Works under TLS                 | Works under HTTP                   | None                                                                     |
| Eavesdropping on LAN                        | Mitigated                       | Visible to anyone on the LAN       | Real but accepted: device serves no PII per [TENETS.md](../../TENETS.md) |
| Tampering on LAN                            | Mitigated                       | A LAN attacker can MITM            | Real; same threat model as any unencrypted LAN service                   |
| Deep links from HTTPS sites                 | Allowed                         | Mixed-content blocked              | None — no integrations require this                                      |
| Service workers / PWA install               | Available                       | Not available over plain HTTP      | None today — no SW or PWA                                                |

Honest framing for `setup.md`: **HTTPS is a Tailscale opt-in.**

## Portless local URL

Yes — `http://velocity.local` (no port) is the goal and is the deliverable of binding `:80`. mDNS already resolves the hostname; nothing else changes.

## Tailscale Serve — bundle or not

Don't bundle. Tailscale Serve requires the user to log into their tailnet, which we cannot do during image build. A doc page with the three commands (`tailscale up`, `tailscale serve --bg http://localhost:80`, `tailscale serve status`) is the right shape. This matches the existing rejection in [tls-local-certificates.md](../platform/operations/tls-local-certificates.md).

## Files to change

- **Delete** [image/stage-velocity/03-velocity-config/files/velocity-nginx.conf](../../image/stage-velocity/03-velocity-config/files/velocity-nginx.conf)
- **Delete** [image/stage-velocity/03-velocity-config/files/velocity-generate-tls.sh](../../image/stage-velocity/03-velocity-config/files/velocity-generate-tls.sh)
- **Delete** `image/stage-velocity/03-velocity-config/files/velocity-generate-tls.service`
- **Edit** [image/stage-velocity/03-velocity-config/00-run.sh](../../image/stage-velocity/03-velocity-config/00-run.sh) — drop nginx package install, drop `velocity-generate-tls` enable, drop `nginx` enable; remove `/var/lib/velocity-report/tls/` directory creation
- **Edit** [image/stage-velocity/03-velocity-config/files/velocity-report.service](../../image/stage-velocity/03-velocity-config/files/velocity-report.service) — change `ExecStart` to listen on `:80`; add `AmbientCapabilities=CAP_NET_BIND_SERVICE` and `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`; keep `User=velocity`
- **Edit** [cmd/radar/radar.go](../../cmd/radar/radar.go) (line ~50) — change default `--listen` from `:8080` to `:80` for production builds; `make dev-go` continues to pass `--listen :8080`
- **Edit** [public_html/src/guides/setup.md](../../public_html/src/guides/setup.md) — replace the "browser warning / install CA" section with a one-paragraph "URL is `http://velocity.local`; for HTTPS install Tailscale" block
- **Edit** [docs/platform/operations/tls-local-certificates.md](../platform/operations/tls-local-certificates.md) — convert to a graveyard entry pointing to [tailscale-remote-access-guide.md](./tailscale-remote-access-guide.md), or delete and run `/fix-links` for inbound references
- **New** `docs/platform/operations/tailscale-serve.md` — three-command quickstart; note that this is user-driven, not bundled
- **Edit** [image/stage-velocity/03-velocity-config/files/velocity-aliases.sh](../../image/stage-velocity/03-velocity-config/files/velocity-aliases.sh) — drop any nginx-related aliases (verify during implementation)
- **Search-and-update** README, ARCHITECTURE, CHANGELOG references to `https://velocity.local`, port 443, nginx, the self-signed CA flow

## Verification

1. Burn image, boot Pi. From a LAN host: `curl -sSf http://velocity.local/ -o /dev/null` returns 200.
2. On the Pi: `ss -ltnp` shows `velocity-report` on `:80`. No nginx. No `:443`. No `:8080` in production.
3. `journalctl -u velocity-report` shows clean start; binding `:80` succeeds under `User=velocity`.
4. From a Tailnet peer (after `tailscale serve --bg http://localhost:80` on the device): `curl https://<host>.<tailnet>.ts.net/api/radar_stats` returns 200 with a valid Let's Encrypt cert.
5. SSE smoke test: open `/events` in a browser, leave it for 90 seconds, confirm the stream stays open. (Go has no nginx-style buffering middleware in the path, but verify.)
6. Image-size diff: `du -sh` before/after on the image stage; expect ~30 MB drop from removing nginx + the openssl cert flow.

## Greenfield assumption

This change ships in the first mass-release image. There are no in-field installs to migrate. No pre-existing user-installed CAs to clean up. No "old image still running nginx" upgrade path to detect. The image bake produces the new shape directly.

## Sequencing

Land **before** [deploy-versioned-binary-plan.md](./deploy-versioned-binary-plan.md). Smaller, no Go refactor; establishes the "no nginx, port 80, HTTP-only by default" baseline that the binary refactor can then assume.
