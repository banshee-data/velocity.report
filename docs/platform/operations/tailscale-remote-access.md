# Tailscale Remote Access

Tailscale creates a WireGuard mesh VPN that gives every enrolled device a
stable `100.x.y.z` address — no port forwarding, no dynamic DNS, no exposure
to public internet scanners. This guide covers RPi-specific installation,
recommended `tailscale up` flags, ACL policy for a velocity-report deployment,
and SSH access.

## Scope

RPi only. The [setup guide](../../../public_html/src/guides/setup.md#remote-access-with-tailscale-optional)
covers the 5-minute quickstart; this document goes deeper with production
flags, access control, and integration with the server's existing
[listener architecture](../../radar/architecture/networking.md).

## 1. Install Tailscale on the Raspberry Pi

```bash
curl -fsSL https://tailscale.com/install.sh | sh
```

This installs the `tailscaled` daemon and the `tailscale` CLI. On
Raspberry Pi OS (Debian-based), the installer adds the apt repository and
starts the service automatically.

Verify:

```bash
tailscale version
systemctl status tailscaled
```

## 2. Authenticate and bring the node up

### Minimal (interactive)

```bash
sudo tailscale up
```

Follow the printed URL to authenticate in a browser. Suitable for a Pi
with a keyboard/monitor attached, or when SSH'd in.

### Headless (recommended for RPi)

Generate an auth key from the
[Tailscale admin console](https://login.tailscale.com/admin/settings/keys):

- **Reusable:** No (single-use is safer for a fixed device)
- **Ephemeral:** No (the Pi is a permanent node)
- **Pre-approved:** Yes (avoids manual approval)
- **Tags:** `tag:velocity-report` (see ACL section below)

```bash
sudo tailscale up --auth-key=tskey-auth-<key> --hostname=velocity-pi
```

### Recommended flags

| Flag                      | Purpose                                                       |
| ------------------------- | ------------------------------------------------------------- |
| `--hostname=velocity-pi`  | Human-readable MagicDNS name (`velocity-pi.<tailnet>.ts.net`) |
| `--auth-key=tskey-auth-…` | Headless authentication — no browser needed                   |
| `--ssh`                   | Enable Tailscale SSH (see §5 below)                           |
| `--accept-dns=true`       | Accept MagicDNS names (default; listed for clarity)           |

Full command for production:

```bash
sudo tailscale up \
  --auth-key=tskey-auth-<key> \
  --hostname=velocity-pi \
  --ssh
```

## 3. Access the velocity-report web UI

Once Tailscale is running on both the Pi and a phone/laptop:

```text
http://velocity-pi:8080        # MagicDNS (if enabled)
http://100.x.y.z:8080          # Tailscale IP (always works)
```

The LiDAR monitor is on `:8081`:

```text
http://velocity-pi:8081
```

Tailscale-protected debug endpoints (serial commands, DB backup, tailsql,
pprof) are served on `:8080` under `/debug/*` via `tsweb.Debugger` — these
are accessible only from loopback or authenticated Tailscale peers. See
[networking.md](../../radar/architecture/networking.md) for the full listener
segmentation.

## 4. ACL policy recommendations

Tailscale ACLs live in the
[admin console](https://login.tailscale.com/admin/acls/file). Below is a
minimal policy for a velocity-report deployment.

### Tags

```jsonc
"tagOwners": {
  "tag:velocity-report": ["autogroup:admin"]
}
```

### ACL rules

```jsonc
"acls": [
  // Allow admin devices to reach velocity-report Pi on all ports
  {
    "action": "accept",
    "src": ["autogroup:member"],
    "dst": ["tag:velocity-report:*"]
  },
  // Allow the Pi to reach nothing else (least privilege)
  {
    "action": "accept",
    "src": ["tag:velocity-report"],
    "dst": ["tag:velocity-report:*"]
  }
]
```

This grants all Tailscale members access to the Pi's web UI, SSH, and
debug endpoints. The Pi itself can only talk to other velocity-report
nodes (useful if you add a second Pi).

### Restricting port access

For tighter control, replace `*` with specific ports:

```jsonc
{
  "action": "accept",
  "src": ["autogroup:member"],
  "dst": ["tag:velocity-report:8080,8081,22"],
}
```

## 5. SSH access via Tailscale

With `--ssh` enabled at bring-up, Tailscale provides SSH access without
managing host keys or opening port 22 on the LAN:

```bash
ssh velocity-pi              # MagicDNS
ssh 100.x.y.z               # Tailscale IP
```

Tailscale SSH authenticates via the Tailscale identity, not SSH keys. The
SSH ACL in the admin console controls who can connect:

```jsonc
"ssh": [
  {
    "action": "accept",
    "src": ["autogroup:member"],
    "dst": ["tag:velocity-report"],
    "users": ["pi", "root"]
  }
]
```

**Benefit:** No need to distribute SSH keys or manage `authorized_keys`.
Revoking a team member's Tailscale access immediately revokes their SSH
access.

## 6. Verifying the setup

From any enrolled device:

```bash
# Check the Pi is reachable
tailscale ping velocity-pi

# Verify the web UI
curl -s http://velocity-pi:8080/api/config | head -c 200

# Verify debug endpoints (Tailscale peers only)
curl -s http://velocity-pi:8080/debug/db-stats
```

From the Pi itself:

```bash
tailscale status          # List connected peers
tailscale netcheck        # Diagnose connectivity
```

## 7. Keeping Tailscale updated

Tailscale updates itself via apt on Debian-based systems:

```bash
sudo apt update && sudo apt upgrade tailscale
```

Or enable auto-update:

```bash
sudo tailscale set --auto-update
```

## 8. Troubleshooting

| Symptom                      | Cause                              | Fix                                        |
| ---------------------------- | ---------------------------------- | ------------------------------------------ |
| `tailscale up` hangs         | Pi has no internet access          | Check `eth0`/`wlan0` gateway and DNS       |
| MagicDNS names don't resolve | MagicDNS disabled in admin console | Enable under DNS settings in admin console |
| `/debug/*` returns 403       | Request not from Tailscale peer    | Access via Tailscale IP, not LAN IP        |
| SSH connection refused       | `--ssh` not passed at bring-up     | `sudo tailscale set --ssh`                 |

## Non-goals

- **Tailscale on macOS visualiser**: covered separately if needed; the
  visualiser connects to the Pi's gRPC endpoint, which is reachable over
  Tailscale without additional configuration.
- **Tailscale Funnel**: exposes services to the public internet — directly
  conflicts with the privacy-first deployment model.
- **Multi-site mesh**: coordinating multiple Pis is a v0.6.0+ concern.
