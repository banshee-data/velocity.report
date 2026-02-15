---
layout: doc.njk
title: Setup your Radar
description: Build a professional traffic radar with Raspberry Pi and weatherproof infrastructure - no cameras, no cloud, just local speed data
section: guides
difficulty: intermediate
time: 4-6 hours
cost: $563-596
date: 2026-01-25
tags: [hardware, raspberry-pi, infrastructure, traffic-safety]
---

**A professional traffic logger with weatherproof infrastructure deployment that keeps data local, requires no cameras, and helps you advocate for safer streets.**

**Difficulty**: Intermediate • **Time**: 4-6 hours • **Cost**: $563-596

**In this guide**: [Parts List](#parts-and-tools-list) • [Build Steps](#step-by-step-build-guide) • [Generate Reports](#step-7-generate-pdf-reports) • [Troubleshooting](#troubleshooting)

---

## Introduction

Measuring vehicle speeds is the first step toward safer streets. Without data, convincing city officials to address speeding can be challenging.

This guide will show you how to build your own privacy-first traffic radar using open-source software and off-the-shelf Doppler technology (the same sensors municipalities use). No cameras, no license plates, just local speed data that produces professional traffic reports.

This weatherproof infrastructure deployment gives community advocates, parents, and civic-minded makers the evidence they need to drive change, with professional-grade hardware designed for permanent outdoor installations.

## Who This Guide Is For

- **Community advocates**: Get professional data for traffic calming proposals
- **Parents**: Prove speeding near schools with evidence, not emotion
- **Data enthusiasts**: Build useful civic tech with open hardware
- **Local officials**: Validate commercial traffic studies with independent data

**Not sure?** This project takes 4-6 hours and costs $563-596. If you care about street safety and need a permanent monitoring solution, you'll find it worthwhile.

## Before You Begin

**Skills required**:

- Basic Linux command line (SSH, file editing, navigation)
- Basic hardware assembly (connecting cables, mounting)
- Patience for troubleshooting (sensor configuration can be finicky)

**Tools needed**:

- Computer for flashing SD card and SSH access
- Screwdrivers for assembly
- Optional: Multimeter for troubleshooting connections

**No soldering required** • **No coding required** • **No prior radar experience needed**

## Privacy & Legal Considerations

### What This System Measures

- ✅ **Collected**: Vehicle speed, direction, timestamp
- ❌ **Not collected**: License plates, vehicle photos, driver identity
- ❌ **Not transmitted**: All data stays on your device

### Is This Legal?

**In most jurisdictions, yes.** You're measuring public behaviour on public streets, similar to what traffic engineers and academic researchers do.

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
- Selling data commercially
- Creating safety hazards

**Disclaimer**: Laws vary. When in doubt, consult local authorities or an attorney.

### Understanding Speed: The Physics Behind Street Safety

Speed isn't just a number on a sign: it's physics! And physics always wins.

The kinetic energy of a moving object follows this formula:

$$K_E = \frac{1}{2} m v^2$$

Where $m$ is mass and $v$ is velocity. The key insight: energy scales with the _square_ of velocity.

**Real-world impact**:

- A 3,000 lb sedan at **40 mph** carries **four times** the crash energy of the same car at 20 mph
- At **50 mph**, that energy jumps to **6.25 times** what it was at 20 mph
- Even a 5 mph difference (say, 30 mph vs 35 mph) increases crash energy by 36%

For anyone outside the vehicle (pedestrians, cyclists, kids) this exponential relationship is the difference between walking away and never walking again.

Streets designed for 25 mph but driven at 40? That's not just a little faster: it's 2.56× the destructive force on impact.

**Your radar measures what matters**: actual speeds, not posted limits. You'll capture the real behaviour, quantify the risk, and have data that speaks louder than feelings.

---

## What You'll Build

This guide walks you through building a professional, weatherproof traffic monitoring system:

- A Raspberry Pi radar logger that captures vehicle speeds via Doppler radar
- A SQLite database that stores detections locally (no cloud)
- A live web dashboard with real-time speeds, histograms, and time-of-day patterns
- Professional PDF reports with traffic engineering metrics (p50, p85, p98)
- Weatherproof infrastructure designed for permanent outdoor deployment

**Privacy by design**: No cameras, license plates, or identifying information—just velocity measurements.

**[PLACEHOLDER: Image showing completed infrastructure deployment (weatherproof enclosure mounted on utility pole)]**

---

## Parts and Tools List

**New to radar sensors?** The **OmniPreSense OPS7243-A-CW-R2** is recommended for infrastructure deployment. It's weatherproof (IP67), has 100m range, and handles outdoor conditions reliably.

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

**Build overview** (total time: 4-6 hours):

1. [Connect Sensor to Raspberry Pi](#step-1-connect-the-sensor-to-the-raspberry-pi) — 10-15 minutes
2. [Configure Sensor Output Mode](#step-2-configure-sensor-output-mode) — 10 minutes
3. [Verify Data Stream](#step-3-verify-data-stream) — 5 minutes
4. [Install Software](#step-4-install-software) — 30-60 minutes
5. [Access the Web Dashboard](#step-5-access-the-web-dashboard) — 5 minutes
6. [Mount the Radar Sensor](#step-6-mount-the-radar-sensor) — 15-30 minutes
7. [Generate PDF Reports](#step-7-generate-pdf-reports) — After data collection

---

### Step 1: Connect the Sensor to the Raspberry Pi

_Estimated time: 30-45 minutes_

The OPS7243-A-CW-R2 sensor (RS232 interface, designated R2) requires a serial HAT for connection to the Raspberry Pi 4.

1. **Flash Raspberry Pi OS** to your microSD card:
   - Use [Raspberry Pi Imager](https://www.raspberrypi.com/software/)
   - Choose "Raspberry Pi OS Lite" (64-bit recommended)
   - Configure WiFi and SSH in advanced settings before writing

2. **Install serial HAT on Raspberry Pi 4**:
   - Power off Pi completely
   - Attach Waveshare RS232/485 HAT to 40-pin GPIO header
   - Ensure all pins are aligned and fully seated

3. **Wire sensor to HAT**:

**[PLACEHOLDER: Diagram showing RS232 wiring connections between OPS7243 sensor and Waveshare HAT, with colour-coded wires and pin labels]**

| Sensor Pin (RS232) | HAT Terminal           | Wire Colour (typical) |
| ------------------ | ---------------------- | --------------------- |
| VCC (5V)           | +5V or separate supply | Red                   |
| GND                | GND                    | Black                 |
| TX                 | RX (receive)           | Green/Yellow          |
| RX                 | TX (transmit)          | Blue/White            |

**Critical**: RS232 uses RX↔TX crossover. Sensor TX connects to HAT RX, and vice versa.

4. **Configure Pi serial port**:

```bash
# Disable serial console to enable serial for sensor
sudo raspi-config
# Navigate to: Interface Options → Serial Port
# - "Login shell over serial?" → NO
# - "Serial port hardware enabled?" → YES
```

5. **Enable serial HAT** (add to `/boot/config.txt`):

```bash
# Edit boot configuration
sudo nano /boot/config.txt

# Add these lines at the end:
dtoverlay=uart0
enable_uart=1
```

6. **Reboot and verify**:

```bash
# Reboot to apply changes
sudo reboot

# After reboot, verify serial device exists
ls -l /dev/serial0
# Should show link to ttyAMA0 or ttyS0
```

**Success criteria**: `/dev/serial0` exists and links to a serial device

**Power considerations**:

- RS232 sensor typical draw: 300-440mA at 5V (~2.2W)
- Power from dedicated supply (not Pi's 5V pin) for stability
- Use low-voltage disconnect if running on battery/solar

**Platform note**: These commands are for Linux. On macOS, use `stty -f` instead of `stty -F`

---

### Step 2: Configure Sensor Output Mode

_Estimated time: 10 minutes_

The OmniPreSense OPS243 sensor ships with CSV output by default, but this software expects **JSON output**.

1. **Connect via serial terminal**:

   ```bash
   # Set serial port parameters
   stty -F /dev/ttyUSB0 19200 cs8 -parenb -cstopb

   # Connect to sensor (press Ctrl+A then K to exit)
   screen /dev/ttyUSB0 19200
   ```

   **Note**: On macOS, use `stty -f` instead of `stty -F`

2. **Configure sensor** (type these commands in the terminal):

   ```
   OJ    # Enable JSON output mode
   UM    # Set units to Meters per second
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

**If it's not working**:

- **Still seeing CSV?** → Type `OJ` again
- **No response?** → Verify baud rate is 19200
- **Garbled output?** → Check serial port settings

**Need more sensor info?** Type `??` to see module information or `?V` for firmware version.

**Full documentation**: [OmniPreSense Support](https://www.omnipresense.com/support)

---

### Step 3: Verify Data Stream

_Estimated time: 5 minutes_

Confirm the sensor is streaming data correctly:

```bash
# View raw sensor output
screen /dev/ttyUSB0 19200
```

**Success looks like**: You see JSON output with vehicle detections:

```json
{ "magnitude": 1.2, "speed": 3.4 }
```

When vehicles pass, you'll see more detailed information. The key is that you see JSON-formatted output (curly braces `{}`), not comma-separated values.

**If it's not working**:

- **No output at all?** → Check baud rate (19200) and port (`/dev/ttyUSB0` or `/dev/serial0`)
- **Garbled text?** → Verify sensor is in JSON mode (type `OJ` command from Step 2)
- **CSV format?** → Reconfigure sensor with `OJ` command from Step 2
- **Permission denied?** → Add your user to dialout group: `sudo usermod -a -G dialout $USER` then log out/in

---

### Step 4: Install Software

_Estimated time: 30-60 minutes_

On your Raspberry Pi:

```bash
# Clone the repository
git clone https://github.com/banshee-data/velocity.report.git
cd velocity.report

# Build the deployment tool and server binary
make build-deploy
make build-radar-linux

# Install as system service
./velocity-deploy install --binary ./velocity-report-linux-arm64
```

The installer will:

1. Install the binary to `/usr/local/bin/velocity-report`
2. Create a dedicated `velocity` service user
3. Create the data directory at `/var/lib/velocity-report/`
4. Install and start the systemd service

**Success criteria**:

```bash
# Verify service is running
sudo systemctl status velocity-report
# Should show "active (running)" in green
```

**Where files are stored**:

- **Database**: `/var/lib/velocity-report/sensor_data.db` (SQLite database with all vehicle detections)
- **PDF Reports**: Generated at `tools/pdf-generator/output/` when requested
- **Application logs**: View with `sudo journalctl -u velocity-report.service -f`

**If something goes wrong**: The installation steps use the deployment tool commands shown later in this guide. Check logs with `sudo journalctl -u velocity-report -f` (press Ctrl+C to exit)

---

### Step 5: Access the Web Dashboard

_Estimated time: 5 minutes_

Open your browser and visit:

```text
http://raspberrypi.local:8080
```

(Or use your Pi's IP address: `http://192.168.1.XXX:8080`)

**What you'll see**:

- Real-time vehicle detections with speeds and timestamps
- Speed distribution histograms
- Time-of-day traffic patterns
- Speed heatmaps

**[PLACEHOLDER: Screenshot of web dashboard showing real-time vehicle detections, speed histogram, and time-of-day traffic patterns]**

**Success criteria**: Dashboard loads and shows "No data yet" or live vehicle detections

**If dashboard won't load**:

1. Check service is running: `sudo systemctl status velocity-report`
2. Find your Pi's IP address: `hostname -I`
3. Try connecting from the Pi itself: `curl http://localhost:8080/`

If all else fails, check the logs: `sudo journalctl -u velocity-report -f`

---

### Step 6: Mount the Radar Sensor

_Estimated time: 1-2 hours for complete weatherproof installation_

**1. Prepare weatherproof enclosure**:

**Mounting preparation**:

- Drill mounting holes in back plate for hose clamps
- Install cable glands for power and (optional) Ethernet

**Sensor positioning**:

- Mount sensor inside with clear view through front panel
- Use acrylic or polycarbonate window if sensor doesn't face forward

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

**Why mount higher?** Mounting 4-8 feet off ground reduces false positives from animals, balls, or blowing debris. It provides cleaner line of sight to vehicle traffic.

**Pole mounting best practices**:

- Choose location with clear view (no trees/signs blocking)
- Ensure pole is stable (utility poles preferred over signposts)
- Check local regulations about attaching equipment to public infrastructure
- Consider solar panel if no AC power available nearby

---

### Step 7: Generate PDF Reports

_Estimated time: Varies — requires data collection period_

After collecting data for a few days or weeks, generate professional reports.

**Via Web Dashboard:**

1. Navigate to the **Reports** tab
2. Select your site from the dropdown
3. Configure report settings:
   - **Date range**: Select start and end dates for the report period
   - **Cosine angle**: Correction factor for sensor mounting angle (see below)
4. Click **Generate Report**
5. Download the PDF when ready

**Cosine angle correction**: If your sensor isn't mounted parallel to traffic flow, measured speeds will be lower than actual speeds. The cosine angle setting compensates for this. For a sensor mounted at 30° off-axis, set cosine angle to 30°—the system applies the correction factor automatically. Leave at 0° if mounted parallel to traffic.

**What's in the report**:

- **p50 (median)**: Half of vehicles go faster than this
- **p85 (traffic engineering standard)**: Speed at which 85% of traffic travels at or below
- **p98 (top 2%)**: Threshold where the fastest regular drivers operate
- Histograms, time-of-day charts, and crash physics analysis

#### Comparison Reports: Measuring Intervention Effectiveness

Comparison reports let you analyse the impact of traffic calming measures by comparing two time periods side-by-side. This is invaluable for advocacy—showing city officials that a speed hump reduced p85 speeds by 12 mph is far more compelling than anecdotal observations.

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

**Making your case**: Print the report and bring it to city council. Instead of "cars go too fast," say "85% of drivers exceed the posted 25 mph limit, with p85 at 38 mph." With comparison reports, you can add: "After the speed hump installation, p85 dropped from 42 mph to 31 mph—a 26% reduction."

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

The web dashboard runs on port 8080 and is accessible on your local network by default:

```text
http://raspberrypi.local:8080
# or
http://192.168.1.XXX:8080
```

**Security considerations for LAN-only deployment**:

- ✅ **No authentication required** if your network is trusted (home/office)
- ✅ **Router firewall** blocks external access by default
- ✅ **Data never leaves your network** - no cloud services
- ⚠️ **Anyone on your WiFi** can access the dashboard

**Best practices**:

- Use strong WiFi password (WPA3 if supported)
- Change default Pi password immediately
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

**Please, do not expose this service directly to the public internet.** The dashboard has no authentication, no HTTPS, and no rate limiting.

If you need remote access, please use [Tailscale](#remote-access-with-tailscale-optional).

---

## Using Your Data for Advocacy

### Presenting to City Council

**Do**:

- Print professional PDF reports
- Compare your data to posted speed limits
- Propose specific solutions (speed humps, signage, enforcement)
- Bring photos showing context (residential area, school zone)

**Don't**:

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
- ✅ "85% of drivers exceed the 25 mph limit, with p85 at 39 mph, well above the engineering standard for residential safety."

- ❌ "Someone's going to get hurt!"
- ✅ "At 39 mph, crash energy is 143% higher than at the posted 25 mph limit. Our data shows consistent speeding during school hours."

**[PLACEHOLDER: Photo of community member presenting PDF report at city council meeting with speed data displayed on screen]**

---

## Troubleshooting

**Most issues happen because of**:

1. Wrong baud rate (must be 19200)
2. Sensor still in CSV mode (run `OJ` command to switch to JSON)
3. Wrong device port (check `ls /dev/tty*` before and after plugging in sensor)
4. Insufficient power (use quality 2.5A+ power supply)

**Quick fixes**:

- **No sensor data?** → Check the device exists: `ls /dev/ttyUSB0`
- **Service won't start?** → Check logs: `sudo journalctl -u velocity-report -f`
- **Dashboard won't load?** → Verify service running: `sudo systemctl status velocity-report`
- **Need more help?** → See [TROUBLESHOOTING.md](../../../TROUBLESHOOTING.md) or ask on [Discord](https://discord.gg/XXh6jXVFkt)

---

## Uninstalling

To completely remove velocity.report:

```bash
# Stop and disable service
sudo systemctl stop velocity-report
sudo systemctl disable velocity-report

# Remove files
sudo rm /usr/local/bin/velocity-report
sudo rm /etc/systemd/system/velocity-report.service
sudo rm -rf /var/lib/velocity-report/

# Remove service user
sudo userdel velocity
```

**Warning**: This deletes all collected data. Export PDFs first if you want to keep them.

---

## Wrap-Up & Next Steps

You've successfully built a professional, weatherproof traffic radar system.

**What you've accomplished**:

- Built hardware for permanent neighbourhood traffic monitoring
- Configured a Doppler radar sensor with RS232 serial interface
- Deployed a complete web-based monitoring system
- Set up local data storage with no cloud dependencies
- Created a weatherproof infrastructure installation suitable for all-weather deployment

**Keep it running**: A week of data shows patterns. A month is compelling. Three months across different seasons is irrefutable evidence.

**Maintenance recommendations**:

- Check enclosure monthly for condensation
- Clean sensor lens seasonally
- Document installation with photos
- Monitor system logs for any issues
- Test weatherproofing seals periodically

**Make it count**:

Traffic safety advocacy shouldn't require a six-figure budget or an engineering degree. With ~$350-450 in parts and a weekend of work, you've built something that produces the same metrics cities pay consultants thousands for.

Show your neighbours. File public records requests to compare your data to official counts. Bring your PDF report to city council meetings. Advocate for traffic calming with evidence nobody can dismiss.

---

## Resources & Links

- **Project Overview**: See the [main README](../../../README.md) for project background and philosophy
- **GitHub Repository**: [github.com/banshee-data/velocity.report](https://github.com/banshee-data/velocity.report)
- **OmniPreSense Support**: [omnipresense.com/support](https://www.omnipresense.com/support)
- **Community Discord**: [discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)

**Related Documentation**:

- **Troubleshooting**: See [TROUBLESHOOTING.md](../../../TROUBLESHOOTING.md) for common issues
- **System Design**: Read [ARCHITECTURE.md](../../../ARCHITECTURE.md) for technical details
- **Report Customisation**: Check [PDF Generator README](../../../tools/pdf-generator/README.md)
- **Contributing**: See [CONTRIBUTING.md](../../../CONTRIBUTING.md) for conventions and workflow

**Traffic Safety Resources**:

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

Let's build safer streets together. If the speeds don't drop, the work can't stop.
