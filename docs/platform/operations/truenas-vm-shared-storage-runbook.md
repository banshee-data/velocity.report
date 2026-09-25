# TrueNAS `arrow` to VM `bansheeworker`: local shared storage

Status: solved. The VM `bansheeworker` (guest hostname `arrow-worker`) mounts NFS exports from the
TrueNAS host `arrow`'s pool over a local, Tailscale-independent path. Verified: the guest mount
survives a guest reboot. Not yet verified: a full TrueNAS host reboot (see section 6). This
document records what was tried, why each attempt failed, the confirmed root cause, and the
working solution, so a future change to this host does not repeat a night's worth of failed
attempts and one real outage.

## 1. The problem

`bansheeworker`'s only network interface was a macvtap NIC bridged onto the host's physical
interface, `eno1`. macvtap in bridge mode isolates the guest from its own host: guest and host
share a switch port, so the switch never reflects frames between them directly. Every other host
on the LAN could reach both; they could not reach each other.

The VM's `/etc/fstab` mounted two exports over a Tailscale address (`100.74.230.36`, the host's old
tailnet identity). When the host's Tailscale presence lapsed, the mounts broke: `df` on the guest
hung until the NFS timeout expired.

The goal: give the VM a local path to the pool's datasets that depends on neither the physical LAN
(blocked by macvtap isolation) nor Tailscale (no longer present on the host).

## 2. What was tried, and why it failed

### 2.1 Isolated bridge, repeated attempts

The natural fix for macvtap isolation is a bridge: create a new interface on the host, attach the
VM's NIC to it, and let host and guest talk over that shared L2 segment. Six consecutive attempts
to create such a bridge (`10.10.10.1/24`, no members, via TrueNAS's Network > Add > Bridge, applied
through its "Test Changes" mechanism) all failed the same way: the host went unreachable for
almost exactly the configured test window (60 to 300 seconds), then TrueNAS's automatic rollback
restored it. This happened both before and after a full host reboot, which ruled out a flaky NIC
or driver.

### 2.2 A CLI shortcut that caused a real outage

TrueNAS's network changes are applied in two steps: `interface.commit` stages the change and
schedules an automatic rollback after a timeout; `interface.checkin` confirms the change and
cancels that rollback. A script that called `commit` and then immediately called `checkin` was
built on the assumption that `commit` blocks until the underlying resync finishes. It does not:
`commit` returns in well under a second, so the immediate `checkin` confirmed a half-applied,
broken network state and cancelled the safety rollback that would otherwise have fixed it. The
host went dark until someone reached the physical console and restored `eno1` by hand.

**Rule that follows from this**: never chain `interface.checkin` after `interface.commit` in a
script. Confirm a change only after independently observing that the host is reachable in its new
state, and only from a session whose own transport does not depend on the change being tested (the
physical console, not a browser tab or SSH session riding on the interface being changed).

### 2.3 A dead end: `interface.query`'s fields do not mean what they look like they mean

Partway through diagnosis, `midclt call interface.query` showed `eno1` as
`{"fake": false, "dhcp": true, "aliases": []}`, which looks exactly like "this interface is
registered in the network database." It was not. A live retry of the isolated-bridge design with
that state in place reproduced the identical failure. A direct query against the actual table
(`midclt call datastore.query network.interfaces`, or `sqlite3 -readonly /data/freenas-v1.db
"select * from network_interfaces"` on a copy) showed the table completely empty at that point.

**Rule that follows from this**: to check whether an interface is genuinely registered, query
`datastore.query network.interfaces` directly (the same call `InterfaceService.sync()` itself
uses), or read the middleware log's "Interfaces in database" line. Do not trust `interface.query`'s
`fake` or `dhcp` fields for this; they reflect live kernel state, not database membership.

## 3. Root cause, confirmed from source and logs

TrueNAS's middleware log (`/var/log/middlewared.log`, `root:adm`, readable via
`sudo cp` and `chmod` for a diagnostic session) recorded every attempt. It, together with the
shipped middleware source
(`/usr/lib/python3/dist-packages/middlewared/plugins/network.py`, world-readable), gives a
complete, evidence-based explanation with no remaining guesswork.

