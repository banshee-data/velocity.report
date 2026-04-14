---
layout: doc.njk
title: Set Up Your Radar
description: Build a privacy-first traffic radar; 1x Pi, no cameras, no cloud, just local speed data PDFs
section: guides
difficulty: intermediate
time: 2-4 hours
cost: $647
date: 2026-04-15T12:00:00Z
tags: [hardware, raspberry-pi, infrastructure, traffic-safety]
---

**A weatherproof traffic logger that keeps data local, requires no cameras, and helps you make a data-informed case for safer streets.**

**Difficulty**: Intermediate • **Time**: 2–4 hours • **Cost**: $592–$647

---

## Introduction

Measuring vehicle speeds is the first step toward safer streets. Without data, the conversation stalls at "it feels fast" versus "the speed limit is fine."

This guide walks you through building a privacy-first traffic radar using a pre-built Raspberry Pi image and off-the-shelf Doppler technology. No cameras, no licence plates, no cloud accounts. The system starts collecting data the moment it boots.

## Before you begin

**Tools needed**:

- Computer with [Raspberry Pi Imager](https://www.raspberrypi.com/software/) installed
- 5/16" nut driver (for steel bands)
- Screwdrivers, drill, adhesive
- Optional: multimeter for testing connections

**No soldering required** 👩‍💻 **No coding required** 🛜 **No prior radar experience needed**

---

## Privacy considerations

### What this system collects

|                        |                                                          |
| ---------------------- | -------------------------------------------------------- |
| ✅ **Collected**       | Vehicle speed, direction, timestamp                      |
| ❌ **Not collected**   | No licence plates, no vehicle photos, no driver identity |
| ❌ **Not transmitted** | All data stays on your device                            |

The system records vehicle speed data without cameras, licence plates, or personal details.

### Civic position

In most jurisdictions, measuring vehicle speeds on public streets from your own property is legal: it is the same activity traffic engineers and academic researchers perform. However, you should only mount on public utility poles with explicit permission. Check local regulations for long-term installations. When in doubt, consult local authorities.

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

## Parts and tools list

The **OmniPreSense OPS7243-A-CW-R2** is recommended for infrastructure deployment: weatherproof (IP67), 100 m range, RS232 interface.

### Bill of materials

<!-- IMAGE 2: Flat-lay parts photo
     Subject: all parts from the BOM laid out on a table, labelled
     Purpose: lets the reader verify they have everything before starting
     Format: landscape, photograph, annotated with part names -->

| Part             | Recommended Model                                                                                     | Price    | Notes                                                   |
| ---------------- | ----------------------------------------------------------------------------------------------------- | -------- | ------------------------------------------------------- |
| Radar Sensor     | [OPS7243-A-CW-R2](https://omnipresense.com/product/31099/)                                            | $420     | Speed-only, RS232 interface, IP67 enclosure             |
| Mounting Plate   | [OPS100-BK](https://omnipresense.com/product/mounting-bracket-all-weather-enclosures/)                | $50      | Metal mounting bracket for OPS7243 enclosure            |
| M12 Cable        | [OPS700-CBL-M1-PT-1.8](https://omnipresense.com/product/rs-232-cable-with-m12-connector-for-ops7243/) | $17      | M12 to pigtail, connects sensor to DE-9                 |
| Raspberry Pi 4   | Raspberry Pi 4 (4 GB)                                                                                 | $45      | Also compatible with Pi 5                               |
| SD Card          | SanDisk High Endurance 32 GB                                                                          | $10      | Designed for continuous recording                       |
| PoE HAT          | Waveshare PoE HAT (F)                                                                                 | $29      | Powers the Pi over Ethernet; stacks with the serial HAT |
| Serial HAT       | Waveshare RS232/485 HAT                                                                               | $18      | Required for RS232 interface                            |
| RS-232 Connector | Adafruit DE-9                                                                                         | $3       | Connects pigtail to HAT                                 |
| **Core total**   |                                                                                                       | **$592** | Required for all deployments                            |
| Roof Rack Mount  | PVC pipe, 2×4 & hardware ([detail](#roof-rack-mount-bill-of-materials))                               | $55      | Optional: for car-mounted mobile deployment             |
| **Full total**   |                                                                                                       | **$647** | Core + roof rack mount                                  |

Power is delivered over Ethernet through the PoE HAT. You will need a PoE-capable switch or a PoE injector on the network side.

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

![HAT stacking: PoE HAT on Raspberry Pi 4, serial HAT stacked on top](/img/guide-stack.JPG)

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

The same hardware supports two deployment modes: a fixed mount at home, or a portable rig on your car roof rack. Pick whichever suits the situation — or build both and move the sensor to whichever street needs attention this month. That is what traffic engineers do, except they charge considerably more for the privilege.

#### Deployment option A: home installation (permanent)

A permanent mount on your property, aimed at the street. This is the one for long-term baselines: seasonal comparisons, before-and-after studies, and the kind of multi-month dataset that makes it difficult for a committee to claim the problem is temporary.

**Enclosure preparation**:

- Drill mounting holes in the back plate for hose clamps
- Install cable glands for power and Ethernet
- Mount the sensor inside with a clear view through the front panel
- Use plastic or nylon standoffs (metal obstructs the radar signal)

**Positioning**:

- Mount 4–8 feet off the ground (reduces false detections from small objects)
- Use two stainless steel hose clamps (top and bottom)
- Choose a location with a clear line of sight to traffic

![Radar beam cone angle: top-down view showing sensor angle to direction of travel](/img/guide-angel.png)

**Aiming**:

- **Angle**: as close to 0° (parallel with traffic flow) as practical. Lower angles produce more accurate speed measurements because they need less cosine correction. At 0°, the radar reads the full vehicle speed directly. At 30°, measured speeds are 86.6% of actual, and the correction amplifies measurement noise.
- **Road coverage**: a 0° angle gives the best accuracy but the narrowest field of view. A slight angle (10–20°) lets the radar beam sweep across the full road width, capturing vehicles in all lanes. Choose the smallest angle whose field-of-view triangle fully encompasses the lanes you need to measure.
- **Orientation**: face approaching or receding traffic (not perpendicular)
- **Record your mounting angle**: you will enter this in the dashboard as the cosine error angle (Step 5). To measure it:
  1. Stand back from the sensor and take a photo looking straight down at the road surface, perpendicular to the kerb. Include both the sensor enclosure and the road in the frame. The kerb gives you a reliable reference line.
  2. Open the photo on a phone or computer. Draw one line along the kerb (this represents 90° to traffic flow) and a second line from the sensor along its beam direction.
  3. Measure the angle between the two lines and subtract from 90° to get the angle relative to traffic flow. A phone protractor app or any image annotation tool works.

![Aiming reference: sensor beam direction relative to traffic flow on Sutro Street](/img/guide-aim-sutro.png)

**Weatherproofing checklist**:

- ✅ All cable glands sealed
- ✅ Desiccant pack inside enclosure
- ✅ Enclosure gasket intact and clean
- ✅ Seal tested before final mounting

**Success criteria**: enclosure is weatherproof, sensor aims correctly, mounting is secure

#### Deployment option B: car roof rack mount (mobile)

A portable rig that clamps to a standard roof rack. Park on any street, aim the sensor, collect data for a few hours or a few days, then drive to the next location. One sensor, many streets.

<!-- IMAGE 9: Car roof rack mount — completed build
     Subject: the assembled PVC and timber mount sitting on a workbench,
     showing the enclosure, hose clamps, corner braces, and 2×4 crossbar
     Purpose: lets the reader see the finished result before starting the build
     Format: photograph, landscape -->

<!-- IMAGE 10: Car roof rack mount — installed on vehicle
     Subject: the mount clamped to a car roof rack with the sensor enclosed
     and aimed along the street. Show the PoE cable routed into the car.
     Purpose: shows real-world deployment so readers know what to expect
     Format: photograph, landscape -->

<!-- IMAGE 11: Car roof rack mount — in operation
     Subject: the car parked on a residential street with the sensor running,
     dashboard visible on a laptop or phone inside the car
     Purpose: completes the story — build → mount → measure
     Format: photograph, landscape -->

##### Roof rack mount bill of materials

One hardware store trip. Prices from Lowe's (April 2026):

| Part                                           | Model / Item #      | Unit Price | Qty |   Total |
| ---------------------------------------------- | ------------------- | ---------: | --: | ------: |
| 4 in × 2 ft DWV foam core SCH 40 pipe          | ADS 03400 (#256096) |     $24.78 |   1 |  $24.78 |
| 2 in × 4 in × 96 in kiln-dried whitewood stud  | #26818 (#330568)    |      $3.85 |   1 |   $3.85 |
| 3 in to 5 in stainless steel adjustable clamp  | #67004 (#143645)    |      $3.78 |   2 |   $7.56 |
| 1-13/16 in to 3 in galvanised adjustable clamp | LTS 5276 (#5327202) |      $2.88 |   2 |   $5.76 |
| 3 in × 0.75 in black steel corner brace 4-pack | #22504PK (#5217432) |      $5.48 |   1 |   $5.48 |
| #8 × 1-1/8 in wood-to-wood deck screws (75-pk) | #42890 (#755725)    |      $7.98 |   1 |   $7.98 |
| **Subtotal (roof rack mount)**                 |                     |            |     | **$55** |

You will also need:

- A PoE-capable battery pack or a 12 V car outlet to PoE adapter
- A short Ethernet cable (sensor to Pi inside the car)

![Engineering drawing: isometric view of the roof rack sensor mount with bill of materials](/img/rack-drawing-iso-bom.png)

##### Building the roof rack mount

![Completed T-frame mount: 32-inch crossbar, 24-inch upright, and two 45° braces with PVC pipe](/img/guide-frame.JPG)

1. **Cut the 2×4** into three pieces: one 32 in crossbar, one 24 in upright, and two 11 in braces (measured on the top edge, with 45° miters on both ends).

2. **Assemble the T-frame.** Screw the upright to the centre of the crossbar so it stands vertical. Attach the two 45° braces — one each side — from the crossbar to the upright using corner braces and deck screws. The braces carry the load; get them tight.

3. **Cut the PVC pipe** to the length of the sensor enclosure plus 4–6 in clearance on each side. This is the sensor cradle. It sits vertically on top of the upright.

4. **Attach the PVC pipe to the upright** using the two 3–5 in stainless steel clamps. The pipe stands vertical, centred on the upright top.

5. **Seat the sensor enclosure inside the PVC pipe.** The foam core pipe has a wide enough bore for the OPS7243 enclosure.

6. **Clamp the crossbar to the roof rack** using the two 1-13/16 to 3 in galvanised clamps. Tighten firmly — the mount needs to handle wind while parked, not motorway speeds. Do not drive with the sensor running.

7. **Route the Ethernet cable** from the sensor through a rear window seal or door gap into the car. Connect to the Pi and power source inside.

![Engineering drawing: front, top, and side orthographic views of the roof rack sensor mount](/img/rack-drawing-ortho.png)

**Aiming**: park the car parallel to the kerb with the sensor aimed along the street, not across it. The same angle guidance from the home installation applies. A parked car pointed down the road is already close to 0° — which is the geometry you want.

![Aiming reference: sensor beam direction relative to traffic flow (same principle applies to roof rack mounting)](/img/guide-aim-sutro.png)

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

Print the report. Bring it to the meeting. The data does the persuading, so let it.

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

### What to avoid

- **Do not share raw database files.** The PDF is the presentation format.
- **Do not identify drivers or vehicles.** The system collects no personal data, and neither should the presentation.
- **Include site maps for public locations; leave your home address off.** When the sensor covers a school zone, a park, a commercial district, or a senior centre, the site map shows exactly where the problem is. If the sensor is mounted at your house, it's best to omit the map. Your data should make your street safer, not your house easier to find.
- **Do not lead with how the speed feels.** Lead with the measured speed. A number is harder for a committee to talk past than an anecdote.
- **Do not accept a one-off fix as a permanent answer.** A new speed hump slows traffic for weeks. The question is whether it still works in six months.

### Why continuous monitoring matters

Most speed assessments capture a few days of data. Continuous monitoring gives you the full timeline:

- **Baseline** before any intervention
- **Initial effect** in the first weeks after a change
- **Long-term compliance** months and seasons later
- **Seasonal shifts**: school terms, holidays, construction
- **Regression**: whether speeds drift back once the novelty wears off

This is the difference between asking the council to act and being able to show whether the action worked.

### Building support

Data is harder to dismiss when the room is full and the ask has been consistent for months.

1. **Share with neighbours**: show the dashboard or hand out a printed report
2. **Partner with local groups**: PTA, parent councils, neighbourhood associations, cycling and road safety campaigns
3. **Attend regularly**: present updated data quarterly, not once. Each presentation enters the public record.
4. **Widen the audience**: share the report with your state or county DOT, your elected representative, or the local press
5. **Collect across seasons**: summer and winter patterns differ; a multi-season dataset is harder to set aside
6. **Follow up after every intervention**: generate a comparison report and bring it to the next meeting

Policy changes often take multiple budget cycles. The community that keeps measuring is the one that gets heard.

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
