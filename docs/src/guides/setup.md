---
layout: doc.njk
title: Setup your Citizen Radar
description: Step-by-step guide to assembling and deploying a Citizen Radar for traffic monitoring
section: guides
date: 2025-11-05
---

# **Build Your Own Privacy-First Speed Radar with Open-Source Tools**

### A DIY traffic logger that keeps data local, skips the camera, and helps your neighborhood get safer streets.

**Difficulty**: Intermediate | **Time**: 2-4 hours (DIY) or 4-6 hours (Infrastructure)

---

## **Choose Your Deployment**

This guide covers two deployment options:

### **DIY Deployment** (~$150-200)

- **Best for**: Temporary monitoring, testing locations, short-term studies
- **Hardware**: Raspberry Pi Zero 2 W, USB radar sensor (OPS243 A-CW-RP or A-CW-WB)
- **Enclosure**: 3D printed case (no weatherproofing)
- **Mounting**: 1/4-20 tripod mount (camera tripod compatible)
- **Pros**: Lower cost, portable, easy to relocate
- **Cons**: Indoor or sheltered outdoor only, less stable mounting

### **Infrastructure Deployment** (~$350-450)

- **Best for**: Permanent installations, long-term monitoring, all-weather deployment
- **Hardware**: Raspberry Pi 4, radar sensor (OPS7243-A-CW-R2 for 100m range), serial HAT
- **Enclosure**: IP67 waterproof housing
- **Mounting**: Pole clamps for street/utility poles (adjustable diameter)
- **Pros**: Weatherproof, stable mounting, professional appearance, long range (100m)
- **Cons**: Higher cost, harder to relocate

**Choose your path and follow the corresponding instructions below.**

---

## **Introduction**

Ever wonder how fast cars are really going past your house or down your kid's school street? You've probably felt like drivers treat your neighborhood like a racetrack—but without hard data, it's tough to get city officials to take action.

Here's a weekend project that fixes that.

Using an off-the-shelf Doppler radar module (the same tech police use) and open-source software, you can build your own privacy-first traffic logger. No cameras, no license plates—just speed data stored locally on a Raspberry Pi.

You'll wire up hardware, configure a sensor, and deploy a web dashboard showing real-time vehicle speeds. After collecting data for a few days or weeks, generate professional PDF reports with industry-standard traffic metrics.

Whether you're a concerned parent, a local activist, or someone who likes building useful things, this is a meaningful project with real-world impact.

### **Why Speed Matters: The Physics of Safety**

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

**Your radar measures what matters**: actual speeds, not posted limits. You'll capture the real behavior, quantify the risk, and have data that speaks louder than feelings.

---

## **What You'll Build**

By the end of this guide, you'll have:

- A Raspberry Pi radar logger capturing vehicle speeds via Doppler radar
- A SQLite database storing detections locally (no cloud)
- A live web dashboard with real-time speeds, histograms, and time-of-day patterns
- Professional PDF reports with traffic engineering metrics (p50, p85, p98)

**Privacy by design**: No cameras, license plates, or identifying information—just velocity measurements.

---

## **Parts and Tools List**

### **Understanding OmniPreSense Product Codes**

OmniPreSense radar sensors are available in multiple configurations. The product code format is:

```
203-OPS[model]-[data_type]-[modulation]-[interface]
```

**Product Code Breakdown**:

| Component      | Options      | Meaning                                              |
| -------------- | ------------ | ---------------------------------------------------- |
| **203-**       | Fixed prefix | Mouser manufacturer code for OmniPreSense            |
| **OPS[model]** | 243, 7243    | Sensor model (243 = standard, 7243 = IP67 enclosure) |
| **Data Type**  | A, C         | A = Speed only, C = Speed + Distance                 |
| **Modulation** | CW, FC       | CW = Continuous Wave, FC = FMCW (range capability)   |
| **Interface**  | RP, WB, R2   | RP = USB, WB = USB + Bluetooth, R2 = RS-232          |

**Examples**:

- `203-OPS243-A-CW-RP` = Sensor PCB, speed-only, continuous wave, USB interface
- `203-OPS7243-C-FC-R2` = Sensor in IP67 housing, speed+distance, FMCW, RS232 interface

---

### **Available Models**

All available OmniPreSense radar sensors we document:

