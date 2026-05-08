# Tailscale remote access

Operator and contributor reference for the Tailscale integration on the
Pi image. For the click-through enrolment flow that an end user sees,
read the user guide at
[public_html/src/guides/tailscale.md](../../../public_html/src/guides/tailscale.md)
instead.

## What's vendored

The Pi image installs `tailscaled` and the `tailscale` CLI from the
upstream apt repo, then **masks** the systemd unit. Nothing reaches
out to Tailscale's coordination server unless the operator opts in
through the web UI. This is a privacy tenet, not a configuration
detail: the image is published publicly and may run in environments
where outbound traffic is sensitive, so the default state is
"installed but inert."

| Concern             | Where it lives                                                                        |
| ------------------- | ------------------------------------------------------------------------------------- |
| Package install     | [image/stage-velocity/07-velocity-tailscale/01-run.sh](../../../image/stage-velocity/07-velocity-tailscale/01-run.sh) |
| systemd unmask flow | [cmd/velocity-ctl/tailscale.go](../../../cmd/velocity-ctl/tailscale.go)               |
| sudoers grant       | [image/stage-velocity/03-velocity-config/00-run.sh](../../../image/stage-velocity/03-velocity-config/00-run.sh) (lines 49–51) |
| Manager / IPN bus   | [internal/tailscale/manager.go](../../../internal/tailscale/manager.go)               |
| HTTP endpoints      | [internal/api/server_tailscale.go](../../../internal/api/server_tailscale.go)         |
| Web UI              | [web/src/routes/(constrained)/settings/+page.svelte](../../../web/src/routes/%28constrained%29/settings/+page.svelte) |

## Trust model

Three boundaries, each enforced by something other than convention:

1. **Daemon lifecycle requires root** (unmask/enable/start/stop/mask).
   The non-root `velocity` service user is granted exactly two sudo
   actions, by literal argv with no wildcards:

   ```
   /usr/local/bin/velocity-ctl tailscale enable-tailscaled
   /usr/local/bin/velocity-ctl tailscale disable-tailscaled
   ```

2. **Daemon configuration runs as `velocity`** over the local API
   socket. After the daemon starts, `velocity-ctl` runs
   `tailscale set --operator=velocity`, which authorises the service
   user to drive `tailscaled` without root for everything else
   (login, prefs, serve config, status).

