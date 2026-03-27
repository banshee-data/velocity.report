---
layout: doc.njk
title: Set Up Your Radar
description: Build a privacy-first traffic radar with Raspberry Pi and a pre-built image — no cameras, no cloud, just local speed data
section: guides
difficulty: intermediate
time: 2-4 hours
cost: $563-596
date: 2026-03-26
tags: [hardware, raspberry-pi, infrastructure, traffic-safety]
---

**A weatherproof traffic logger that keeps data local, requires no cameras, and helps you make the case for safer streets with evidence.**

**Difficulty**: Intermediate • **Time**: 2–4 hours • **Cost**: $563–596

**In this guide**: [Parts List](#parts-and-tools-list) • [Build Steps](#step-by-step-build-guide) • [Generate Reports](#step-7-generate-pdf-reports) • [Troubleshooting](#troubleshooting)

---

## Introduction

Measuring vehicle speeds is the first step toward safer streets. Without data, the conversation tends to stall at "it feels fast" versus "the speed limit is fine" — and feelings, however justified, do not survive contact with a council agenda.

This guide shows you how to build a privacy-first traffic radar using a pre-built Raspberry Pi image and off-the-shelf Doppler technology (the same sensors municipalities use). No cameras, no licence plates, no cloud accounts. Just local speed data that produces professional traffic reports.

The whole system is designed for permanent outdoor deployment: weatherproof hardware, a single SD card flash, and software that starts collecting data the moment it boots.

## Who This Guide Is For

- **Community advocates** who need professional data for traffic calming proposals
- **Parents** who want to prove speeding near schools with evidence, not exasperation
- **Data enthusiasts** who appreciate useful civic tech built on open hardware
- **Local officials** who want to validate commercial traffic studies independently

**Not sure?** The project takes 2–4 hours and costs $563–596. If you care about street safety and want a permanent monitoring solution, that is a reasonable afternoon.

## Before You Begin

**Skills required**:

- Basic Linux command line (SSH, file editing, navigation)
- Basic hardware assembly (connecting cables, mounting)
- Patience for troubleshooting (sensor configuration can be finicky)

**Tools needed**:

- Computer with [Raspberry Pi Imager](https://www.raspberrypi.com/software/) installed
- Screwdrivers for assembly
- Optional: Multimeter for troubleshooting connections

**No soldering required** • **No coding required** • **No prior radar experience needed**

## Privacy & Legal Considerations

### What This System Does (and Does Not) Collect

|                        |                                                          |
| ---------------------- | -------------------------------------------------------- |
| ✅ **Collected**       | Vehicle speed, direction, timestamp                      |
| ❌ **Not collected**   | No licence plates, no vehicle photos, no driver identity |
| ❌ **Not transmitted** | No upload, all data stays on your device                 |

The system records vehicle speed data without collecting camera images, licence plates, or other personal details. The point is to measure traffic, not to build a private surveillance habit.

### Is This Legal?

**In most jurisdictions, yes.** You are measuring public behaviour on public streets, similar to what traffic engineers and academic researchers do.

**Generally allowed**:

- Monitoring streets visible from your property
- Temporary studies (1-4 weeks) for community advocacy
- Presenting findings to local government
- Sharing aggregate statistics (PDF reports)

**May require permission**:

- Mounting on utility poles (contact utility company)
- Long-term installations (>1 month)
- School zones or government property

**Not allowed**:

- Monitoring private property
- Creating safety hazards

**Disclaimer**: Laws vary. When in doubt, consult local authorities and/or an attorney.

### Understanding Speed: The Physics Behind Street Safety

Speed is not just a number on a sign. It is physics, and physics does not negotiate.

The kinetic energy of a moving object follows this formula:

$$K_E = \frac{1}{2} m v^2$$

Where $m$ is mass and $v$ is velocity. The key insight: energy scales with the _square_ of velocity.

**Real-world impact**:

- A 3,000 lb sedan at **40 mph** carries **four times** the crash energy of the same car at 20 mph
- At **50 mph**, that energy jumps to **6.25 times** what it was at 20 mph
- Even a 5 mph difference (say, 30 mph vs 35 mph) increases crash energy by 36%

For anyone outside the vehicle — pedestrians, cyclists, children — this exponential relationship is the difference between walking away and never walking again.

Streets designed for 25 mph but driven at 40? That driver is doing 2.56× the destructive force should they collide with a po.

**Your radar measures what matters**: actual speeds, not posted limits. You capture real behaviour, quantify the risk, and produce data that speaks more clearly than feelings.

---

## What You Will Build

A weatherproof traffic monitoring system that runs on a Raspberry Pi and starts working the moment you plug it in:

- **Doppler radar logger** that captures vehicle speeds 24/7
- **Local SQLite database** — all data stays on the device
- **Live web dashboard** with real-time speeds, histograms, and time-of-day patterns
- **Professional PDF reports** with traffic engineering metrics (p50, p85, p98)
- **Weatherproof hardware** designed for permanent outdoor deployment

The velocity.report Pi image includes everything pre-configured: the server, the web dashboard, the PDF generator, serial port settings, and a systemd service that starts automatically on boot. You flash one SD card and connect one sensor.

**Privacy by design**: no cameras, no licence plates, no identifying information — just velocity measurements.

**[PLACEHOLDER: Image showing completed infrastructure deployment (weatherproof enclosure mounted on utility pole)]**

---

## Parts and Tools List

**New to radar sensors?** The **OmniPreSense OPS7243-A-CW-R2** is recommended for infrastructure deployment. It is weatherproof (IP67), has 100m range, and handles outdoor conditions reliably.

### Bill of Materials

| Part                 | Recommended Model                                                                                     | Price (approx) | Notes                                                                   |
| -------------------- | ----------------------------------------------------------------------------------------------------- | -------------- | ----------------------------------------------------------------------- |
| Doppler Radar Sensor | [OPS7243-A-CW-R2](https://omnipresense.com/product/31099/)                                            | $420           | Speed-only, RS232 interface (designated R2), 100m range, IP67 enclosure |
| Mounting Plate       | [OPS100-BK](https://omnipresense.com/product/mounting-bracket-all-weather-enclosures/)                | $50            | Metal mounting bracket for OPS7243 enclosure                            |
| M12 cable            | [OPS700-CBL-M1-PT-1.8](https://omnipresense.com/product/rs-232-cable-with-m12-connector-for-ops7243/) | $17            | Connects sensor to board                                                |
| Microcontroller      | Raspberry Pi 4 (4GB)                                                                                  | $45            | More reliable for 24/7 operation                                        |
| SD Card              | SanDisk High Endurance 32GB                                                                           | $10-20         | Designed for continuous recording                                       |
| Serial HAT           | Waveshare RS232/485 HAT                                                                               | $18-28         | Required for R2 (RS232) interface                                       |
| RS-232 connector     | adafruit-DE-9                                                                                         | $3-8           | Connect sensor to board                                                 |
| Power Supply         | Unifi U-PoE (15W)                                                                                     | $8             | Stable power for continuous operation                                   |
| **TOTAL**            |                                                                                                       | **$563-596**   |                                                                         |

---

### Tools Required

- Basic screwdrivers, drill, adhesive
- Computer for flashing SD card and SSH access
- 5/16" nut driver (for steel bands)
- Optional: Multimeter for testing connections

---

## Step-by-Step Build Guide

**Build overview** (total time: 2–4 hours):

1. [Flash the Pi Image](#step-1-flash-the-pi-image) — 10–15 minutes
2. [Connect Sensor to Raspberry Pi](#step-2-connect-the-sensor-to-the-raspberry-pi) — 15–30 minutes
3. [Configure Sensor Output Mode](#step-3-configure-sensor-output-mode) — 10 minutes
4. [Verify Data Stream](#step-4-verify-data-stream) — 5 minutes
5. [Access the Web Dashboard](#step-5-access-the-web-dashboard) — 5 minutes
6. [Mount the Radar Sensor](#step-6-mount-the-radar-sensor) — 1–2 hours
7. [Generate PDF Reports](#step-7-generate-pdf-reports) — After data collection

---

### Step 1: Flash the Pi Image

_Estimated time: 10–15 minutes_

The velocity.report image is a complete Raspberry Pi OS with all software pre-installed and pre-configured. Flash it to an SD card and the system is ready to run — no package installation, no build steps, no configuration files to edit.

1. **Download and install** [Raspberry Pi Imager](https://www.raspberrypi.com/software/) on your computer

2. **Open Raspberry Pi Imager** and select your Pi model (Pi 4, Pi 400, or Pi 5)

3. **Choose the velocity.report image**:
   - Click **Choose OS** → scroll to the bottom → **Use custom**
   - Download the latest `.img.xz` file from [GitHub Releases](https://github.com/banshee-data/velocity.report/releases)
   - Select the downloaded file

4. **Choose your SD card** — a 32 GB high-endurance card is recommended for continuous recording

5. **Configure Wi-Fi and SSH** (important):
   - Click the **gear icon** (⚙) or **Edit Settings** before writing
   - **Set hostname**: `velocity` (or your preference)
   - **Enable SSH**: Use password authentication
   - **Set username and password**: Choose something sensible and write it down
   - **Configure Wi-Fi**: Enter your network name and password

6. **Write** the image to the SD card

7. **Insert the SD card** into your Raspberry Pi and power it on

8. **Wait 1–2 minutes** for first boot, then verify you can connect:

```bash
ssh velocity@velocity.local
# Or use the IP address if hostname resolution does not work
```

**What the image includes** (so you do not have to):

| Component              | Location                                      | Purpose                                      |
| ---------------------- | --------------------------------------------- | -------------------------------------------- |
| velocity-report server | `/usr/local/bin/velocity-report`              | Radar data collection and web dashboard      |
| velocity-ctl           | `/usr/local/bin/velocity-ctl`                 | Device management and updates                |
| PDF generator          | `/opt/velocity-report/tools/pdf-generator/`   | Professional traffic reports                 |
| Systemd service        | `/etc/systemd/system/velocity-report.service` | Starts automatically on boot                 |
| Udev rules             | `/etc/udev/rules.d/99-velocity-report.rules`  | Creates `/dev/velocity-radar` device symlink |

The image also pre-configures serial port settings, UART overlays, and the service user. These are the things you would normally spend thirty minutes getting wrong, so they arrive done.

**Success criteria**: You can SSH into the Pi and the service is running:

```bash
sudo systemctl status velocity-report
# Should show "active (running)"
```

---

### Step 2: Connect the Sensor to the Raspberry Pi

_Estimated time: 15–30 minutes_

The OPS7243-A-CW-R2 sensor (RS232 interface) connects to the Raspberry Pi via a serial HAT. The Pi image has already configured the serial port and UART settings — you just need to wire things up.

1. **Power off** the Raspberry Pi completely

2. **Attach the serial HAT** (Waveshare RS232/485) to the 40-pin GPIO header. Ensure all pins are aligned and fully seated.

3. **Wire the sensor to the HAT**:

**[PLACEHOLDER: Diagram showing RS232 wiring connections between OPS7243 sensor and Waveshare HAT, with colour-coded wires and pin labels]**

| Sensor Pin (RS232) | HAT Terminal           | Wire Colour (typical) |
| ------------------ | ---------------------- | --------------------- |
| VCC (5V)           | +5V or separate supply | Red                   |
| GND                | GND                    | Black                 |
| TX                 | RX (receive)           | Green/Yellow          |
| RX                 | TX (transmit)          | Blue/White            |

**Critical**: RS232 uses RX↔TX crossover. Sensor TX connects to HAT RX, and vice versa.

4. **Power up** the Raspberry Pi

5. **Verify the serial device exists**:

```bash
ls -l /dev/serial0
# Should show a link to ttyAMA0
```

If you are using a USB-Serial adapter instead of a HAT, the image creates a `/dev/velocity-radar` symlink automatically when it detects the OmniPreSense sensor.

**Success criteria**: `/dev/serial0` or `/dev/velocity-radar` exists

**Power considerations**:

- RS232 sensor typical draw: 300–440 mA at 5V (~2.2W)
- Power the sensor from a dedicated supply (not the Pi's 5V pin) for stability
- Use a low-voltage disconnect if running on battery or solar

---

### Step 3: Configure Sensor Output Mode

_Estimated time: 10 minutes_

The OmniPreSense sensor ships with CSV output by default. velocity.report expects **JSON output**, so this step switches formats and saves the setting.

1. **Connect via serial terminal**:

   ```bash
   # Set serial port parameters
   stty -F /dev/serial0 19200 cs8 -parenb -cstopb

   # Connect to sensor (press Ctrl+A then K to exit)
   screen /dev/serial0 19200
   ```

2. **Configure sensor** (type these commands in the terminal):

   ```
   OJ    # Enable JSON output mode
   UM    # Set units to metres per second
   OM    # Enable magnitude reporting
   A!    # Save settings permanently
   ```

   Press Enter after each command. The sensor should respond with `O`.

3. **Verify** you see JSON output (lines with curly braces `{}`):

   ```json
   { "magnitude": 1.2, "speed": 3.4 }
   ```

**[PLACEHOLDER: Screenshot of terminal showing sensor configuration commands and JSON output verification]**

**Success criteria**: You see JSON output (not CSV) when vehicles pass

**If it is not working**:

- **Still seeing CSV?** → Type `OJ` again and press Enter
- **No response?** → Verify baud rate is 19200
- **Garbled output?** → Check serial port settings match the above

**Need more sensor info?** Type `??` to see module information or `?V` for firmware version.

**Full documentation**: [OmniPreSense Support](https://www.omnipresense.com/support)

---

### Step 4: Verify Data Stream

_Estimated time: 5 minutes_

Confirm the sensor is streaming data correctly:

```bash
screen /dev/serial0 19200
```

**Success looks like**: JSON output with vehicle detections:

```json
{ "magnitude": 1.2, "speed": 3.4 }
```

When vehicles pass, you will see more detailed information. The key is that you see JSON-formatted output (curly braces `{}`), not comma-separated values.

**If it is not working**:

- **No output at all?** → Check baud rate (19200) and port (`/dev/serial0` or `/dev/velocity-radar`)
- **Garbled text?** → Reconfigure sensor with `OJ` command from Step 3
- **CSV format?** → Run `OJ` command from Step 3
- **Permission denied?** → The image adds the default user to the `dialout` group, but if you created a different user: `sudo usermod -a -G dialout $USER` then log out and back in

---

### Step 5: Access the Web Dashboard

_Estimated time: 5 minutes_

Open a browser on any device on the same network and visit:

```text
http://velocity.local:8080
```

(Or use the Pi's IP address: `http://192.168.1.XXX:8080`)

**What you will see**:

- Real-time vehicle detections with speeds and timestamps
- Speed distribution histograms
- Time-of-day traffic patterns
- Speed heatmaps

**[PLACEHOLDER: Screenshot of web dashboard showing real-time vehicle detections, speed histogram, and time-of-day traffic patterns]**

**Success criteria**: The dashboard loads and shows "No data yet" or live vehicle detections

**If the dashboard will not load**:

1. Check the service is running: `sudo systemctl status velocity-report`
2. Find the Pi's IP address: `hostname -I`
3. Try connecting from the Pi itself: `curl http://localhost:8080/`

If none of that helps, check the logs: `sudo journalctl -u velocity-report -f`

---

### Step 6: Mount the Radar Sensor

_Estimated time: 1–2 hours for complete weatherproof installation_

**1. Prepare weatherproof enclosure**:

**Mounting preparation**:

- Drill mounting holes in back plate for hose clamps
- Install cable glands for power and (optional) Ethernet

**Sensor positioning**:

- Mount sensor inside with clear view through front panel
- Use acrylic or polycarbonate window if sensor does not face forward

**2. Install sensor inside enclosure**:

- Aim radar sensor through front of enclosure
- **Critical**: Avoid metal obstructions in front of sensor (Doppler radar uses RF energy)
- Use plastic/nylon standoffs to mount sensor board

**3. Mount enclosure to pole**:

**Positioning**:

- Mount 4-8 feet off ground (reduces false positives from small objects)
- Use two stainless steel hose clamps (top and bottom)

**Aiming**:

- **Angle**: 20-45° off-axis from traffic flow
- **Orientation**: Face oncoming OR receding traffic (not perpendicular)
- Tighten clamps securely but avoid over-tightening (can crack enclosure)
- **Record your mounting angle**: Measure and note the angle off-axis for cosine correction in site configuration

**[PLACEHOLDER: Diagram showing proper radar sensor mounting angle (20-45° off-axis from traffic flow) with top-down view of street and sensor position]**

**[PLACEHOLDER: Photo showing weatherproof enclosure mounted on utility pole with proper angle and positioning, including close-up of hose clamp mounting]**

**4. Weatherproofing checklist**:

- ✅ All cable glands properly sealed
- ✅ Desiccant pack inside enclosure
- ✅ Enclosure gasket intact and clean
- ✅ Test enclosure seal before final mounting

**Success criteria**: Enclosure is weatherproof, sensor aims correctly, mounting is secure

**Why mount higher?** At 4–8 feet off the ground, the sensor has a cleaner line of sight to vehicle traffic and fewer opinions about squirrels.

**Pole mounting best practices**:

- Choose a location with clear view (no trees or signs blocking)
- Ensure the pole is stable (utility poles preferred over signposts)
- Check local regulations about attaching equipment to public infrastructure
- Consider a solar panel if no mains power is available nearby

---

### Step 7: Generate PDF Reports

_Estimated time: Varies — requires a data collection period_

After collecting data for a few days or weeks, you can generate professional reports that speak the language traffic engineers and council members understand.

**Via Web Dashboard:**

1. Navigate to the **Reports** tab
2. Select your site from the dropdown
3. Configure report settings:
   - **Date range**: Select start and end dates for the report period
   - **Cosine angle**: Correction factor for sensor mounting angle (see below)
4. Click **Generate Report**
5. Download the PDF when ready

**Cosine angle correction**: If your sensor is not mounted parallel to traffic flow, measured speeds will be lower than actual speeds. The cosine angle setting compensates for this. For a sensor mounted at 30° off-axis, set the cosine angle to 30° — the system applies the correction factor automatically. Leave at 0° if mounted parallel to traffic.

**What is in the report**:

- **p50 (median)**: Half of vehicles go faster than this
- **p85 (traffic engineering standard)**: Speed at which 85% of traffic travels at or below
- **p98 (top 2%)**: Threshold where the fastest regular drivers operate
- Histograms, time-of-day charts, and crash physics analysis

#### Comparison Reports: Measuring Intervention Effectiveness

Comparison reports let you analyse the impact of traffic calming measures by comparing two time periods side by side. This is where the data earns its keep — showing council members that a speed hump reduced p85 speeds by 12 mph is considerably more persuasive than anyone's recollection of how things felt.

**When to use comparison reports:**

- **Before/after interventions**: Measure the effect of speed humps, signage, or enforcement campaigns
- **Seasonal comparisons**: Compare summer vs winter traffic patterns
- **Week-over-week analysis**: Track whether speeding issues are consistent or sporadic

**To generate a comparison report via Web Dashboard:**

1. Navigate to the **Reports** tab
2. Select your site from the dropdown
3. Set the **Primary period** dates (e.g., after intervention: 1-7 December 2025)
4. Enable **Compare with previous period**
5. Set the **Comparison period** dates (e.g., before intervention: 1-7 November 2025)
6. Click **Generate Report**

The report includes:

- **Side-by-side metrics**: p50, p85, p98 for both periods with percentage changes
- **Dual-period histogram**: Overlaid speed distributions with clear legend
- **Comparison distribution table**: Detailed breakdown of speed buckets across periods

**[PLACEHOLDER: Sample page from PDF report showing speed distribution histogram, p50/p85/p98 statistics, and time-of-day traffic patterns]**

**Making your case**: Print the report and bring it to council. Instead of "cars go too fast", say "85% of drivers exceed the posted 25 mph limit, with p85 at 38 mph." With comparison reports, you can add: "After the speed hump installation, p85 dropped from 42 mph to 31 mph — a 26% reduction."

---

#### Site Configuration Periods: Time-Based Sensor Settings

When you reposition your sensor or adjust its mounting angle, historical data needs to be corrected using the angle that was in effect at the time of collection. The site configuration periods feature (based on a Type 6 Slowly Changing Dimension pattern) tracks these changes automatically.

**Why configuration periods matter:**

- **Accurate historical data**: If you moved the sensor from 15° to 30° on 1st December, data before that date uses the 15° correction, and data after uses 30°
- **Retroactive corrections**: Realised your angle measurement was wrong? Update the configuration period and all reports automatically apply the correct correction
- **Comparison report accuracy**: When comparing two time periods, each period uses the appropriate cosine correction for that date range

**Managing configuration periods via Web Dashboard:**

1. Navigate to **Sites** → select your site
2. Click **Configuration Periods**
3. Add a new period with:
   - **Start date**: When this configuration became active
   - **End date**: When this configuration ended (leave blank for current)
   - **Cosine error angle**: The sensor mounting angle for this period
   - **Notes**: Optional description (e.g., "Moved sensor to east side of pole")

**Example scenario:**

| Period                   | Cosine Angle | Notes                           |
| ------------------------ | ------------ | ------------------------------- |
| 1 Jan 2025 → 15 Mar 2025 | 21°          | Initial installation            |
| 15 Mar 2025 → 1 Jun 2025 | 35°          | Repositioned after storm damage |
| 1 Jun 2025 → (current)   | 21°          | Restored to original position   |

When generating a report for April 2025, the system automatically applies the 35° correction. A comparison report spanning February (21°) vs April (35°) applies each correction independently.

---

## Network Access & Security

### Local Network Deployment (Recommended)

The web dashboard runs on port 8080 and is accessible to any device on your local network:

```text
http://velocity.local:8080
# or
http://192.168.1.XXX:8080
```

**Security considerations for LAN-only deployment**:

- ✅ **No authentication required** if your network is trusted (home or office)
- ✅ **Router firewall** blocks external access by default
- ✅ **Data never leaves your network** — no cloud services involved
- ⚠️ **Anyone on your Wi-Fi** can access the dashboard

**Best practices**:

- Use a strong Wi-Fi password (WPA3 if supported)
- Change the default Pi password immediately
- Keep Pi OS updated: `sudo apt update && sudo apt upgrade`
- Consider network segmentation for additional security

---

### Remote Access with Tailscale (Optional)

[Tailscale](https://tailscale.com) provides secure remote access from anywhere without exposing your Pi to the public internet.

**Why Tailscale?**

- Zero-configuration VPN
- End-to-end encrypted
- NAT traversal (works behind routers)
- Free for personal use (up to 20 devices)
- No port forwarding or dynamic DNS needed

**Setup** (5 minutes):

1. **Install Tailscale on your Pi**:

```bash
# Download and install Tailscale
curl -fsSL https://tailscale.com/install.sh | sh

# Start Tailscale and authenticate
sudo tailscale up
```

2. **Authenticate** via the URL shown (opens browser)

3. **Install Tailscale on your phone/laptop** from app store

4. **Access dashboard** from anywhere:

```text
# Use the Tailscale IP shown in admin console
http://100.x.y.z:8080
```

**Benefits**:

- Access dashboard while away from home
- Share access with trusted colleagues (invite to Tailscale network)
- Monitor multiple deployments from single dashboard
- No exposure to public internet scanners

**See also**: [Tailscale documentation](https://tailscale.com/kb/start)

---

### Public Internet Deployment (Not Recommended)

**Please do not expose this service directly to the public internet.** The dashboard has no authentication, no HTTPS, and no rate limiting. It was not designed for that, and it will not thank you for the experience.

If you need remote access, please use [Tailscale](#remote-access-with-tailscale-optional).

---

## Using Your Data for Advocacy

### Presenting to Council

**Do**:

- Print professional PDF reports
- Compare your data to posted speed limits
- Propose specific solutions (speed humps, signage, enforcement)
- Bring photos showing context (residential area, school zone)

**Do not**:

- Share raw database dumps
- Attack specific drivers
- Make emotional appeals without data backup
- Demand immediate action without acknowledging budget constraints

### Building Community Support

1. **Share with neighbours** - Show them the data
2. **Partner with local groups** - PTA, neighbourhood associations
3. **File public records requests** - Compare to city traffic studies
4. **Document over time** - Show patterns, not one-off incidents

### Example Talking Points

- ❌ "Cars go way too fast on our street!"
- ✅ "85% of drivers exceed the 25 mph limit, with p85 at 39 mph — well above the engineering standard for residential safety."

- ❌ "Someone's going to get hurt!"
- ✅ "At 39 mph, crash energy is 143% higher than at the posted 25 mph limit. Our data shows consistent speeding during school hours."

**[PLACEHOLDER: Photo of community member presenting PDF report at city council meeting with speed data displayed on screen]**

---

## Troubleshooting

**Most issues come down to**:

1. Wrong baud rate (must be 19200)
2. Sensor still in CSV mode (run the `OJ` command to switch to JSON)
3. Wrong device port (check `ls /dev/tty*` before and after plugging in the sensor)
4. Insufficient power (use a quality 2.5A+ power supply)

**Quick fixes**:

- **No sensor data?** → Check the device exists: `ls /dev/serial0` or `ls /dev/velocity-radar`
- **Service will not start?** → Check logs: `sudo journalctl -u velocity-report -f`
- **Dashboard will not load?** → Verify the service is running: `sudo systemctl status velocity-report`
- **Need more help?** → See [TROUBLESHOOTING.md](../../../TROUBLESHOOTING.md) or ask on [Discord](https://discord.gg/XXh6jXVFkt)

---

## Updating the Software

The image makes zero unsolicited network requests. Updates happen when you decide, not when a server somewhere feels inspired.

```bash
# Check whether a newer version is available
sudo velocity-ctl upgrade --check

# Download and apply the latest release
sudo velocity-ctl upgrade

# If something goes wrong, roll back to the previous version
sudo velocity-ctl rollback
```

Updates replace the server binary and run any database migrations. Your sensor data and configuration are preserved. If you prefer offline upgrades (air-gapped deployments, for example), you can apply a binary directly:

```bash
sudo velocity-ctl upgrade --binary /path/to/velocity-report
```

---

## Reinstalling or Starting Fresh

Since velocity.report runs from a dedicated Pi image, the simplest way to start fresh is to re-flash the SD card using Step 1.

If you want to remove velocity.report from a Pi that has other things on it, or just want to clean up:

```bash
# Stop and disable the service
sudo systemctl stop velocity-report
sudo systemctl disable velocity-report

# Remove binaries and data
sudo rm /usr/local/bin/velocity-report
sudo rm /usr/local/bin/velocity-ctl
sudo rm /etc/systemd/system/velocity-report.service
sudo rm -rf /var/lib/velocity-report/

# Remove service user
sudo userdel velocity
```

**Warning**: This deletes all collected data. Export your PDF reports first if you want to keep them.

---

## Wrap-Up & Next Steps

You have a working traffic radar.

**What you have built**:

- A weatherproof Doppler radar sensor connected to a Raspberry Pi
- A pre-configured system that collects speed data, serves a live dashboard, and generates professional reports
- Local-only data storage with no cloud dependencies
- A permanent installation suitable for all-weather deployment

**Keep it running**: A week of data shows patterns. A month is compelling. Three months across different seasons is the kind of evidence that survives contact with a budget meeting.

**Maintenance**:

- Check the enclosure monthly for condensation
- Clean the sensor lens seasonally
- Document the installation with photos
- Monitor system logs for any issues
- Run `sudo velocity-ctl upgrade --check` periodically

**Make it count**:

Traffic safety advocacy should not require a six-figure budget or an engineering degree. With roughly $560 in parts and an afternoon of work, you have built something that produces the same metrics cities pay consultants thousands for.

Show your neighbours. File public records requests to compare your data to official counts. Print the PDF report and bring it to council. The data does not care who collected it — it just needs to be accurate, and now it is.

---

## Resources & Links

- **Project overview**: See the [main README](../../../README.md) for project background and philosophy
- **GitHub repository**: [github.com/banshee-data/velocity.report](https://github.com/banshee-data/velocity.report)
- **OmniPreSense support**: [omnipresense.com/support](https://www.omnipresense.com/support)
- **Community Discord**: [discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)

**Related documentation**:

- **Troubleshooting**: See [TROUBLESHOOTING.md](../../../TROUBLESHOOTING.md) for common issues
- **System design**: Read [ARCHITECTURE.md](../../../ARCHITECTURE.md) for technical details
- **Report customisation**: Check [PDF Generator README](../../../tools/pdf-generator/README.md)
- **Contributing**: See [CONTRIBUTING.md](../../../CONTRIBUTING.md) for conventions and workflow

**Traffic safety resources**:

- Vision Zero Network: [visionzeronetwork.org](https://visionzeronetwork.org)
- NACTO Urban Street Design Guide: [nacto.org](https://nacto.org/publication/urban-street-design-guide/)
- FHWA Speed Management: [safety.fhwa.dot.gov/speedmgt](https://safety.fhwa.dot.gov/speedmgt/)

---

## Appendix: Sensor Selection Guide

### Understanding OmniPreSense Product Codes

OmniPreSense offers radar sensors in multiple configurations. The product code format is:

```
203-OPS[model]-[data_type]-[modulation]-[interface]
```

**Product Code Breakdown**:

| Component      | Options      | Meaning                                                  |
| -------------- | ------------ | -------------------------------------------------------- |
| **203-**       | Fixed prefix | Mouser manufacturer code for OmniPreSense                |
| **OPS[model]** | 243, 7243    | Sensor model (243 = standard PCB, 7243 = IP67 enclosure) |
| **Data Type**  | A, C         | A = Speed only, C = Speed + Distance                     |
| **Modulation** | CW, FC       | CW = Continuous Wave, FC = FMCW (range capability)       |
| **Interface**  | RP, WB, R2   | RP = USB, WB = USB + Bluetooth, R2 = RS-232              |

**Examples**:

- `203-OPS7243-A-CW-R2` = Sensor in IP67 housing, speed-only, continuous wave, RS232 interface
- `203-OPS7243-C-FC-R2` = Sensor in IP67 housing, speed+distance, FMCW, RS232 interface

### Available Models Comparison

| Model               | Modulation | Speed | Distance | Interface  | IP67 | Range | Price |
| ------------------- | ---------- | ----- | -------- | ---------- | ---- | ----- | ----- |
| 203-OPS7243-A-CW-R2 | Doppler    | Yes   | No       | RS232 (R2) | Yes  | 100m  | ~$415 |
| 203-OPS7243-C-FC-R2 | FMCW       | Yes   | Yes      | RS232 (R2) | Yes  | 60m   | ~$435 |

**Key specifications**:

- **A-type**: Speed only, ≈100m range (recommended for traffic monitoring)
- **C-type**: Speed + distance, ≈60m range (FMCW)
- **CW modulation**: Doppler (speed measurement)
- **FC modulation**: FMCW (adds range/frequency-modulated distance)
- **R2 interface**: RS232 industrial (requires serial HAT)
- **7243 series**: IP67 weatherproof enclosure for outdoor deployment

### Power Requirements

All models operate on **5V DC**:

**RS232 models** (R2 interface):

- Require separate 5-24V power supply
- RS232 provides data lines only, no power
- Typical draw: 300-440mA at 5V (~2.2W)

**Important**: RS232 models (R2) require external 5V power in addition to the RS232 data connection.

---

[Back to top](#)

---

Let’s build safer streets. If the speeds do not drop, the work cannot stop.