| Model               | Type / Modulation         | Interface  | IP67 | Range | Price     |
| ------------------- | ------------------------- | ---------- | ---- | ----- | --------- |
| 203-OPS243-A-CW-RP  | A / CW — Speed only       | USB (RP)   | No   | 100m  | ~$100-130 |
| 203-OPS243-C-FC-RP  | C / FC — Speed + Distance | USB (RP)   | No   | 60m   | ~$130-160 |
| 203-OPS7243-A-CW-R2 | A / CW — Speed only       | RS232 (R2) | Yes  | 100m  | ~$150-180 |
| 203-OPS7243-C-FC-R2 | C / FC — Speed + Distance | RS232 (R2) | Yes  | 60m   | ~$150-180 |
| 203-OPS7243-C-FC-RP | C / FC — Speed + Distance | USB (RP)   | Yes  | 60m   | ~$150-180 |

**Key specifications (short)**:

- A = speed only (≈100 m). C = speed + distance (≈60 m).
- CW = Doppler (speed). FC = FMCW (adds range/frequency-modulated distance).
- RP = USB plug-and-play. R2 = RS232 industrial (use a serial HAT).
- WB (Bluetooth/Wi‑Fi) variants are omitted from our recommendations.

---

### **Our Recommendations**

#### **DIY Deployment (~$150-200)**

**Best choice**: `203-OPS243-A-CW-RP`

- Affordable (~$100–130), USB plug-and-play, speed-only (recommended for most DIY use).

**Alternative**: `203-OPS243-C-FC-RP`

- Adds distance (FMCW) if you need it (60 m). We do not recommend WB (wireless) variants.

#### **Infrastructure Deployment (~$350-450)**

**Best choice**: `203-OPS7243-A-CW-R2`

- 100 m range, industrial RS232, robust for outdoor installations. Requires a serial HAT and enclosure.

**Alternative**: `203-OPS7243-C-FC-R2` (60 m) if you need distance measurements.

---

### **Quick Decision Guide**

**I want the cheapest option that works**:
→ `203-OPS243-A-CW-RP` (~$100-130)

**I need maximum range (100 m) for outdoor installation**:
→ `203-OPS7243-A-CW-R2` (RS232, requires serial HAT)

**I want distance measurement too**:
→ `203-OPS243-C-FC-RP` (60 m)

**I need long range for an outdoor installation**:
→ `203-OPS7243-A-CW-R2` (~$415, includes IP67 weatherproof enclosure)

**I don't know which to choose**:
→ `203-OPS243-A-CW-RP` is the safe, budget-friendly choice

---

### **Power Requirements**

All models operate on **5V DC**:

-- **RP/ENC interface models (USB)**: Draw power directly from USB connection (5V via USB)

- **R2 interface models (RS232)**: Require separate 5-24V power supply (RS232 data lines only, no power)
- **Typical draw**: 300-440mA at 5V (~2.2W)

**Important**: The USB interface models (RP, WB, ENC) are powered via USB. The RS232 models (R2) require external 5V power in addition to the RS232 data connection.

---

### **DIY Deployment Bill of Materials**

| Part                 | Example Model/Part Number                   | Price (approx) | Notes                                 |
| -------------------- | ------------------------------------------- | -------------- | ------------------------------------- |
| Doppler Radar Sensor | OPS243-A-CW-RP (Mouser: 203-OPS243-A-CW-RP) | ~$100-130      | Speed-only (A), USB interface (RP)    |
| Microcontroller      | Raspberry Pi Zero 2 W                       | ~$15-20        | WiFi built-in for dashboard access    |
| Power Supply         | 5V 2.5A USB-C adapter                       | ~$10-15        | Official Pi power supply recommended  |
| SD Card              | SanDisk 32GB microSD                        | ~$8-12         | A1/A2 rated for better performance    |
| USB Cable (optional) | USB-A to micro-USB or USB-C                 | ~$5-10         | If sensor doesn't include USB cable   |
| Tripod (optional)    | Desktop or camera tripod                    | ~$10-25        | Standard 1/4-20 threading             |
| **TOTAL**            |                                             | **~$150-210**  | Add $20-30 for WB variant w/Bluetooth |

**Alternative sensors**:

-- **With distance measurement**: `203-OPS243-C-FC-RP` (~$130-160) - adds range capability (FMCW)