3. **Inbound HTTP authorization is by source plus optional
   capability grant.** LAN and loopback are admin; the tailnet is
   gated by `velocity.report/cap/{view,admin}` grants in your tailnet
   policy (see [Capability grants](#capability-grants) below).

## Enable / disable flow

When the operator toggles Tailscale on in Settings:

1. The web UI POSTs `/api/tailscale/enable`. The Go server shells
   out to `sudo /usr/local/bin/velocity-ctl tailscale
   enable-tailscaled`, which unmasks, enables, and starts the
   service, waits up to 15 s for `/var/run/tailscale/tailscaled.sock`
   to appear, and runs `tailscale set --operator=velocity`.
2. The manager subscribes to the IPN bus, calls
   `StartLoginInteractive`, and caches the resulting `BrowseToURL`
   for up to 5 minutes.
3. The web UI fast-polls `/api/tailscale/status` (every 2 s during
   login) and renders the URL plus a QR code. The user opens it in
   their tailnet account and approves the device.
4. Once the node reaches `Running`, `applyDevicePolicy` runs **once
   per Enable**:
   - `RunSSH=true` via `EditPrefs` → Tailscale SSH on.
   - `SetServeConfig` with a web handler at `https://<fqdn>:443/`
     proxying to `http://127.0.0.1:8080` → web UI on the tailnet.

   These two steps record their results independently
   (`sshOK`/`sshErr`, `serveOK`/`serveErr`) so the Settings page can
   surface partial failures.

Disable reverses it: clear the serve config, set `WantRunning=false`,
then `disable-tailscaled` runs `stop`, `disable`, and `mask` in that
order. The node identity stays on disk; toggling on again resumes
the same membership without a fresh login.

## What the operator still has to do

The image vendors the daemon and the lifecycle plumbing. It does
**not** vendor anything that has to live in your tailnet account:

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
deployment, run `sudo tailscale up --auth-key=…` on the Pi over SSH
*before* using the web UI; the manager picks up the running daemon
on its first poll.

## Capability grants

By default any tailnet peer that can reach the Pi has full admin
access. To split read from write, use
[application capability grants](https://tailscale.com/kb/1324/grants-app-capabilities).

velocity-report recognises two cap names:

- `velocity.report/cap/view` — read-only access to `/api/*` and
  `/events`.
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

| Mode  | Behaviour                                                                                                          |
| ----- | ------------------------------------------------------------------------------------------------------------------ |
| `off` | (default) Capability checks disabled. Every reachable peer is admin. Use this until grants are validated.          |
| `on`  | Enforce caps for tailnet-sourced requests. LAN and loopback are still admin. Flip to `on` after grants are wired.  |

### Trust model

The gate is a **default-deny wrapper** installed around the entire HTTP
mux. Anything not on a small explicit allowlist requires a cap; routes
added later by other packages (LiDAR, debug, db admin, etc.) inherit
the default-deny policy automatically.

Source classification rules:

- **Loopback `RemoteAddr` + `X-Forwarded-For` set** → trust XFF; this
  is how `tailscale serve` forwards tailnet requests to the local
  HTTP server.
- **Loopback with no XFF** → host-local (the Go server itself,
  `velocity-ctl`, a local `curl`). Treated as admin.
- **Non-loopback `RemoteAddr`** → LAN or direct hit on `:8080`.
  `X-Forwarded-For` is **ignored** entirely so a LAN attacker who
  can reach the server directly cannot forge a tailnet identity by
  setting the header. Treated as admin.

Failure modes:

- The daemon authoritatively reporting "no such peer" → 403
  `{"error":"unknown_peer"}`.
- A transient lookup error (socket down, timeout) → fail-open. A
  tailscaled blip should not deny every authorised user at once.
- A peer with no caps → 403 `{"error":"missing_cap","required":"…"}`.

### Caveats

- **Non-tailnet sources are always admin.** Loopback (`127.0.0.1`,
  `velocity-ctl`) and LAN sources bypass the cap check entirely.
  Gate LAN access at the network layer (firewall, VLAN) if that
  isn't acceptable.
- **`/api/tailscale/status` is unconditionally reachable** so an
  operator with a botched grant policy can still see the daemon
  state and recover.
- **Grants only protect the velocity-report HTTP API.** They do not
  cover Tailscale SSH, the gRPC visualiser stream, or any other
  port. Use ACL rules for those.
- **Recovery from a misconfigured ACL relies on the LAN bypass.**
  Once you've validated grants on a test peer, flip
  `-ts-cap-enforcement=on` and restart.

## Hostname and MagicDNS

The MagicDNS name comes from the daemon, not from the image. The
daemon picks up the system hostname at first `tailscale up`, then
the coordination server owns it. To set a specific hostname, run
`sudo tailscale up --hostname=<name>` on the Pi over SSH before
toggling on in the web UI; once enrolled, the name is sticky.
The web UI displays the FQDN and short name on the Settings page
when connected.

## Listener layout

The served web UI proxies to `http://127.0.0.1:8080` — the same Go
server that the LAN reaches. The LiDAR monitor on `:8081` is **not**
proxied through tailscale serve and is reachable over the tailnet
only via its IP (`http://<tailnet-ip>:8081`). See
[networking.md](../../radar/architecture/networking.md) for the
full listener segmentation.

## Troubleshooting

| Symptom                                         | Likely cause                                                                                  | Where to look                                                                                                                  |
| ----------------------------------------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| Toggle errors with "operation not permitted"    | sudoers entry missing or the `velocity` user is not in the `velocity` group.                  | Re-image, or apply [03-velocity-config/00-run.sh](../../../image/stage-velocity/03-velocity-config/00-run.sh) by hand.         |
| Login URL never appears                         | Daemon cannot reach `login.tailscale.com`. Almost always a DNS or outbound-firewall problem.  | `journalctl -u tailscaled` and `tailscale netcheck` over SSH.                                                                  |
| Connected but Settings shows "Web UI: failed"   | MagicDNS name not yet propagated, or HTTPS certs disabled in the admin console.               | The manager retries serve setup 6 times; if it still fails, enable HTTPS at `login.tailscale.com/admin/dns` and toggle off/on. |
| Cap-gated peer gets 403 when it shouldn't       | Grant is on the wrong tailnet policy line, or `-ts-cap-enforcement=on` was set prematurely.   | Check the grant in the admin console, and look for `auth: capability enforcement armed` in the journald log.                   |
| LAN client unexpectedly hits 403                | The LAN bypass relies on the request not coming through tailscale serve. Funnel breaks this.  | Funnel is unsupported (see Non-goals). If you've enabled it, disable it.                                                       |

For the velocity-report side, `journalctl -u velocity-report` shows
all auth-gate decisions (the arming event, WhoIs failures, and any
403 responses).

## Non-goals

- **Tailscale Funnel** (public-internet exposure). Conflicts with
  the privacy tenet and breaks the LAN-vs-tailnet source
  classification that capability grants depend on.
- **Multi-site mesh.** Coordinating multiple Pis on one tailnet
  works fine, but aggregating their data is a separate project.
- **Headless auth-key via the web UI.** Available via the CLI on
  the Pi; deliberately not surfaced in the toggle.
- **Tailscale on the macOS visualiser.** The visualiser reaches the
  gRPC endpoint over the tailnet without further configuration once
  both ends are enrolled.
