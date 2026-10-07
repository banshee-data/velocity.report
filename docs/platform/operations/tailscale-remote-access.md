# Tailscale remote access

Operator and contributor reference for the Tailscale integration on the
Pi image. For the click-through enrolment flow that an end user sees,
read the user guide at
[public_html/src/guides/tailscale.md](../../../public_html/src/guides/tailscale.md)
instead.

## What's vendored

The Pi image contains neither Tailscale nor a Tailscale apt repository.
The first time the operator opts in through the web UI, the binary
downloads the project's pinned static Tailscale payload, verifies its
SHA-256, installs it under `/opt/velocity-report/tailscale/`, links
`tailscale` and `tailscaled` into `/usr/local/bin`, and writes its own
`tailscaled.service`. Nothing reaches out to Tailscale's coordination
server until then. This is a privacy tenet, not a configuration detail:
the image is published publicly and may run in environments where
outbound traffic is sensitive, so the default state is "absent until
asked for." Disabling stops and **masks** the unit.

| Concern             | Where it lives                                                                                                                |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Pinned payload      | [internal/tailscaleinstall/installer.go](../../../internal/tailscaleinstall/installer.go)                                     |
| systemd unmask flow | [internal/cmd/device/tailscale.go](../../../internal/cmd/device/tailscale.go)                                                 |
| sudoers grant       | [image/stage-velocity/03-velocity-config/00-run.sh](../../../image/stage-velocity/03-velocity-config/00-run.sh) (lines 54–57) |
| Manager / IPN bus   | [internal/tailscale/manager.go](../../../internal/tailscale/manager.go)                                                       |
| HTTP endpoints      | [internal/api/server_tailscale.go](../../../internal/api/server_tailscale.go)                                                 |
| Web UI              | [web/src/routes/settings/+page.svelte](../../../web/src/routes/settings/+page.svelte)                                         |

## Trust model

Three boundaries, each enforced by something other than convention:

1. **Installing and the daemon lifecycle require root** (install,
   unmask/enable/start/stop/mask). The non-root `velocity` service user
   is granted exactly three sudo actions, by literal argv with no
   wildcards:

   ```
   /usr/local/bin/velocity device tailscale install
   /usr/local/bin/velocity device tailscale enable-tailscaled
   /usr/local/bin/velocity device tailscale disable-tailscaled
   ```

2. **Daemon configuration runs as `velocity`** over the local API
   socket. After the daemon starts, `enable-tailscaled` runs
   `tailscale set --operator=velocity`, which authorises the service
   user to drive `tailscaled` without root for everything else
   (login, prefs, serve config, status).

