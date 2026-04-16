---
layout: doc.njk
title: Set Up Your Radar
description: Build a privacy-first traffic radar; 1x Pi, no cameras, no cloud, just local speed data PDFs
section: guides
difficulty: intermediate
time: 2-4 hours
cost: $650
date: 2026-04-15T12:00:00Z
tags: [hardware, raspberry-pi, infrastructure, traffic-safety]
---

**A weatherproof traffic logger that keeps data local, requires no cameras, and helps you make a data-informed case for safer streets.**

**Difficulty**: Intermediate • **Time**: 2–4 hours • **Cost**: $640

---

## Introduction

Safer streets start with measured traffic speeds.

One afternoon, a Raspberry Pi, and a radar sensor. By the evening you will have a speed monitor logging every vehicle that passes, a live dashboard, and the beginnings of the dataset that goes to your next council meeting. No cameras, no licence plates, no cloud accounts: just local speed data on hardware you own.

## Before you begin

**Tools needed**:

- Computer with [Raspberry Pi Imager](https://www.raspberrypi.com/software/) installed
- 5/16" nut driver (steel bands)
- Screwdriver
- Handsaw or circular saw
- Drill and bits
- Pencil and tape measure
- Optional: mitre box
- Optional: multimeter

**No soldering required** · **No coding required** · **No prior radar experience needed**

---

## Privacy considerations

### What this system collects

|                        |                                                          |
| ---------------------- | -------------------------------------------------------- |
| ✅ **Collected**       | Vehicle speed, direction, timestamp                      |
| ❌ **Not collected**   | No licence plates, no vehicle photos, no driver identity |
| ❌ **Not transmitted** | All data stays on your device                            |

### Compliance

Measuring vehicle speeds on public streets from your own property is generally legal. Mounting on public utility poles requires explicit permission. Always check your local regulations.

---

## What you will build

![Complete assembled velocity.report unit: weatherproof enclosure mounted on a pole with cables routed](/img/guide-hero.jpg)

- **Doppler radar logger**: captures vehicle speeds 24/7
- **Local SQLite database**: all data stays on the device
- **Live web dashboard**: real-time speeds, histograms, time-of-day patterns
- **Professional PDF reports**: traffic engineering metrics (p50, p85, p98)
- **Weatherproof hardware**: designed for permanent outdoor deployment

The velocity.report Pi image includes everything pre-configured: flash one SD card, connect one sensor, and it starts collecting.

---

## Parts list

<!-- IMAGE 2: Flat-lay parts photo
     Subject: all parts from the BOM laid out on a table, labelled
     Purpose: lets the reader verify they have everything before starting
     Format: landscape, photograph, annotated with part names -->

| Part                    | Part No.                                                                                                                                        | Price    | Notes                                          |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ---------------------------------------------- |
| Radar sensor            | [OPS7243-A-CW-R2](https://omnipresense.com/product/31099/)                                                                                      | $420     | IP67, 100 m range, RS232                       |
| Sensor mounting bracket | [OPS100-BK](https://omnipresense.com/product/mounting-bracket-all-weather-enclosures/)                                                          | $50      | Metal bracket for OPS7243 enclosure            |
| Sensor cable            | [OPS700-CBL-M1-PT-1.8](https://omnipresense.com/product/rs-232-cable-with-m12-connector-for-ops7243/)                                           | $17      | M12 to pigtail; connects sensor to serial HAT  |
| Server                  | [Raspberry Pi 4 (2Gb)](https://www.raspberrypi.com/products/raspberry-pi-4-model-b/)                                                            | $55      | Also compatible with Pi 5                      |
| PoE HAT (802.3af/at)    | [Waveshare PoE HAT (F) #26399](https://www.waveshare.com/poe-hat-f.htm)                                                                         | $20      | Powers the Pi + sensor over Ethernet           |
| Serial HAT              | [Waveshare RS232 HAT #17498](https://www.waveshare.com/product/iot-communication/wired-comm-converter/rs232-rs485-can-dali2/2-ch-rs232-hat.htm) | $16      | RS232 interface for the sensor                 |
| Serial connector        | [Adafruit DE-9](https://www.adafruit.com/product/3123)                                                                                          | $3       | Connects sensor cable to serial HAT            |
| SD card                 | [SanDisk High Endurance (32 GB)](https://www.sandisk.com/products/memory-cards/microsd-cards/sandisk-high-endurance-uhs-i-microsd)              | $23      | High-endurance; designed for continuous writes |
| **Core total**          |                                                                                                                                                 | **$604** |                                                |
| ABS pipe (4in × 2ft)    | [Lowe's #256096](https://www.lowes.com/pd/Charlotte-Pipe-4-in-x-2-ft-ABS-DWV-Pipe/3415778)                                                      | $25      | Sensor mast                                    |
| Timber stud (2×4, 8ft)  | [Lowe's #330568](https://www.lowes.com/pd/Unbranded-2-4-8-KD-DF-SELECT-STUD/5003667531)                                                         | $4       | Crossbar and upright                           |
| Small hose clamps ×2    | [Lowe's #5327202](https://www.lowes.com/pd/RELIABILT-Indoor-Hook-up-and-Outdoor-Exhaust-Dryer-Vent-Kit/5014298699)                              | $6       | Clamp crossbar to roof rack bars               |
| Corner braces (4-pack)  | [Lowe's #5217432](https://www.lowes.com/pd/RELIABILT-3-in-x-0-75-in-x-3-in-Gauge-Black-Steel-Corner-Brace-4-Pack/5013834841)                    | $5       | Brace the T-frame                              |
| Deck screws (67-pack)   | [Lowe's #5333958](https://www.lowes.com/pd/Grip-Rite-8-x-1-5-8-in-Wood-To-Wood-Deck-Screws-67-Per-Box/5014220681)                               | $6       | Frame assembly                                 |
| **Full total**          |                                                                                                                                                 | **$650** | Core + roof rack mount                         |

Power is delivered over Ethernet via the PoE HAT. You will need a PoE-capable switch or injector on the network side. For mobile deployment you will also need a PoE battery pack or 12 V car outlet adapter, and a short Ethernet cable.

---

## Step-by-step build guide

<div class="not-prose gradient-border rounded-lg p-5 my-6 text-sm leading-relaxed w-fit">
<p class="font-semibold text-gray-900 dark:text-gray-100 mb-3">Build overview <span class="font-normal text-gray-500">(total time: 2–4 hours)</span></p>
<ol class="list-decimal list-inside space-y-1 text-gray-600 dark:text-gray-300">
<li><a href="#step-1-wire-the-sensor-to-the-raspberry-pi" class="link">Wire the Sensor to the Raspberry Pi</a>: 15–30 minutes</li>
<li><a href="#step-2-flash-the-pi-image" class="link">Flash the Pi Image</a>: 10–15 minutes</li>
<li><a href="#step-3-access-the-web-dashboard" class="link">Access the Web Dashboard</a>: 5 minutes</li>
<li><a href="#step-4-mount-the-radar-sensor" class="link">Mount the Radar Sensor</a>: 1–2 hours</li>
<li><a href="#step-5-configure-your-site" class="link">Configure Your Site</a>: 10 minutes</li>
<li><a href="#step-6-generate-reports" class="link">Generate Reports</a>: after data collection</li>
</ol>
</div>

### Step 1: wire the sensor to the Raspberry Pi

_Estimated time: 15–30 minutes_

The OPS7243-A-CW-R2 sensor connects to the Raspberry Pi via an RS232 serial HAT. The PoE HAT stacks on top to provide power over Ethernet.

![HAT stacking: PoE HAT on Raspberry Pi 4, serial HAT stacked on top](/img/guide-stack.jpg)

1. **Attach the PoE HAT** to the Raspberry Pi's 40-pin GPIO header

2. **Stack the serial HAT** (Waveshare RS232/485) on top of the PoE HAT. Ensure all pins are aligned and fully seated.

3. **Wire the sensor to the HAT** following the wiring diagram below:

<!-- IMAGE 4: M12 and DE-9 connector pinout diagrams
     Subject: face-on view of M12 4-pin connector and DE-9 connector with
     coloured pins matching wire colours (brown/black/blue/white)
     Purpose: makes it obvious which wire goes where without reading a table
     Format: SVG, generated (see connector diagram tooling) -->

![Radar wiring diagram: M12 cable from OPS7243 sensor through pigtail and DE-9 connector to Waveshare RS232 HAT](/img/radar-wiring.svg)

---

### Step 2: flash the Pi image

_Estimated time: 10–15 minutes_

The velocity.report image is a complete Raspberry Pi OS with everything pre-installed and pre-configured; flash it to an SD card and the system is ready to run.

#### Option A: use the custom Raspberry Pi Imager catalogue (recommended)

This opens Raspberry Pi Imager with the velocity.report image pre-loaded:

**macOS**

```bash
cd "/Applications/Raspberry Pi Imager.app/Contents/MacOS/" && \
  ./rpi-imager --repo https://velocity.report/rpi.json
```

**Linux**

```bash
rpi-imager --repo https://velocity.report/rpi.json
```

**Windows**

```bash
"C:\Program Files (x86)\Raspberry Pi Imager\rpi-imager.exe" --repo https://velocity.report/rpi.json
```

![Screenshot of the Raspberry Pi Imager](/img/rpi-imager.png)

1. **Select your Pi model**
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
```

To watch live logs:

```bash
velocity-log
```

The image installs shell aliases for common operations: `velocity-status`, `velocity-log`, `velocity-bounce` (restart), `velocity-stop`, and `velocity-start`. See the [Reference](#reference) section for the full list.

---

### Step 3: access the web dashboard

_Estimated time: 5 minutes_

Open a browser on any device on the same network: [https://velocity.local](https://velocity.local)

The Pi generates a self-signed TLS certificate on first boot. Your browser will show a certificate warning: this is expected. To eliminate the warning, download the CA certificate from `https://velocity.local/ca.crt` and add it to your browser or system trust store.

**Success criteria**: the dashboard loads and shows live vehicle detections or "No data yet"

![Web dashboard showing live speed detections, histogram, and time-of-day chart](/img/guide-dash-screenshot.png)

**If the dashboard will not load**:

1. Check the service is running: `velocity-status`
2. Find the Pi's IP address: `hostname -I`
3. Test from the Pi itself: `curl -k https://localhost/`
4. Check logs: `velocity-log`

---

### Step 4: mount the radar sensor

_Estimated time: 1–2 hours_

The same hardware supports two deployment modes: a portable rig on your car roof rack, or a fixed mount at home. Pick whichever suits the situation, or build both and move the sensor to whichever street needs attention this month.

#### Deployment option A: car roof rack mount (mobile)

A portable rig that clamps to a standard roof rack. Park on any street, collect data for a few hours or a few days, then drive to the next location. One sensor, many streets.

Parts are listed in the [parts list](#parts-list) above. You will also need a PoE battery pack or 12 V car outlet adapter, and a short Ethernet cable from sensor to Pi.

![Engineering drawing: isometric view of the roof rack sensor mount with bill of materials](/img/rack-drawing-iso-bom.png)

##### Building the roof rack mount

![Completed T-frame mount: 32-inch crossbar, 24-inch upright, and two 45° braces with PVC pipe](/img/guide-frame.jpg)

1. **Cut the 2×4** into three pieces: one 32 in crossbar, one 24 in upright, and two 11 in braces (measured on the top edge, with 45° miters on both ends).

2. **Assemble the T-frame.** Screw the upright to the centre of the crossbar so it stands vertical. Attach the two 45° braces (one on each side) from the crossbar to the upright using corner braces and deck screws. The braces carry the load; get them tight.

3. **Drill mounting holes in the pipe.** Mark two positions on the pipe where it will overlap the upright: 6 inches and 12 inches down from the top end of the upright. At each position, drill a pilot hole through both walls of the pipe and into the upright wood so the screw seats cleanly and the pipe cannot rotate.

4. **Attach the PVC pipe to the upright** using the two 3–5 in stainless steel clamps. The pipe stands vertical, overlapping the top of the upright. Thread each clamp through its pilot holes and tighten.

5. **Attach the sensor to the pipe.** Using hose clamps secure the sensor to the pipe.

6. **Clamp the crossbar to the roof rack** using the two 1-13/16 to 3 in galvanised clamps. Before drilling the clamp holes in the crossbar, measure the centre-to-centre distance between your roof rack bars. The drawing shows holes spaced 14 inches apart; adjust the spacing to match your own rack so the bolts land squarely on the bars. Tighten firmly: the mount needs to handle wind while parked, not motorway speeds. Do not drive with the sensor running.

7. **Route the cable** from the sensor through a rear window seal or door gap into the car. Connect to the Pi and power source inside.

#### Deployment option B: home installation (permanent)

A permanent mount on your property, aimed at the street. This is the one for long-term baselines: seasonal comparisons, before-and-after studies, and the kind of multi-month dataset that builds a compelling case over time.

**Positioning**:

- Mount 4–8 feet off the ground (reduces false detections from small objects)
- Use two stainless steel hose clamps (top and bottom)
- Choose a location with a clear line of sight to traffic
- Use plastic or nylon standoffs (metal obstructs the radar signal)

**Weatherproofing checklist**:

- ✅ All cable glands sealed
- ✅ Enclosure gasket intact and clean
- ✅ Seal tested before final mounting

**Success criteria**: enclosure is weatherproof, sensor aims correctly, mounting is secure

#### Aiming the sensor

Once the sensor is mounted, aim it before collecting data. These guidelines apply to both deployments. For the mobile mount, parking parallel to the kerb already puts you close to 0°.

![Aiming reference: sensor beam direction relative to traffic flow on Sutro Street](/img/guide-aiming.jpg)

- **Angle**: as close to 0° (parallel with traffic flow) as practical, typically 15–25°. Lower angles need less cosine correction, so the speed readings are more accurate.
- **Road coverage**: the beam must encompass the full lane of traffic you are measuring. Angling the sensor 15–25° widens the field-of-view triangle enough to capture vehicles across the lane while keeping cosine error small. Choose the smallest angle whose beam triangle fully covers the lane.
- **Orientation**: face approaching or receding traffic (not perpendicular)

![Radar beam cone angle: top-down view showing sensor angle to direction of travel](/img/guide-cosign-angle.jpg)

<div class="not-prose gradient-border rounded-lg p-5 my-6 text-sm leading-relaxed max-w-[60%] mx-auto">
<p class="font-semibold text-gray-900 dark:text-gray-100 mb-3">Recording your mounting angle</p>
<p class="text-gray-600 dark:text-gray-300 mb-3">You will enter this in the dashboard as the cosine error angle (Step 5).</p>
<ol class="list-decimal list-inside space-y-2 text-gray-600 dark:text-gray-300">
<li>Take a photo looking straight down at the road surface, perpendicular to the kerb. Include the sensor and the road in the frame.</li>
<li>Draw one line along the kerb and a second line from the sensor along its beam direction.</li>
<li>Measure the angle between the two lines and subtract from 90°. A phone protractor app works.</li>
</ol>
</div>

---

### Step 5: configure your site

_Estimated time: 10 minutes_

Before generating reports, configure your site in the dashboard so speed measurements are corrected for your mounting angle.

1. Open the dashboard at [https://velocity.local](https://velocity.local)
2. Navigate to **Site Settings**
3. Set the **site location** on the interactive map
4. Set the **cosine error angle** to match your radar mounting angle from Step 4. Drag the red dot on the radar field-of-view triangle to adjust the angle visually, or type the value directly. The triangle should encompass the road lanes you want to measure.
5. Add any **notes** about the installation (useful for later reference)
6. **Save** the configuration

The system stores this as the active site configuration period. Reports use the angle automatically to correct measured speeds. If you change the mounting angle later, create a new configuration period so historical reports remain accurate.

---

### Step 6: generate reports

After collecting data for a few days, generate professional reports from the dashboard.

1. Open the dashboard at [https://velocity.local](https://velocity.local)
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

## Take your data to city hall

Print the report. Bring it to the meeting. The data does the persuading.

Whether you are speaking at a city council session, a town board hearing, a parish council meeting, or submitting written comments to a transportation committee, the approach is the same: state the measured speed, explain what it means, and make clear that you intend to keep measuring.

### What to bring

![Sample PDF report page showing speed histogram, p85 metric, and site map](/img/stack.png)

- **Printed PDF report**: a physical document can be held, marked up, filed into the public record, and passed to the person who was not at the meeting. A screen share cannot.
- **Site photos**: the street, the school, the park, the crossing. Data tells the story; photos make it concrete.
- **Before-and-after comparison** (if available): if the council has already approved changes, bring the comparison report showing whether p85 actually dropped and whether it stayed down.

### What to say

Lead with the metric traffic engineers already use:

> "The 85th-percentile speed on [street name] is [X] mph. The posted limit is [Y] mph."

The **p85** (85th-percentile speed) is the standard threshold in US federal speed surveys and UK Department for Transport assessments. Using it means your data speaks the same language as a professional traffic study.

Then adapt to the situation:

- **If the intervention worked**: "After the [speed hump / signage / enforcement], p85 dropped from [X] to [Y] mph."
- **If it worked and then wore off**: "p85 dropped to [Y] mph initially. [N] months later it has returned to [W] mph."
- **If nothing has changed**: "We have [N] weeks of continuous data. The p85 is [X] mph, consistently [Z] mph above the posted limit. This is the normal condition of this street."

### What to suggest

Present the problem first, then name what might help:

- **Speed humps or raised crossings**: reduce p85 by 5–15 mph in most studies
- **Curb bulb-outs (US) / kerb extensions (UK)**: narrow the crossing distance
- **Chicanes or lane narrowing**: reduce the straight-line path
- **20 mph zones**: lower posted limits near schools and parks
- **Radar speed signs**: real-time driver feedback
- **Targeted enforcement**: your time-of-day data shows peak violation hours

You do not need to prescribe the answer. Present the evidence, name the options, and ask what the council can commit to and when.

### Presenting effectively

- **Share the PDF, not the database.** The report is the presentation format; raw files invite misinterpretation.
- **Keep it about speed, not about drivers.** The system collects no personal data, and neither should the presentation.
- **Include site maps for public locations; leave your home address off.** When the sensor covers a school zone, a park, a commercial district, or a senior centre, the site map shows exactly where the problem is. If the sensor is mounted at your house, omit the map. Your data should make your street safer, not your house easier to find.
- **Lead with the measured speed.** A number is harder for a committee to talk past than an anecdote.
- **Keep measuring after the fix.** A new speed hump slows traffic for weeks. The question is whether it still works in six months.

### Why continuous monitoring matters

Most speed assessments capture a few days of data. Continuous monitoring gives you the full timeline:

- **Baseline** before any intervention
- **Initial effect** in the first weeks after a change
- **Long-term compliance** months and seasons later
- **Seasonal shifts**: school terms, holidays, construction
- **Regression**: whether speeds drift back once the novelty wears off

This is the difference between asking the council to act and being able to show whether the action worked.

### Building support

Data carries more weight when more people stand behind it. You do not need to be an organiser to build support; you just need to share what you have found.

1. **Share with neighbours**: show the dashboard or hand out a printed report
2. **Partner with local groups**: PTA, parent councils, neighbourhood associations, cycling and road safety campaigns
3. **Attend regularly**: present updated data quarterly, not once. Each presentation enters the public record.
4. **Widen the audience**: share the report with your state or county DOT, your elected representative, or the local press
5. **Collect across seasons**: summer and winter patterns differ; a multi-season dataset is harder to set aside
6. **Follow up after every intervention**: generate a comparison report and bring it to the next meeting

Policy changes often take multiple budget cycles. The community that keeps measuring is the one that gets heard.

A week of data shows patterns. A month is compelling. Three months across different seasons is the kind of evidence that holds up in a budget discussion.

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
sudo velocity-ctl backup
```

This creates a timestamped copy in `/var/lib/velocity-report/backups/`. If you are about to re-flash the SD card, copy the backup off the Pi first:

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

| Problem                 | Fix                                                               |
| ----------------------- | ----------------------------------------------------------------- |
| No sensor data          | `ls /dev/serial0` or `ls /dev/velocity-radar` to check the device |
| Service will not start  | `velocity-log` to check logs                                      |
| Dashboard will not load | `velocity-status` to verify the service                           |
| Certificate warning     | Download CA from `https://velocity.local/ca.crt` and trust it     |
| Garbled or CSV output   | Connect via `screen /dev/serial0 19200` and send `OJ` then `A!`   |
| Permission denied       | `sudo usermod -a -G dialout $USER` then log out and back in       |

USB-serial adapters get a `/dev/velocity-radar` symlink automatically.

- **Full sensor documentation**: [OmniPreSense Support](https://www.omnipresense.com/support)
- **More help**: [TROUBLESHOOTING.md](../../../TROUBLESHOOTING.md) or [Discord](https://discord.gg/XXh6jXVFkt)

---

## Maintenance

- Check the enclosure monthly for condensation
- Clean the sensor window seasonally
- Run `sudo velocity-ctl upgrade --check` periodically

---

## Reference

### What the image includes

| Component              | Location                                      | Purpose                                 |
| ---------------------- | --------------------------------------------- | --------------------------------------- |
| velocity-report server | `/usr/local/bin/velocity-report`              | Radar data collection and web dashboard |
| velocity-ctl           | `/usr/local/bin/velocity-ctl`                 | Device management and updates           |
| PDF generator          | `/opt/velocity-report/tools/pdf-generator/`   | Professional traffic reports            |
| Systemd service        | `/etc/systemd/system/velocity-report.service` | Starts automatically on boot            |
| Nginx reverse proxy    | `/etc/nginx/sites-enabled/velocity`           | TLS termination, HTTPS on port 443      |
| TLS certificates       | `/var/lib/velocity-report/tls/`               | Self-signed CA and server certificate   |

The image also pre-configures serial port settings, UART overlays, sensor initialisation (JSON mode, units, magnitude reporting), and the service user.

### Commands

| Command                             | Purpose                                                     |
| ----------------------------------- | ----------------------------------------------------------- |
| `velocity-status`                   | `systemctl status velocity-report.service`                  |
| `velocity-log`                      | `journalctl -u velocity-report.service -u nginx.service -f` |
| `velocity-bounce`                   | `sudo systemctl restart velocity-report.service`            |
| `velocity-stop`                     | `sudo systemctl stop velocity-report.service`               |
| `velocity-start`                    | `sudo systemctl start velocity-report.service`              |
| `velocity-report version`           | Print the installed server version                          |
| `sudo velocity-ctl upgrade --check` | Check whether a newer release is available                  |
| `sudo velocity-ctl upgrade`         | Download and apply the latest release                       |
| `sudo velocity-ctl rollback`        | Restore the previous version                                |
| `sudo velocity-ctl backup`          | Create a timestamped snapshot of binary and database        |

---

## Links

- **GitHub repository**: [github.com/banshee-data/velocity.report](https://github.com/banshee-data/velocity.report)
- **OmniPreSense support**: [omnipresense.com/support](https://www.omnipresense.com/support)
- **Community Discord**: [discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)
- **Troubleshooting**: [TROUBLESHOOTING.md](../../../TROUBLESHOOTING.md)
- **System design**: [ARCHITECTURE.md](../../../ARCHITECTURE.md)
- **Report customisation**: [PDF Generator README](../../../tools/pdf-generator/README.md)
- **Contributing**: [CONTRIBUTING.md](../../../CONTRIBUTING.md)

---

[Back to top](#)