### 3.1 `eno1` had no row in the network database

`InterfaceService.sync()` builds its "interfaces to preserve" set with a direct database query,
then unconfigures any physical interface that is not in that set, unconditionally, before doing
anything else. `eno1` had never been given a database row (it existed only as a live, DHCP-managed
kernel interface, outside TrueNAS's own configuration tracking), so every commit, whatever it
touched, stripped `eno1`'s address as a side effect. This explains why even the "isolated bridge,
zero members" design, meant to leave `eno1` alone entirely, still took the host down every time.

### 3.2 Bridge membership fails while the VM's macvtap NIC exists

Two early attempts used a bridge with `eno1` as a member (the intended, textbook TrueNAS layout).
Both failed with a subprocess error: `ip link set eno1 master br1` returned a non-zero exit
status. Confirmed live: `eno1` had (and has) an active macvtap child, the VM's own NIC. Linux
allows only one `rx_handler` per network device; bridge membership and macvtap both need to own it,
and are mutually exclusive. `eno1` cannot be enslaved into a bridge while a macvtap child of it
exists, a kernel constraint, not a TrueNAS defect. (A related, separate defect: TrueNAS's own
`bridge_setup()` swallows this exception and lets the sync continue as if it had succeeded, leaving
both the bridge and `eno1` broken. Worth an upstream bug report; not itself part of the fix here.)

### 3.3 Recovery came from the DHCP client, not from TrueNAS

`interface.rollback()` deletes all rows from the network tables (confirmed by both a comment in the
source and by the log showing an empty "Interfaces in database" line after every rollback), then
resyncs. With an empty database that resync unconfigures `eno1` again and stops: nothing in the
commit, rollback, or sync path ever restores an address on an interface that was never in the
database. `eno1`'s own independent `dhclient` process (started outside this code path, holding a
router-issued lease) was what eventually noticed the address was gone and re-acquired it. Recovery
time was therefore the full rollback timeout plus however long that separate reacquisition took,
which is why outage duration tracked the configured test window so closely.

## 4. The solution

### 4.1 Register `eno1` in the database, on its own

The fix for section 3.1: give `eno1` a database row before creating anything else. Edit the
interface in place (Network > `eno1` > Edit) without changing its addressing: ticking "Get IP
Address Automatically from DHCP" is enough if that already matches its live state. Apply through
the normal Test Changes / Save Changes flow. This alone caused no outage, because `eno1` becomes
the only interface in the preserved set and nothing gets unconfigured.

Verify the row exists before relying on it:

```bash
midclt call datastore.query network.interfaces
```

should list `eno1` with `int_dhcp: true` (or the equivalent static fields).

### 4.2 Create the bridge

With `eno1` registered, the isolated bridge design that had failed six times worked immediately,
with no outage:

- Network > Add > Bridge, name `br1`, no members, static address `10.10.10.1/24`.
- Apply through Test Changes with a generous window (300 seconds is comfortable), confirm the host
  stays reachable throughout (a fresh Shell session or `ping` from a second host is a better test
  than watching the same browser tab that issued the change), then Save Changes.

`br1` reports link state `DOWN` in `ip -br a` until a real member is attached; this is expected for
an empty bridge and does not indicate a problem.

### 4.3 Attach the VM and configure the guest

Add a second NIC device to the VM (VirtIO adapter, "NIC To Attach" set to `br1`) and restart it.
The guest's network stack determines the new device's name; check with `ip -br a` rather than
assuming. On this VM (Debian, managed by classic `ifupdown`, not systemd-networkd) the new
interface came up as `ens4`. Give it a static address and persist it as a drop-in file:

```bash
sudo tee /etc/network/interfaces.d/ens4-local.cfg <<'EOF'
auto ens4
iface ens4 inet static
    address 10.10.10.2
    netmask 255.255.255.0
EOF
sudo ifup ens4
```