**3D printing files**: Available at [project repository](https://github.com/banshee-data/velocity.report/tree/main/hardware/enclosures)

**Note**: DIY deployment is **not fully weatherproof**. Deploy indoors (e.g., window facing street) or under shelter only.

---

### **Infrastructure Deployment Bill of Materials**

| Part                 | Example Model/Part Number                     | Price (approx) | Notes                                  |
| -------------------- | --------------------------------------------- | -------------- | -------------------------------------- |
| Doppler Radar Sensor | OPS7243-A-CW-R2 (Mouser: 203-OPS7243-A-CW-R2) | ~$415          | Speed-only (A), RS232 (R2), 100m range |
| Microcontroller      | Raspberry Pi 4 (4GB)                          | ~$55-75        | More reliable for 24/7 operation       |
| Serial HAT           | Waveshare RS232/485 HAT                       | ~$25-35        | Required for R2 (RS232) interface      |
| Power Supply         | 5V 4A industrial adapter                      | ~$20-30        | Stable power for continuous operation  |
| SD Card              | SanDisk High Endurance 64GB                   | ~$15-20        | Designed for continuous recording      |
| Cable Glands         | PG11 cable glands (2-pack)                    | ~$8-12         | Weatherproof cable entry               |
| Pole Mount           | Stainless steel hose clamps                   | ~$10-15        | 2-4" diameter range                    |
| Mounting Plate       | Aluminum or HDPE plate                        | ~$10-20        | Custom cut to fit enclosure            |
| **TOTAL**            |                                               | **~$341-459**  | Using OPS7243-A-CW-R2 (100m range)     |

**Alternative sensors**:

- **USB instead of RS232**: `203-OPS7243-C-FC-R2` (~$435) - 60m range, adds distance measurement

**Note**: The A-type sensor (OPS7243-A-CW-R2) provides 100m range vs 60m for C-type sensors. For outdoor traffic monitoring, the longer range is more important than distance measurement capability.

**Alternative sensors**:

- **USB instead of RS232**: `203-OPS7243-C-FC-RP` (~$150-180) - No HAT required, still needs weatherproof enclosure

**Note**: R2 (RS232) interface provides the most robust industrial-grade connection for permanent outdoor installations, but USB interfaces (RP/WB) are simpler to set up if you don't need the extra reliability.

**Pole mounting**: Standard utility poles are typically 4-6" diameter. Adjustable hose clamps provide secure, non-invasive mounting.

**Power options**: For locations without AC power, consider:

- Solar panel + battery (add ~$80-150)
- PoE HAT + PoE injector (add ~$40-60, requires Ethernet run)

---

### Tools (Both Deployments)

- Basic screwdrivers, drill, adhesive
- Computer for flashing/config
- Optional: multimeter for testing connections

---

## **Step-by-Step Build Guide**

### **Step 1: Mount the Radar Sensor**

#### **DIY Deployment: Tripod Mount**

1. **Install threaded insert** in 3D printed case bottom:

   - Use 1/4-20 threaded insert (standard camera tripod size)
   - Heat insert with soldering iron and press into mounting hole
   - Ensure insert is flush and threads are clean

2. **Mount to tripod**:

   - Use any standard camera tripod or desktop tripod
   - Position near window facing street
   - Ensure sensor has clear view through glass (Doppler radar works through windows)

3. **Aiming the sensor**:
   - **Angle**: 20-45° off-axis from traffic flow (NOT perpendicular)
   - **Height**: Window height is typically fine; avoid ground-level clutter
   - **Clear view**: No curtains, screens, or obstructions between sensor and street

**Limitation**: DIY deployment works through windows but range may be reduced. Glass causes some signal attenuation.

---

#### **Infrastructure Deployment: Pole Mount**

1. **Prepare weatherproof enclosure**:

   - Drill mounting holes in back plate for hose clamps
   - Install cable glands in appropriate positions for power and (optional) Ethernet
   - Mount sensor inside enclosure with clear view through front panel
   - Consider acrylic or polycarbonate window if sensor doesn't face forward

2. **Position sensor correctly inside enclosure**:

   - Radar sensor should aim through front of enclosure
   - **Critical**: Doppler radar uses RF energy; avoid metal obstructions in front of sensor
   - Use plastic/nylon standoffs to mount sensor board

3. **Mount enclosure to pole**:

   - Position 4-8 feet off ground (reduces false positives from small objects)
   - Use two stainless steel hose clamps (top and bottom of enclosure)
   - **Angle**: 20-45° off-axis from traffic flow
   - **Orientation**: Sensor should face oncoming OR receding traffic (not perpendicular)
   - Tighten clamps securely but avoid over-tightening (can crack enclosure)

4. **Weatherproofing checklist**:
   - All cable glands properly sealed
   - Desiccant pack inside enclosure
   - Enclosure gasket intact and clean
   - Test enclosure seal before final mounting

**Why mount higher?** Mounting 4-8 feet off ground helps reduce false positives from small objects like animals, bouncing balls, or blowing debris. It also provides a cleaner line of sight to vehicle traffic.

**Pole mounting best practices**:

- Choose location with clear view of traffic (no trees/signs blocking)
- Ensure pole is stable (utility poles preferred over signposts)
- Check local regulations about attaching equipment to public infrastructure
- Consider solar panel if no AC power available nearby

---

### **Step 2: Connect the Sensor to the Raspberry Pi**

#### **DIY Deployment: USB Connection (Pi Zero)**

The OPS243/OPS7243 A-CW series sensors with RP, WB, or ENC interfaces use USB connection:

1. **Flash Raspberry Pi OS** to your microSD card:

   - Use [Raspberry Pi Imager](https://www.raspberrypi.com/software/)
   - Choose "Raspberry Pi OS Lite" (64-bit recommended for Pi Zero 2 W)
   - Configure WiFi and SSH in advanced settings before writing

2. **Connect sensor**:

   - Plug sensor's USB connector directly into Pi Zero's USB port
   - Use USB OTG adapter if needed
   - Sensor will appear as `/dev/ttyUSB0` or `/dev/ttyACM0`

3. **Power on**:
   - Connect 5V power supply to Pi's USB-C port
   - Wait for boot (first boot takes 1-2 minutes)
   - SSH into Pi: `ssh pi@raspberrypi.local`

**Verify sensor connection**:

```bash
ls /dev/tty* | grep -E 'ttyUSB|ttyACM'
# Should show /dev/ttyUSB0 or similar
```

---

#### **Infrastructure Deployment: RS232 Serial Connection (Pi 4)**

The OPS7243-A-CW-R2 sensor (or OPS7243-C-FC-R2 variant) uses RS232 serial, requiring a serial HAT:

1. **Install serial HAT on Raspberry Pi 4**:

   - Power off Pi completely
   - Attach Waveshare RS232/485 HAT to 40-pin GPIO header
   - Ensure all pins are aligned and fully seated

2. **Wire sensor to HAT**:

| Sensor Pin (RS232) | HAT Terminal           | Wire Color (typical) |
| ------------------ | ---------------------- | -------------------- |
| VCC (5V)           | +5V or separate supply | Red                  |
| GND                | GND                    | Black                |
| TX                 | RX (receive)           | Green/Yellow         |
| RX                 | TX (transmit)          | Blue/White           |

**Critical**: RS232 uses RX↔TX crossover. Sensor TX connects to HAT RX, and vice versa.

3. **Configure Pi serial port**:

```bash
# Disable serial console (enables serial for sensor)
sudo raspi-config
# Navigate to: Interface Options → Serial Port
# - "Login shell over serial?" → NO
# - "Serial port hardware enabled?" → YES
```

4. **Enable serial HAT** (add to `/boot/config.txt`):

```bash
sudo nano /boot/config.txt
# Add these lines:
dtoverlay=uart0
enable_uart=1
```

5. **Reboot and verify**:

```bash
sudo reboot
# After reboot:
ls -l /dev/serial0
# Should show link to ttyAMA0 or ttyS0
```

**Power considerations**:

- RS232 sensor draws ~150mA at 5V
- Power from dedicated supply (not Pi's 5V pin) for stability
- Use low-voltage disconnect if running on battery/solar

---

### **Step 3: Configure Sensor Output Mode**

The OPS243 sensor ships with CSV output by default, but this software expects **JSON output**.

1. **Connect via serial terminal**:

   ```bash
   stty -F /dev/ttyUSB0 19200 cs8 -parenb -cstopb
   screen /dev/ttyUSB0 19200
   ```

2. **Configure sensor** (type these two-character commands quickly):

   ```
   OJ    # Enable JSON output mode
   UM    # Set units to Meters per second (software default)
   OM    # Enable magnitude reporting
   A!    # Save configuration to memory
   ```

3. **Verify** you see JSON output:

   ```json
   { "magnitude": 1.2, "speed": 3.4 }
   ```

**Common commands**:

- `??` - Module information
- `?V` - Firmware version
- `Om` - Disable magnitude (if too noisy)
- `AX` - Reset to factory defaults

**Full documentation**: [OmniPreSense Support](https://www.omnipresense.com/support)

---

### **Step 4: Verify Data Stream**

Confirm the sensor is streaming data:

```bash
cat /dev/ttyUSB0
# or
screen /dev/ttyUSB0 19200
```

You should see JSON output like:

```json
{ "magnitude": 1.2, "speed": 3.4 }
```

For detected vehicles, expect detailed transit data:

```json
{
  "classifier": "object_outbound",
  "end_time": "1750719826.467",
  "start_time": "1750719826.031",
  "delta_time_msec": 736,
  "max_speed_mps": 13.39,
  "min_speed_mps": 11.33,
  "max_magnitude": 55,
  "avg_magnitude": 36,
  "total_frames": 7,
  "frames_per_mps": 0.5228,
  "length_m": 9.86,
  "speed_change": 2.799
}
```

**Troubleshooting**:

- No output? Check baud rate (19200) and port (`/dev/ttyUSB0` or `/dev/serial0`)
- Garbled output? Verify sensor is in JSON mode (`OJ` command)
- For RS232, verify TX/RX crossover wiring

---

### **Step 5: Install Software**

On your Raspberry Pi:

```bash
git clone https://github.com/banshee-data/velocity.report.git
cd velocity.report
make build-radar-linux
sudo ./scripts/setup-radar-host.sh
```

The setup script will:

1. Install the binary to `/usr/local/bin/velocity-report`
2. Create a dedicated `velocity` service user
3. Create the data directory at `/var/lib/velocity-report/`
4. Install and start the systemd service
5. Optionally migrate an existing database

**Where files are stored**:

- **Database**: `/var/lib/velocity-report/sensor_data.db` (SQLite database with all vehicle detections)
- **PDF Reports**: Generated in the repository at `tools/pdf-generator/output/` when requested via web dashboard or command line
- **Application logs**: View with `sudo journalctl -u velocity-report.service -f`

**Useful commands**:

```bash
sudo systemctl status velocity-report    # Check status
sudo systemctl restart velocity-report   # Restart
sudo journalctl -u velocity-report -f   # View logs
```

---

### **Step 6: Access the Web Dashboard**

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

**Troubleshooting**:

- Check service is running: `sudo systemctl status velocity-report`
- View logs: `sudo journalctl -u velocity-report -f`
- Find Pi's IP address: `hostname -I`
- Ensure port 8080 isn't blocked by a firewall

---

### **Step 7: Generate PDF Reports**

After collecting data for a few days or weeks, generate professional reports.

**Via Web Dashboard**:

1. Navigate to the **Sites** tab
2. Configure your site details
3. Click **Generate Report**

**Via Command Line**:

```bash
make install-python      # One-time: install dependencies
make pdf-config          # Create config template
# Edit config.json with date range and location
make pdf-report CONFIG=config.json
```

See the [PDF Generator README](https://github.com/banshee-data/velocity.report/tree/main/tools/pdf-generator) for details.

**What's in the report**:

- **p50 (median)**: Half of vehicles go faster than this
- **p85 (traffic engineering standard)**: Speed at which 85% of traffic travels at or below
- **p98 (top 2%)**: Threshold where the fastest regular drivers operate
- Histograms, time-of-day charts, and crash physics analysis

**Making your case**: Print the report and bring it to city council. Instead of "cars go too fast," say "85% of drivers exceed the posted 25 mph limit, with p85 at 38 mph."

---

## **Network Access & Security**

### **Local Network Deployment (Recommended)**

By default, the web dashboard runs on port 8080 and is accessible on your local network:

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

---

### **Remote Access with Tailscale (Optional)**

For secure remote access from anywhere without exposing your Pi to the public internet, use [Tailscale](https://tailscale.com):

**Why Tailscale?**

- Zero-configuration VPN
- End-to-end encrypted
- NAT traversal (works behind routers)
- Free for personal use (up to 20 devices)
- No port forwarding or dynamic DNS needed

**Setup** (5 minutes):

1. **Install Tailscale on your Pi**:

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
```

2. **Authenticate** via the URL shown (opens browser)

3. **Install Tailscale on your phone/laptop** from app store

4. **Access dashboard** from anywhere:

```text
http://100.x.y.z:8080
# (Use the Tailscale IP shown in admin console)
```

**Benefits**:

- Access dashboard while away from home
- Share access with trusted colleagues (invite to Tailscale network)
- Monitor multiple deployments from single dashboard
- No exposure to public internet scanners

**See also**: [Tailscale documentation](https://tailscale.com/kb/start)

---

### **Public Internet Deployment (Not Recommended)**

**Do not expose this service directly to the public internet.** The dashboard has no authentication, no HTTPS, and no rate limiting.

If you need public access, use Tailscale (above) or set up a reverse proxy with:

- HTTPS (Caddy/nginx with Let's Encrypt)
- Authentication (HTTP basic auth minimum)
- Rate limiting

See [Security Analysis](https://github.com/banshee-data/velocity.report/blob/main/docs/security-analysis.md) for detailed hardening guidance.

---

## **Legal & Privacy Considerations**

### **What This System Collects**

- ✅ Vehicle speed and direction
- ✅ Timestamp of detection
- ❌ **No** cameras, license plates, or identifying information
- ❌ **No** cloud transmission - all data stays local

This privacy-first design is legal for civic use in most jurisdictions. You're measuring public behavior on public streets, similar to what traffic engineers do.

### **Know Your Local Rules**

**We are not lawyers.** Before deploying, check if you need permission for:

- Mounting equipment on utility poles (usually requires permission)
- Long-term installations in public spaces
- School zones or government property

**Generally OK**: Monitoring the street in front of your home for community advocacy, temporary studies (1-4 weeks), presenting findings to local government.

**Not OK**: Monitoring private property, interfering with traffic devices, creating safety hazards.

### **Use Data Responsibly**

- Share aggregate statistics (PDF reports), not raw database dumps
- Focus on safety improvements, not shaming individuals
- Inform neighbors about your monitoring project
- Be transparent about methodology and limitations

**Disclaimer**: This is not legal advice. Laws vary by location and use case. When in doubt, consult local authorities or an attorney.

---

## **Wrap-Up & Next Steps**

Nice work! You've built a working traffic radar from scratch.

**What you've accomplished**:

- Built hardware equivalent to $10k+ professional traffic counters
- Configured a Doppler radar sensor (USB or RS232)
- Deployed a complete web-based monitoring system
- Set up local data storage with no cloud dependencies
- (Infrastructure only) Created a weatherproof permanent installation

**Keep it running**: A week of data shows patterns. A month is compelling. Three months across different seasons is irrefutable evidence.

**Deployment-specific tips**:

- **DIY**: Move sensor to test different locations, monitor during different times (school hours, weekends), bring indoors during bad weather
- **Infrastructure**: Check enclosure monthly for condensation, clean sensor lens seasonally, document installation with photos

**Make it count**:

Traffic safety advocacy shouldn't require a six-figure budget or an engineering degree. With $150-450 in parts and a weekend of work, you've built something that produces the same metrics cities pay consultants thousands for.

Show your neighbors. File public records requests to compare your data to official counts. Bring your PDF report to city council meetings. Advocate for traffic calming with evidence nobody can dismiss.

---

## **Resources & Links**

- **GitHub Repository**: [github.com/banshee-data/velocity.report](https://github.com/banshee-data/velocity.report)
- **OmniPreSense Support**: [omnipresense.com/support](https://www.omnipresense.com/support)
- **Community Discord**: [discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)
- **Project Website**: [velocity.report](https://velocity.report)

**Traffic Safety Resources**:

- Vision Zero Network: [visionzeronetwork.org](https://visionzeronetwork.org)
- NACTO Urban Street Design Guide: [nacto.org](https://nacto.org/publication/urban-street-design-guide/)
- FHWA Speed Management: [safety.fhwa.dot.gov/speedmgt](https://safety.fhwa.dot.gov/speedmgt/)

---

Let's build safer streets, one Pi at a time.
