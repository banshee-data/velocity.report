---
layout: doc.njk
title: Remote access with Tailscale
description: Optional — reach your radar from anywhere over a private Tailscale tailnet with Tailscale SSH and a served web UI
section: guides
difficulty: easy
time: 2 minutes
date: 2026-04-18T12:00:00Z
tags: [networking, tailscale, remote-access]
---

**Reach your velocity.report device from anywhere on your private Tailscale tailnet — Tailscale SSH, plus the web UI served over HTTPS, without opening any ports on your home router.**

This is optional. The device is fully functional on a local network without Tailscale. Enrol it only if you want remote access.

## Why you might want this

Share the dashboard with a neighbour, a council staffer, or anyone else who has Tailscale — they can open it from home, without you opening ports or setting up a VPN. Also handy for reaching the device yourself if it's deployed somewhere you don't live.

## What you get

Once enrolled, the device is a node on your tailnet with:

- **Tailscale SSH** — `ssh pi@<hostname>` from any other tailnet device. No port 22 on your LAN; no SSH key to manage.
- **Tailscale serve** — `https://<hostname>.<your-tailnet>.ts.net/` reaches the web UI over the tailnet, with Tailscale's automatic HTTPS certificate.
- **Nothing else** — the device does not advertise itself as an exit node, does not accept subnet routes, and does not replace your local DNS.

## Before you begin

You need a Tailscale account and tailnet. Sign up at [tailscale.com](https://tailscale.com) if you don't have one — the personal plan is free.

If you want HTTPS for the served web UI, enable HTTPS certificates for your tailnet at [login.tailscale.com/admin/dns](https://login.tailscale.com/admin/dns). The device fetches its certificate automatically on first connect.

## Enrol the device

1. Open the velocity.report web UI on your local network: `http://<your-pi-ip>/` or `http://velocity.local/`.
2. Go to **Settings**.
3. Find the **Tailscale** card and toggle the switch on.
4. The card shows a login URL and a QR code. Click the link, or scan the QR code with your phone, and sign into your tailnet.
5. After you approve the device, the page updates on its own. The card now shows the MagicDNS name and a link to open the web UI over the tailnet.

That's it. The device is on the tailnet, Tailscale SSH is enabled, and the web UI is published at `https://<hostname>.<your-tailnet>.ts.net/`.

## Turning Tailscale off

Toggle the switch back off in Settings. The device stops talking to the coordination server immediately, and `tailscaled` is masked so it stays off across reboots. The node's identity stays on disk, so toggling on again resumes the same tailnet membership without another login.

To remove the device from your tailnet entirely, delete it from the [Tailscale admin console](https://login.tailscale.com/admin/machines) after disabling.

## Troubleshooting

**The toggle won't turn on, or shows an error.** Check the velocity-report logs over the LAN:

```bash
ssh pi@velocity.local
sudo journalctl -u velocity-report -n 50
```

**Login URL never appears.** The device needs outbound internet for `tailscaled` to reach Tailscale's coordination server. Check `ping login.tailscale.com` from the device.

**HTTPS certificate fails to issue.** Your tailnet might not have HTTPS certificates enabled. Turn it on at [login.tailscale.com/admin/dns](https://login.tailscale.com/admin/dns), then toggle the device off and on again to re-fetch.
