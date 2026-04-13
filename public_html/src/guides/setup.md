---
layout: doc.njk
title: Set Up Your Radar
description: Build a privacy-first traffic radar with Raspberry Pi and a pre-built image; no cameras, no cloud, just local speed data
section: guides
difficulty: intermediate
time: 2-4 hours
cost: $592
date: 2026-03-26
tags: [hardware, raspberry-pi, infrastructure, traffic-safety]
---

**A weatherproof traffic logger that keeps data local, requires no cameras, and helps you make the case for safer streets with evidence.**

**Difficulty**: Intermediate • **Time**: 2–4 hours • **Cost**: $592

**In this guide**: [Parts List](#parts-and-tools-list) • [Build Steps](#step-by-step-build-guide) • [Generate Reports](#step-6-generate-reports) • [Present Your Case](#taking-your-data-to-city-hall) • [Troubleshooting](#troubleshooting)

---

## Introduction

Measuring vehicle speeds is the first step toward safer streets. Without data, the conversation stalls at "it feels fast" versus "the speed limit is fine," and data is what moves the conversation forward.

This guide walks you through building a privacy-first traffic radar using a pre-built Raspberry Pi image and off-the-shelf Doppler technology. No cameras, no licence plates, no cloud accounts. The system starts collecting data the moment it boots.

## Before you begin

**Skills required**:

- Basic Linux command line (SSH, file editing)
- Basic hardware assembly (connecting cables, mounting)

**Tools needed**:

- Computer with [Raspberry Pi Imager](https://www.raspberrypi.com/software/) installed
- Screwdrivers, drill, adhesive
- 5/16" nut driver (for steel bands)
- Optional: multimeter for testing connections

**No soldering required** 👩‍💻 **No coding required** 🛜 **No prior radar experience needed**

---

## Privacy and legal considerations

### What this system collects

|                        |                                                          |
| ---------------------- | -------------------------------------------------------- |
| ✅ **Collected**       | Vehicle speed, direction, timestamp                      |
| ❌ **Not collected**   | No licence plates, no vehicle photos, no driver identity |
| ❌ **Not transmitted** | All data stays on your device                            |

The system records vehicle speed data without cameras, licence plates, or personal details.

### Legal position

In most jurisdictions, measuring vehicle speeds on public streets from your own property is legal: it is the same activity traffic engineers and academic researchers perform. Mount on utility poles only with permission. Check local regulations for long-term installations or school zones. When in doubt, consult local authorities.

---

## What you will build

- **Doppler radar logger**: captures vehicle speeds 24/7
- **Local SQLite database**: all data stays on the device
- **Live web dashboard**: real-time speeds, histograms, time-of-day patterns
- **Professional PDF reports**: traffic engineering metrics (p50, p85, p98)
- **Weatherproof hardware**: designed for permanent outdoor deployment

The velocity.report Pi image includes everything pre-configured: server, web dashboard, PDF generator, serial port settings, and a systemd service that starts automatically on boot. Flash one SD card, connect one sensor.

---

## Parts and tools list

The **OmniPreSense OPS7243-A-CW-R2** is recommended for infrastructure deployment: weatherproof (IP67), 100 m range, RS232 interface.

### Bill of materials

| Part             | Recommended Model                                                                                     | Price    | Notes                                                                    |
| ---------------- | ----------------------------------------------------------------------------------------------------- | -------- | ------------------------------------------------------------------------ |
| Radar Sensor     | [OPS7243-A-CW-R2](https://omnipresense.com/product/31099/)                                            | $420     | Speed-only, RS232 interface (designated R2), 100 m range, IP67 enclosure |
| Mounting Plate   | [OPS100-BK](https://omnipresense.com/product/mounting-bracket-all-weather-enclosures/)                | $50      | Metal mounting bracket for OPS7243 enclosure                             |
| M12 Cable        | [OPS700-CBL-M1-PT-1.8](https://omnipresense.com/product/rs-232-cable-with-m12-connector-for-ops7243/) | $17      | M12 to pigtail, connects sensor to DE-9                                  |
| Raspberry Pi 4   | Raspberry Pi 4 (4 GB)                                                                                 | $45      | Also compatible with Pi 5                                                |
| SD Card          | SanDisk High Endurance 32 GB                                                                          | $10      | Designed for continuous recording                                        |
| Serial HAT       | Waveshare RS232/485 HAT                                                                               | $18      | Required for RS232 interface                                             |
| RS-232 Connector | Adafruit DE-9                                                                                         | $3       | Connects pigtail to HAT                                                  |
| PoE HAT          | Waveshare PoE HAT (F)                                                                                 | $29      | Powers the Pi over Ethernet; stacks with the serial HAT                  |
| **Total**        |                                                                                                       | **$592** |                                                                          |

Power is delivered over Ethernet through the PoE HAT. You will need a PoE-capable switch or a PoE injector on the network side.

---

## Step-by-step build guide

**Build overview** (total time: 2–4 hours):

1. [Wire the Sensor to the Raspberry Pi](#step-1-wire-the-sensor-to-the-raspberry-pi): 15–30 minutes
2. [Flash the Pi Image](#step-2-flash-the-pi-image): 10–15 minutes
3. [Access the Web Dashboard](#step-3-access-the-web-dashboard): 5 minutes
4. [Mount the Radar Sensor](#step-4-mount-the-radar-sensor): 1–2 hours
5. [Configure Your Site](#step-5-configure-your-site): 10 minutes
6. [Generate Reports](#step-6-generate-reports): after data collection

---

### Step 1: wire the sensor to the Raspberry Pi

_Estimated time: 15–30 minutes_

The OPS7243-A-CW-R2 sensor connects to the Raspberry Pi via an RS232 serial HAT. The PoE HAT stacks on top to provide power over Ethernet.

1. **Attach the PoE HAT** to the Raspberry Pi's 40-pin GPIO header

2. **Stack the serial HAT** (Waveshare RS232/485) on top of the PoE HAT. Ensure all pins are aligned and fully seated.

3. **Wire the sensor to the HAT** following the wiring diagram below:

![Radar wiring diagram: M12 cable from OPS7243 sensor through pigtail and DE-9 connector to Waveshare RS232 HAT](/img/radar-wiring.svg)

---

### Step 2: flash the Pi image

_Estimated time: 10–15 minutes_

The velocity.report image is a complete Raspberry Pi OS with all software pre-installed and pre-configured. Flash it to an SD card and the system is ready to run.

#### Option A: use the custom Raspberry Pi Imager catalogue (recommended)

This opens Raspberry Pi Imager with the velocity.report image pre-loaded:

```bash
# macOS
cd "/Applications/Raspberry Pi Imager.app/Contents/MacOS/" && \
  ./rpi-imager --repo https://velocity.report/rpi.json

# Linux
rpi-imager --repo https://velocity.report/rpi.json

# Windows
"C:\Program Files (x86)\Raspberry Pi Imager\rpi-imager.exe" --repo https://velocity.report/rpi.json
```

1. **Select your Pi model** (Pi 4, Pi 400, or Pi 5)
2. **Select velocity.report** from the OS list
3. **Choose your SD card** (32 GB high-endurance recommended)
4. **Configure settings** before writing:
   - Click the **gear icon** (⚙) or **Edit Settings**
   - **Set hostname**: `velocity` (or your preference)
   - **Enable SSH**: select **Allow public-key authentication only** and paste your public key (recommended). If you do not have an SSH key, select password authentication and choose a strong password.
   - **Set username and password**: choose a username and password (required even with key authentication, for `sudo`)
   - **Configure Wi-Fi** (optional): the Pi connects via Ethernet by default through the PoE HAT. Add Wi-Fi credentials only if you need wireless access as a fallback or for initial setup without Ethernet.
5. **Write** the image

#### Option B: manual download

1. Download the latest `.img.xz` file from [GitHub Releases](https://github.com/banshee-data/velocity.report/releases)
2. Open [Raspberry Pi Imager](https://www.raspberrypi.com/software/), select **Choose OS** → **Use custom**, and select the file
3. Configure settings as above, then write

#### After flashing

1. **Insert the SD card** into your Raspberry Pi and connect the PoE Ethernet cable
2. **Wait 1–2 minutes** for first boot, then connect:

```bash
ssh velocity@velocity.local
```

The service starts automatically on boot and configures the sensor (JSON mode, units, magnitude). Verify it is running:

```bash
velocity-status
# Should show "active (running)"
```

To watch live logs:

```bash
velocity-log
```

The image installs shell aliases for common operations: `velocity-status`, `velocity-log`, `velocity-bounce` (restart), `velocity-stop`, and `velocity-start`. See the [Reference](#reference) section for the full list.

---

### Step 3: access the web dashboard

_Estimated time: 5 minutes_

Open a browser on any device on the same network:

### [https://velocity.local](https://velocity.local)

The Pi generates a self-signed TLS certificate on first boot. Your browser will show a certificate warning: this is expected. To eliminate the warning, download the CA certificate from `https://velocity.local/ca.crt` and add it to your browser or system trust store.

The dashboard shows real-time vehicle detections, speed distribution histograms, and time-of-day traffic patterns.

**Success criteria**: the dashboard loads and shows live vehicle detections or "No data yet"

**If the dashboard will not load**:

1. Check the service is running: `velocity-status`
2. Find the Pi's IP address: `hostname -I`
3. Test from the Pi itself: `curl -k https://localhost/`
4. Check logs: `velocity-log`

---

### Step 4: mount the radar sensor

_Estimated time: 1–2 hours_

**Enclosure preparation**:

- Drill mounting holes in the back plate for hose clamps
- Install cable glands for power and Ethernet
- Mount the sensor inside with a clear view through the front panel
- Use plastic or nylon standoffs (metal obstructs the radar signal)

**Positioning**:

- Mount 4–8 feet off the ground (reduces false detections from small objects)
- Use two stainless steel hose clamps (top and bottom)
- Choose a location with a clear line of sight to traffic

**Aiming**:

- **Angle**: as close to 0° (parallel with traffic flow) as practical. Lower angles produce more accurate speed measurements because they need less cosine correction. At 0°, the radar reads the full vehicle speed directly. At 30°, measured speeds are 86.6% of actual, and the correction amplifies measurement noise.
- **Road coverage**: a 0° angle gives the best accuracy but the narrowest field of view. A slight angle (10–20°) lets the radar beam sweep across the full road width, capturing vehicles in all lanes. Choose the smallest angle whose field-of-view triangle fully encompasses the lanes you need to measure.
- **Orientation**: face approaching or receding traffic (not perpendicular)
- **Record your mounting angle**: you will enter this in the dashboard as the cosine error angle (Step 5)

**Weatherproofing checklist**:

- ✅ All cable glands sealed
- ✅ Desiccant pack inside enclosure
- ✅ Enclosure gasket intact and clean
- ✅ Seal tested before final mounting

**Success criteria**: enclosure is weatherproof, sensor aims correctly, mounting is secure

---

### Step 5: configure your site

_Estimated time: 10 minutes_

Before generating reports, configure your site in the dashboard so speed measurements are corrected for your mounting angle.

1. Open the dashboard at [`https://velocity.local`](https://velocity.local)
2. Navigate to **Site Settings**
3. Set the **site location** on the interactive map
4. Set the **cosine error angle** to match your radar mounting angle from Step 4. Drag the red dot on the radar field-of-view triangle to adjust the angle visually, or type the value directly. The triangle should encompass the road lanes you want to measure.
5. Add any **notes** about the installation (useful for later reference)
6. **Save** the configuration

The system stores this as the active site configuration period. Reports use the angle automatically to correct measured speeds. If you change the mounting angle later, create a new configuration period so historical reports remain accurate.

---

### Step 6: generate reports

After collecting data for a few days, generate professional reports from the dashboard.

1. Open the dashboard at [`https://velocity.local`](https://velocity.local)
2. Select your site and set the **date range** for the report period
3. Click **Generate Report**
4. Download the PDF

The report uses the cosine error angle from your site configuration (Step 5) to correct measured speeds automatically.

**What the report includes**:

- **p50 (median)**: half of vehicles go faster than this
- **p85 (traffic engineering standard)**: speed at which 85% of traffic travels at or below
- **p98 (top 2%)**: threshold where the fastest regular drivers operate
- Speed distribution histograms and time-of-day charts

The dashboard also supports **comparison reports** for measuring the effect of traffic calming interventions: select two date ranges and the report shows side-by-side metrics with percentage changes.

---

## Taking your data to city hall

Print the report. Bring it to the meeting. The data does the persuading.

Whether you are speaking at a US city council session, a UK parish council meeting, a town board hearing, or submitting written comments to a transportation committee, the approach is the same: state the measured speed, explain what it means, and make clear that you intend to keep measuring.

### What to bring

- **Printed PDF report**: a physical document can be held, marked up, filed into the public record, and passed to the person who was not at the meeting. A screen share cannot.
- **Site photos**: the street, the school, the park, the crossing. Data tells the story; photos make it concrete.
- **Before-and-after comparison** (if available): if the council has already approved changes, bring the comparison report showing whether p85 actually dropped and whether it stayed down.

### What to say

Lead with the metric traffic engineers already use:

> "85% of vehicles on [street name] travel at or below [X] mph. The posted limit is [Y] mph."

The **p85** is the standard threshold in US speed surveys (FHWA Manual on Uniform Traffic Control Devices) and UK speed assessments (Department for Transport guidance, 20 mph zone evaluations). Using it puts your request on the same footing as a professional traffic study.

If the council has already acted and you have a comparison report:

> "After the [speed hump / signage / enforcement campaign], p85 dropped from [X] to [Y] mph. [N] months later it has returned to [W] mph. The intervention slowed traffic temporarily. It has not changed the long-term behaviour."

If nothing has changed:

> "We have [N] weeks of continuous data. The p85 is [X] mph, consistently [Z] mph above the posted limit. This is not an occasional problem. It is the normal condition of this street."

### What to suggest

Present the problem first, then name what might help:

- **Speed humps or raised crossings**: reduce p85 by 5–15 mph in most studies
- **Kerb extensions (UK) / curb bulb-outs (US)**: narrow the crossing distance
- **Chicanes or lane narrowing**: reduce the straight-line path
- **20 mph (UK) / 25 mph (US) zones**: lower posted limits near schools and parks
- **Radar speed signs**: real-time driver feedback
- **Targeted enforcement**: your time-of-day data shows peak violation hours

You do not need to prescribe the answer. Present the evidence, name the options, and ask what the council can commit to and when.

### What to avoid

- **Do not share raw database files.** The PDF is the presentation format.
- **Do not identify drivers or vehicles.** The system collects no personal data, and neither should the presentation.
- **Do not lead with how the speed feels.** Lead with the measured speed. The data is more persuasive than the emotion.
- **Do not accept a one-off fix as a permanent answer.** A new speed hump slows traffic for weeks. The question is whether it still works in six months. Continuous monitoring answers that question.

### Why continuous monitoring matters

A professional speed survey gives you a snapshot: a few days of data, a report, a recommendation. velocity.report gives you the full timeline:

- **Baseline** before any intervention
- **Initial effect** in the first weeks after a change
- **Long-term compliance** months and seasons later
- **Seasonal shifts**: school terms, holidays, construction
- **Regression**: whether speeds drift back once the novelty wears off

This is the difference between asking the council to act and being able to show the council whether the action worked. Communities that keep measuring hold planners and elected officials to account: not as a confrontation, but as a continuing conversation backed by evidence. The goal is continuous safety improvement, not a single fix that goes unmonitored.

### Building support

1. **Share with neighbours**: show the dashboard or hand out a printed report
2. **Partner with local groups**: PTA, parent councils, neighbourhood associations, cycling and road safety campaigns
3. **Request official data**: file public records requests (US) or FOI requests (UK) for the council's own traffic studies, and compare
4. **Attend regularly**: present updated data at city hall or council quarterly, not once
5. **Collect across seasons**: summer and winter patterns differ; a multi-season dataset is harder to dismiss
6. **Follow up after every intervention**: generate a comparison report and present it at the next meeting

---

## Network access and security

### Local network (recommended)

The dashboard runs on HTTPS (port 443) and is accessible to any device on your local network. The Pi generates a self-signed TLS certificate on first boot; HTTP requests on port 80 redirect to HTTPS automatically. Your router firewall blocks external access by default, and no data leaves the network.

**Best practices**:

- Use SSH key authentication (configured during flashing)
- Use a strong Wi-Fi password (WPA3 if supported) if Wi-Fi is enabled
- Keep the OS updated: `sudo apt update && sudo apt upgrade`

### Remote access with Tailscale (optional)

[Tailscale](https://tailscale.com) provides secure remote access without exposing your Pi to the public internet. Free for personal use.

```bash
# Install Tailscale on your Pi
curl -fsSL https://tailscale.com/install.sh | sh

# Start Tailscale and authenticate
sudo tailscale up
```

Follow the authentication URL, install Tailscale on your phone or laptop, then access the dashboard at `https://100.x.y.z` (using the Tailscale IP from your admin console).

See the [Tailscale documentation](https://tailscale.com/kb/start) for details.

### Shared and untrusted networks

If the Pi is connected to a shared network (school, workplace, library, or multi-tenant building), other devices on that network can reach the dashboard. The dashboard has no user authentication: anyone who can reach port 443 can view your data.

**Recommended isolation**:

- **VLAN**: place the Pi on a dedicated VLAN or network segment so only your devices can reach it. Most managed switches and many consumer routers support VLAN configuration.
- **Firewall rules**: if VLAN isolation is not available, configure the router or switch to restrict access to the Pi's IP address to specific client devices.
- **Dedicated network**: for permanent installations, a small dedicated switch or router (connected to the PoE injector) keeps the Pi off the shared network entirely.

If you cannot isolate the device, use Tailscale (above) and disable the Pi's local network listener.

### Public internet

Do not expose this service to the public internet. The TLS certificate is self-signed and the dashboard has no user authentication. Use Tailscale for remote access.

---

## Updating the software

The image makes zero unsolicited network requests. Updates happen when you decide.

```bash
# Check whether a newer version is available
sudo velocity-ctl upgrade --check

# Download and apply the latest release
sudo velocity-ctl upgrade

# If something goes wrong, roll back
sudo velocity-ctl rollback
```

Updates replace the server binary and run any database migrations. Your data and configuration are preserved. For air-gapped deployments:

```bash
sudo velocity-ctl upgrade --binary /path/to/velocity-report
```

---

## Backup and restore

Back up before re-flashing. Your sensor data took weeks to collect; the software can be re-flashed in ten minutes.

### Back up your data

The database lives at `/var/lib/velocity-report/sensor_data.db`.

```bash
# Use the built-in backup tool
sudo velocity-ctl backup

# Or copy manually
sudo cp /var/lib/velocity-report/sensor_data.db \
  /tmp/sensor_data_$(date +%Y%m%d).db
```

`velocity-ctl backup` creates a timestamped copy in `/var/lib/velocity-report/backups/`. If you are about to re-flash the SD card, copy the backup off the Pi first:

```bash
# From your laptop
scp velocity@velocity.local:/var/lib/velocity-report/sensor_data.db \
  ~/sensor_data_backup_$(date +%Y%m%d).db
```

### Re-flash and restore

1. **Re-flash the SD card** using [Step 2](#step-2-flash-the-pi-image)
2. **Boot the Pi** and verify the service runs (`velocity-status`)
3. **Stop the service**: `velocity-stop`
4. **Copy the backup into place**:

```bash
# From your laptop
scp ~/sensor_data_backup_20260326.db \
  velocity@velocity.local:/tmp/sensor_data.db

# On the Pi
sudo cp /tmp/sensor_data.db /var/lib/velocity-report/sensor_data.db
sudo chown velocity:velocity /var/lib/velocity-report/sensor_data.db
```

5. **Start the service**: it detects the existing database and runs any pending migrations automatically.

```bash
velocity-start
```

---

## Troubleshooting

### Common issues

| Problem                 | Fix                                                                |
| ----------------------- | ------------------------------------------------------------------ |
| No sensor data          | Check device exists: `ls /dev/serial0` or `ls /dev/velocity-radar` |
| Service will not start  | Check logs: `velocity-log`                                         |
| Dashboard will not load | Verify service: `velocity-status`                                  |
| Certificate warning     | Download CA from `https://velocity.local/ca.crt` and trust it      |
| Garbled serial output   | Verify baud rate is 19200 (see below)                              |
| Permission denied       | Add user to dialout group: `sudo usermod -a -G dialout $USER`      |
| Still seeing CSV output | Service sets JSON mode on boot; see manual fix below               |

### Verifying the serial connection

If the sensor is not producing data, check that the serial device exists:

```bash
ls -l /dev/serial0
# Should show a link to ttyAMA0
```

If you are using a USB-serial adapter instead of a HAT, the image creates a `/dev/velocity-radar` symlink automatically when it detects the OmniPreSense sensor.

To connect directly and inspect sensor output:

```bash
stty -F /dev/serial0 19200 cs8 -parenb -cstopb
screen /dev/serial0 19200
```

You should see JSON output (`{"magnitude":1.2,"speed":3.4}`) when vehicles pass. Press Ctrl+A then K to exit `screen`.

### Configuring sensor output mode manually

The service configures the sensor automatically on boot (JSON mode, metres per second, magnitude reporting). If the sensor is producing CSV or garbled output despite a successful boot, connect via serial terminal and reconfigure manually:

```bash
stty -F /dev/serial0 19200 cs8 -parenb -cstopb
screen /dev/serial0 19200
```

Type these commands, pressing Enter after each:

```text
OJ    # Enable JSON output mode
UM    # Set units to metres per second
OM    # Enable magnitude reporting
A!    # Save settings permanently
```

Some commands produce a response (such as echoing the new setting); others are accepted silently. This is normal. After sending all four commands, verify you see JSON output (curly braces `{}`), not CSV, when vehicles pass.

### Verifying the data stream

To confirm the service is receiving data from the sensor:

```bash
velocity-log
```

You should see log entries indicating incoming radar data. If the service is running but no data appears, check the serial wiring and sensor configuration above.

**Full sensor documentation**: [OmniPreSense Support](https://www.omnipresense.com/support)

**More help**: see [TROUBLESHOOTING.md](../../../TROUBLESHOOTING.md) or ask on [Discord](https://discord.gg/XXh6jXVFkt)

---

## Maintenance

- Check the enclosure monthly for condensation
- Clean the sensor window seasonally
- Run `sudo velocity-ctl upgrade --check` periodically
- Monitor logs for issues: `journalctl -u velocity-report --since today`

A week of data shows patterns. A month is compelling. Three months across different seasons is the kind of evidence that holds up in a budget discussion.

---

## Reference

### What the image includes

| Component              | Location                                      | Purpose                                 |
| ---------------------- | --------------------------------------------- | --------------------------------------- |
| velocity-report server | `/usr/local/bin/velocity-report`              | Radar data collection and web dashboard |
| velocity-ctl           | `/usr/local/bin/velocity-ctl`                 | Device management and updates           |
| PDF generator          | `/opt/velocity-report/tools/pdf-generator/`   | Professional traffic reports            |
| Systemd service        | `/etc/systemd/system/velocity-report.service` | Starts automatically on boot            |
| Udev rules             | `/etc/udev/rules.d/99-velocity-report.rules`  | Creates `/dev/velocity-radar` symlink   |
| Nginx reverse proxy    | `/etc/nginx/sites-enabled/velocity`           | TLS termination, HTTPS on port 443      |
| TLS certificates       | `/var/lib/velocity-report/tls/`               | Self-signed CA and server certificate   |

The image also pre-configures serial port settings, UART overlays, sensor initialisation (JSON mode, units, magnitude reporting), and the service user.

### Shell aliases

The image installs these aliases for all interactive shells (via `/etc/profile.d/velocity-aliases.sh`):

| Alias             | Command                                                     |
| ----------------- | ----------------------------------------------------------- |
| `velocity-status` | `systemctl status velocity-report.service`                  |
| `velocity-log`    | `journalctl -u velocity-report.service -u nginx.service -f` |
| `velocity-bounce` | `sudo systemctl restart velocity-report.service`            |
| `velocity-stop`   | `sudo systemctl stop velocity-report.service`               |
| `velocity-start`  | `sudo systemctl start velocity-report.service`              |

### velocity-report subcommands

The server binary supports these subcommands:

| Subcommand | Purpose                                                                |
| ---------- | ---------------------------------------------------------------------- |
| `version`  | Print the installed version                                            |
| `migrate`  | Database migration management (up, down, status, version, force, etc.) |
| `transits` | Transit analytics: analyse, delete, migrate, rebuild                   |

Example:

```bash
velocity-report version
velocity-report migrate status
```

### velocity-ctl subcommands

The device management CLI supports these subcommands:

| Subcommand | Purpose                                                  |
| ---------- | -------------------------------------------------------- |
| `version`  | Print installed version                                  |
| `status`   | Show systemd service status                              |
| `upgrade`  | Check for and apply new releases (`--check` for dry run) |
| `rollback` | Restore the previous version from a timestamped backup   |
| `backup`   | Create a manual snapshot of binary and database          |

Example:

```bash
sudo velocity-ctl version
sudo velocity-ctl upgrade --check
sudo velocity-ctl backup
```

---

## Resources and links

- **GitHub repository**: [github.com/banshee-data/velocity.report](https://github.com/banshee-data/velocity.report)
- **OmniPreSense support**: [omnipresense.com/support](https://www.omnipresense.com/support)
- **Community Discord**: [discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)
- **Troubleshooting**: [TROUBLESHOOTING.md](../../../TROUBLESHOOTING.md)
- **System design**: [ARCHITECTURE.md](../../../ARCHITECTURE.md)
- **Report customisation**: [PDF Generator README](../../../tools/pdf-generator/README.md)
- **Contributing**: [CONTRIBUTING.md](../../../CONTRIBUTING.md)

---

[Back to top](#)