3. **Inbound HTTP authorisation follows the selected profile.** `off` and
   `on` preserve the historical LAN/loopback administration policy. `hardened`
   limits anonymous LAN/loopback callers to aggregate viewing and existing ordinary
   PDF downloads. Tailscale grants confer routine operations; maintenance and access
   management remain OS-local (see [Hardened profile](#hardened-profile)).

## Enable / disable flow

In compatibility profiles, when the operator toggles Tailscale on in Settings:

1. The web UI POSTs `/api/tailscale/enable`. The Go server runs
   `sudo /usr/local/bin/velocity device tailscale install`, which
   installs the pinned payload if it is not already current, then
   `sudo /usr/local/bin/velocity device tailscale enable-tailscaled`,
   which unmasks, enables, and starts the service, waits up to 15 s for
   `/var/run/tailscale/tailscaled.sock` to appear, and runs
   `tailscale set --operator=velocity`.
2. The manager subscribes to the IPN bus, calls
   `StartLoginInteractive`, and caches the resulting `BrowseToURL`
   for up to 5 minutes.
3. The web UI long-polls `/api/tailscale/status?v=<version>&wait=<secs>`,
   which returns as soon as the status changes, and renders the URL
   plus a QR code. The user opens it in
   their tailnet account and approves the device.
4. Once the node reaches `Running`, `applyDevicePolicy` runs **once
   per Enable**:
   - `RunSSH=true` via `EditPrefs` → Tailscale SSH on.
   - `SetServeConfig` with a web handler at `https://<fqdn>:443/`
     proxying to the server's own port on `127.0.0.1` (`:80` on the
     Pi image) → web UI on the tailnet.

   These two steps record their results independently
   (`sshOK`/`sshErr`, `serveOK`/`serveErr`) so the Settings page can
   surface partial failures.

Disable reverses it: clear the serve config, set `WantRunning=false`,
then `disable-tailscaled` runs `stop`, `disable`, and `mask` in that
order. The node identity stays on disk; toggling on again resumes
the same membership without a fresh login.

## What the operator still has to do

The binary carries the pinned daemon and the lifecycle plumbing. It
does **not** carry anything that has to live in your tailnet account:

- A Tailscale account and tailnet (free personal plan is fine).
- HTTPS certificates enabled at
  [login.tailscale.com/admin/dns](https://login.tailscale.com/admin/dns),
  if you want the served web UI to be `https://`. The device
  fetches its certificate automatically on first connect.
- Optional but recommended: a `tag:velocity-report` tag and ACL
  rules. The default tailnet policy lets every member reach every
  member, which is fine for a single-user setup but coarse for
  shared tailnets.
- Optional: `velocity.report/cap/*` grants if you want to split read
  from write access across users.

There is no headless / auth-key path in the web UI. The flow is
always interactive login. If you need headless enrolment for fleet
deployment, run `sudo velocity device tailscale install`,
`sudo velocity device tailscale enable-tailscaled` and
`sudo tailscale up --auth-key=…` on the Pi over SSH _before_ using
the web UI; the manager picks up the running daemon on its first
poll.

Tailscale is pinned and updated with velocity.report releases; it
does not use apt or Tailscale auto-update. A released version
refreshes on the next opt-in or console
`velocity device tailscale install`.

## Capability grants

By default any tailnet peer that can reach the Pi has full admin
access. To split read from write, use
[application capability grants](https://tailscale.com/kb/1324/grants-app-capabilities).

velocity-report recognises two cap names:

- `velocity.report/cap/view` — the read-only endpoints listed in
  `viewRoutes` (`internal/api/server.go`): `/api/events`,
  `/api/radar_stats`, the three `/api/charts/` SVGs, `/api/timeline`,
  `/api/config`, `/api/capabilities`, `/api/version` and a few more;
  plus `GET` on sites, site configuration periods and reports
  (`viewRoutesGetOnly`). Every other route, the LiDAR API included,
  needs admin: an unlisted route defaults to admin.
- `velocity.report/cap/admin` — full access (implies view).

Add a grant to your tailnet policy:

```hujson
"grants": [
  {
    "src": ["autogroup:member"],
    "dst": ["tag:velocity-report"],
    "app": { "velocity.report/cap/view": [{}] }
  },
  {
    "src": ["group:operators"],
    "dst": ["tag:velocity-report"],
    "app": { "velocity.report/cap/admin": [{}] }
  }
]
```

### Enforcement modes

The `-ts-cap-enforcement` flag on `velocity-report` controls whether
the gate is active:

| Mode       | Behaviour                                                                                                                                                                      |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `off`      | (default) Capability checks disabled. Every reachable peer is admin. Use this until grants are validated.                                                                      |
| `on`       | Enforce caps for tailnet-sourced requests. LAN and loopback are still admin. Flip to `on` after grants are wired.                                                              |
| `hardened` | Anonymous LAN/loopback aggregate/PDF reads; explicit permissions for routine administration; OS-local access management and maintenance. Requires the dedicated Serve backend. |

### Compatibility trust model (`on`)

The gate is a **default-deny wrapper** installed around the entire HTTP
mux. Anything not on a small explicit allowlist requires a cap; routes
added later by other packages (LiDAR, debug, db admin, etc.) inherit
the default-deny policy automatically.

Source classification rules:

- **Loopback `RemoteAddr` + `X-Forwarded-For` a tailnet address** →
  trust XFF; this is how `tailscale serve` forwards tailnet requests
  to the local HTTP server. Subject to grants.
- **Loopback `RemoteAddr` + `X-Forwarded-For` any other address** →
  forwarded for someone outside the tailnet, as serve does for a
  Funnel client. Refused: 403 `{"error":"untrusted_forward"}`.
- **`Tailscale-Funnel-Request` header** → serve accepted the request
  from the public internet. Refused: 403 `{"error":"funnel_request"}`.
  serve strips any copy a client sends.
- **Loopback with no XFF** → host-local (the Go server itself,
  `velocity device`, a local `curl`). Treated as admin.
- **Non-loopback `RemoteAddr` in the tailnet range** → a peer
  connecting to the node's tailnet address directly (the Pi image
  listens on `:80` on every interface). Subject to grants.
- **Any other non-loopback `RemoteAddr`** → LAN. `X-Forwarded-For` is
  **ignored** entirely so a LAN attacker who can reach the server
  directly cannot forge a tailnet identity by setting the header.
  Treated as admin.

Failure modes:

- The daemon authoritatively reporting "no such peer" → 403
  `{"error":"unknown_peer"}`.
- A transient lookup error (socket down, timeout) → 503
  `{"error":"peer_lookup_unavailable"}` with `Retry-After`. Successful lookups
  are cached for at most five seconds, measured from lookup start. There is no
  ten-minute stale-identity fallback. Concurrent requests share one in-flight
  lookup per peer; an older completion cannot restore a revoked identity.
- A peer with no caps → 403 `{"error":"missing_cap","required":"…"}`.

### Compatibility caveats

- **Non-tailnet sources are always admin.** Loopback (`127.0.0.1`,
  `velocity device`) and LAN sources bypass the cap check entirely.
  Gate LAN access at the network layer (firewall, VLAN) if that
  isn't acceptable.
- **`/api/tailscale/status` is reachable by every tailnet peer** so an
  operator with a botched grant policy can still see the daemon state
  and recover. With enforcement on, a peer without a view grant gets
  the state fields alone (`redacted: true`), not the tailnet's names,
  peer count or error text, and only an admin gets a pending login URL,
  which enrols the device for whoever opens it.
- **Grants only protect the velocity-report HTTP API.** They do not
  cover Tailscale SSH, the gRPC visualiser stream, or any other
  port. Use ACL rules for those.
- **Some tailnet paths arrive looking local.** A subnet router that
  source-NATs delivers peer traffic from its own LAN address, and
  `tailscale serve --tcp` forwards without `X-Forwarded-For`; both are
  treated as LAN or host traffic, hence admin. The gate covers
  `tailscale serve` HTTP and direct connections to the node's tailnet
  address only.
- **Recovery from a misconfigured ACL relies on the LAN bypass.**
  Once you've validated grants on a test peer, flip
  `-ts-cap-enforcement=on` and restart.

## Hardened profile

Select `--ts-cap-enforcement=hardened` explicitly. Existing installations stay on
`off` by default; `on` remains the compatibility profile. No failed hardened
configuration silently falls back to either mode.

| Caller                                  | Aggregate charts and site display   | Existing PDFs                       | Settings and report generation                      | Raw/source exports | Maintenance or Tailscale enrolment |
| --------------------------------------- | ----------------------------------- | ----------------------------------- | --------------------------------------------------- | ------------------ | ---------------------------------- |
| Anonymous LAN or ordinary loopback HTTP | Yes                                 | Yes                                 | No                                                  | No                 | No                                 |
| Tailscale `view` grant                  | Yes                                 | Yes                                 | No                                                  | No                 | No                                 |
| Tailscale `admin` grant                 | Yes                                 | Yes                                 | Yes                                                 | Yes                | No                                 |
| OS-authorised operator                  | Use the application policy for HTTP | Use the application policy for HTTP | Use authenticated Tailscale HTTP or local CLI tools | Local CLI tools    | Local CLI/console only             |

Ordinary PDFs are deliberately disclosed to the LAN, including their embedded
content. This profile has no per-report privacy designation. Future private reports
must be withheld from the anonymous audience, not merely assigned a native user role.
Source ZIPs, raw events and LiDAR evidence require `data:export`. Site display responses
omit contact/surveyor/address fields; report metadata omits local file paths. Status
responses hide enrolment URLs from every HTTP caller and detailed tailnet metadata
from viewers. `/api/access` returns the caller's operation permissions without a
human identity claim; the frontend gates page mounting and protected actions on them.

Direct Tailscale requests use verified `WhoIs` grants. The LAN listener rejects all
forwarding/capability identity headers and refuses public source addresses. A subnet
router which source-NATs receives only anonymous LAN permissions. Ordinary loopback
traffic also receives only that reading policy.

Serve uses a **separate loopback backend**, `--ts-serve-listen=127.0.0.1:8082` by default.
The manager requests forwarding of `velocity.report/cap/view` and `velocity.report/cap/admin`
via `AcceptAppCaps`; the daemon must support [Serve app capability forwarding](https://tailscale.com/docs/reference/examples/serve#forward-app-capabilities-to-a-local-service)
(Tailscale 1.92 or later). The backend accepts one bounded JSON capability header,
including the daemon's MIME encoding, and refuses missing, duplicate or malformed
claims. It does not infer authority from forwarded IP addresses. Processes able to
connect locally to this backend belong to the OS trust boundary: firewall/container
forwarding must never expose it to another host. Funnel is disabled in managed Serve
configuration and refused by the application.

Authenticated browser requests must have a same-origin `Origin` when supplied;
Cross-site and same-site browser fetches are refused. Direct authenticated access accepts literal
private/tailnet/loopback IP hosts or `localhost`, preventing a public or mDNS
hostname from acquiring authority through DNS rebinding. Use Serve's HTTPS hostname
for MagicDNS browsing. Command-line clients may omit browser headers. Native cookies
and sessions are future work; adding them must preserve origin and credential checks.

Full gRPC requires literal loopback or `localhost` in every profile, including
standalone publishers; it has no authenticated remote mode. Hardened startup also
requires the separate LiDAR HTTP listener to be local, even when currently disabled. Main-mux LiDAR routes carry operation metadata. Remote debug,
SQL, backup and maintenance remain disabled in this profile, irrespective of admin
grants or legacy inner debug gates. Full gRPC stays local: playback controls mutate
shared state and replay completion can return the pipeline to live input. Recording
RPCs are unimplemented and are no longer advertised as supported.

### OS bootstrap, persistent activation and recovery

Retain a working console or OS-authorised SSH session before changing policy. The
`velocity device tailscale` commands manage installation and daemon lifecycle, not
HTTP permissions or enrolment. From that session:

```bash
sudo /usr/local/bin/velocity device tailscale install
sudo /usr/local/bin/velocity device tailscale enable-tailscaled
sudo /usr/local/bin/tailscale up
```

Approve the device and configure the grants in your own tailnet. Check the installed
daemon with `tailscale version`. Preserve the existing unit's database, serial and
LiDAR flags when applying a systemd override. For the stock image's command:

```ini
[Service]
ExecStart=
ExecStart=/usr/local/bin/velocity-report --listen :80 --db-path /var/lib/velocity-report/sensor_data.db --ts-cap-enforcement=hardened --ts-serve-listen=127.0.0.1:8082
```

Save with `sudo systemctl edit velocity-report`, then run
`sudo systemctl daemon-reload` and `sudo systemctl restart velocity-report`.
The first daemon `Running` event reconciles the dedicated Serve target and accepted
capabilities, including after a server restart. Check journal output for
`auth: operation enforcement armed (mode=hardened)` and the Serve publication result.
Unsupported daemon versions refuse the new Serve configuration; direct lookup and LAN permissions
retain their hardened checks. Missing adapters or invalid listeners refuse startup.
A backend bind failure stops the server with a failing exit status for systemd.

If grants, Serve or Tailscale fail, repair them from the retained OS session; local
HTTP does not become admin during recovery. To roll back intentionally, edit the
same override to `--ts-cap-enforcement=off`, reload and restart. This restores the
legacy trust policy, so make that choice explicitly. Stop remote access through
`sudo /usr/local/bin/velocity device tailscale disable-tailscaled` when required.

Local regression tests do not establish successful appliance deployment. Before
recommending hardened activation on v0.5.1, exercise persistent activation, restart,
rollback, viewer/admin parity over direct Tailscale and Serve, outages, revocation,
PDF/ZIP classification and alternate-listener refusal on a Pi and live tailnet.

## Hostname and MagicDNS

The MagicDNS name comes from the daemon, not from the image. The
daemon picks up the system hostname at first `tailscale up`, then
the coordination server owns it. To set a specific hostname, run
`sudo tailscale up --hostname=<name>` on the Pi over SSH before
toggling on in the web UI; once enrolled, the name is sticky.
The web UI displays the FQDN and short name on the Settings page
when connected.

## Listener layout

The served web UI proxies to the server's own port on `127.0.0.1`
(`:80` on the Pi image, `:8080` by default elsewhere) — the same Go
server that the LAN reaches. The LiDAR monitor (`:8081`) and the gRPC
visualiser stream (`:50051`) bind to loopback by default, so they are
not reachable over the tailnet unless an operator rebinds them. See
[networking.md](../../radar/architecture/networking.md) for the
full listener segmentation.

## Troubleshooting

| Symptom                                                  | Likely cause                                                                                                                         | Where to look                                                                                                                  |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------ |
| Toggle errors with "operation not permitted"             | sudoers entry missing or the `velocity` user is not in the `velocity` group.                                                         | Re-image, or apply [03-velocity-config/00-run.sh](../../../image/stage-velocity/03-velocity-config/00-run.sh) by hand.         |
| Login URL never appears                                  | Daemon cannot reach `login.tailscale.com`. Almost always a DNS or outbound-firewall problem.                                         | `journalctl -u tailscaled` and `tailscale netcheck` over SSH.                                                                  |
| Connected but Settings shows "Web UI: failed"            | MagicDNS name not yet propagated, or HTTPS certs disabled in the admin console.                                                      | The manager retries serve setup 6 times; if it still fails, enable HTTPS at `login.tailscale.com/admin/dns` and toggle off/on. |
| Cap-gated peer gets 403 when it shouldn't                | Grant is on the wrong tailnet policy line, or `-ts-cap-enforcement=on` was set prematurely.                                          | Check the grant in the admin console, and look for `auth: capability enforcement armed` in the journald log.                   |
| Peer gets 503 `peer_lookup_unavailable`                  | tailscaled did not answer a fresh identity lookup; expired grants are not reused.                                                    | `journalctl -u tailscaled`; the request can be retried once the daemon answers.                                                |
| Visitor gets 403 `funnel_request` or `untrusted_forward` | Funnel is enabled, or a reverse proxy on the host forwards for addresses outside the tailnet. With enforcement on, both are refused. | Funnel is unsupported (see Non-goals). Disable it, or the proxy.                                                               |

For the velocity-report side, `journalctl -u velocity-report` shows
the arming event, failed identity lookups, refused forwarded requests,
and each refusal for a missing grant with the peer's tailnet address,
the route and the grant it lacked.

## Non-goals

- **Tailscale Funnel** (public-internet exposure). Conflicts with
  the privacy tenet. With enforcement on, requests serve accepted
  through Funnel are refused; with it off they would reach the API as
  host traffic, so do not enable Funnel on this node.
- **Multi-site mesh.** Coordinating multiple Pis on one tailnet
  works fine, but aggregating their data is a separate project.
- **Headless auth-key via the web UI.** Available via the CLI on
  the Pi; deliberately not surfaced in the toggle.
- **Tailscale on the macOS visualiser.** The visualiser reaches the
  gRPC endpoint over the tailnet without further configuration once
  both ends are enrolled.
