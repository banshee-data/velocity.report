# HTTPS via Tailscale Serve

- **Status:** Current
- **Audience:** Operators who want a trusted HTTPS padlock on their device
- **Related:** [tailscale-remote-access.md](./tailscale-remote-access.md), [setup guide](../../../public_html/src/guides/setup.md#remote-access-with-tailscale-optional)

The shipped image serves plain HTTP on the LAN at `http://velocity.local`. There
is no bundled TLS: no nginx, no self-signed certificate, no local CA to install.
The device serves no PII (see [TENETS.md](../../../TENETS.md)), so plain HTTP on a
home LAN is an accepted trade-off.

If you want a real HTTPS padlock — a browser-trusted certificate with no warnings —
the supported path is **Tailscale Serve**. Tailscale obtains a genuine Let's Encrypt
certificate for your device's `*.ts.net` name and terminates TLS for you. Nothing is
self-signed, so there is no warning to click through and no CA to trust.

## Why this is user-driven, not bundled

Tailscale Serve needs you to be logged into _your_ tailnet. The image build cannot
log into your account, so it cannot bake this in. That is the whole reason it is a
three-command opt-in rather than a default. This matches the project's standing
decision not to ship a bundled HTTPS layer.

## Quickstart (three commands)

Run these on the Pi (over SSH, or with a keyboard attached):

```bash
# 1. Join your tailnet (opens a browser URL to authenticate)
sudo tailscale up

# 2. Put HTTPS in front of the local HTTP server on :80
sudo tailscale serve --bg http://localhost:80

# 3. Confirm what is being served, and on what URL
sudo tailscale serve status
```

`serve status` prints the public HTTPS URL, which looks like:

```
https://velocity-pi.<your-tailnet>.ts.net
```

Open that URL from any device signed into the same tailnet. The dashboard loads over
HTTPS with a valid certificate and no browser warning.

## What each command does

| Command                                    | Effect                                                                          |
| ------------------------------------------ | ------------------------------------------------------------------------------- |
| `tailscale up`                             | Enrolls the Pi in your tailnet and provisions its MagicDNS `*.ts.net` name      |
| `tailscale serve --bg http://localhost:80` | Terminates TLS on `:443` over the tailnet and proxies to the local `:80` server |
| `tailscale serve status`                   | Shows the active mappings and the public HTTPS URL                              |

The `--bg` flag runs the proxy in the background so it survives your SSH session and
restarts with the device.

## Turning it off

```bash
sudo tailscale serve --https=443 off   # stop serving HTTPS
sudo tailscale serve status            # confirm it is gone
```

The local `http://velocity.local` LAN endpoint is unaffected by Tailscale Serve; it
keeps working whether or not Serve is enabled.

## Notes and limits

- **This is HTTPS over the tailnet, not the public internet.** Only devices signed
  into your tailnet can reach the `*.ts.net` URL. To expose the device publicly you
  would need Tailscale _Funnel_, which conflicts with the privacy-first model and is
  out of scope — see the Non-goals in [tailscale-remote-access.md](./tailscale-remote-access.md).
- **The web UI has no authentication.** Tailnet membership is the access control. A
  shared or untrusted LAN should be isolated at the network layer regardless — see
  the setup guide's [shared and untrusted networks](../../../public_html/src/guides/setup.md#shared-and-untrusted-networks)
  section.
- For production tailnet setup — auth keys, ACLs, tags, and Tailscale SSH — see the
  full [Tailscale remote access guide](./tailscale-remote-access.md).