**A note on DHCP address drift, not a bridge problem.** After the first NIC attach and VM restart,
the guest's original address stopped answering, which looked like the hardware change had broken
something. It had not: the guest's primary interface is DHCP-managed, and the router had simply
re-leased it to a different address across the VM's several restarts. `ip -br a` inside the guest
resolved this in one line. Check for address drift before assuming a hardware change broke the
guest.

### 4.4 NFS and fstab

- Add `10.10.10.0/24` to both NFS shares' Networks list (Shares > UNIX (NFS) Shares > Edit),
  alongside the existing `192.168.99.0/24`.
- Add a name for the new address to the guest's `/etc/hosts`: `10.10.10.1 arrow-local`.
- Back up and edit `/etc/fstab`: replace the dead Tailscale address with `arrow-local`, and drop
  the `x-systemd.after=tailscaled.service,x-systemd.requires=tailscaled.service` mount options,
  which no longer apply.

```bash
sudo cp /etc/fstab /etc/fstab.bak-$(date +%Y%m%d%H%M%S)
sudo sed -i 's/100\.74\.230\.36/arrow-local/g' /etc/fstab
sudo sed -i 's/,x-systemd.after=tailscaled.service,x-systemd.requires=tailscaled.service//g' /etc/fstab
sudo systemctl daemon-reload
sudo systemctl restart remote-fs.target
```

## 5. Verification

- `df -h /mnt/captures /mnt/results` shows both exports mounted over `arrow-local` (`10.10.10.1`).
- A 50 MiB write to `/mnt/results` completed at 603 MB/s, full local virtio-net speed, with no
  tailnet hop.
- The guest was rebooted; `ens4` and both mounts came back automatically, with no manual steps,
  confirmed immediately after boot.

## 6. Known limitations and possible follow-ups

- **Host reboot is untested.** The interface and bridge configuration are now genuinely persisted
  in TrueNAS's database, so they are expected to survive a reboot, but this has not been observed
  directly. Worth a deliberate, low-traffic test.
- **Tailscale remains installed and running on the guest** (not the host). It is no longer needed
  for this mount; removing it, or leaving it for other uses, is a separate decision.
- **The "proper" `br0`-with-`eno1`-member layout** (TrueNAS's documented default topology, where the
  host's own address moves onto the bridge and `eno1` becomes a plain member) remains blocked by
  the constraint in section 3.2 for as long as the VM's current NIC is attached to `eno1`. It could
  be done by stopping the VM (or removing its NIC device) first, creating `br0` with `eno1` as a
  member, moving `eno1`'s address to `br0`, then reattaching the VM's NIC to `br0` once it is a
  selectable option. Not attempted: it needs VM downtime and was not required to reach the goal.
- **The `bridge_setup()` exception-swallowing defect** in section 3.2 is worth reporting upstream
  with this evidence, independent of anything this deployment does.

## 7. Reference: safe commit procedure

Applies to any future change to `eno1`, `br1`, or a new interface on this host.

1. Stage the change through the normal UI form; this only creates a pending change, no risk yet.
2. Apply with Test Changes and a window of at least 180 seconds.
3. Confirm reachability independently of the tab that issued the change: open a fresh Shell
   session, or ping from a second host. Never trust the same browser tab's own responsiveness as
   proof, since a lost connection there can look identical to a slow page.
4. Only once reachability is confirmed, click Save Changes. If anything looks wrong, do nothing
   and let the automatic rollback run its course, or trigger it immediately with
   `midclt call interface.rollback`.
5. Never script a `commit` followed by an immediate `checkin`. There is no reliable signal, short
   of independently observing the new state, that the resync has actually finished.

If the host is genuinely unreachable and the automatic rollback has not yet fired, use the
physical console (or IPMI/BMC console) rather than any network-dependent session: it does not
depend on the interface being changed. From there, `ip -br a` and `midclt call
interface.has_pending_changes` establish the current state before deciding whether to wait for the
rollback or force it.
