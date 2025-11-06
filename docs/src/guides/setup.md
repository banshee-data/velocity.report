---
layout: doc.njk
title: Setup your Citizen Radar
description: Step-by-step guide to assembling and deploying a Citizen Radar for traffic monitoring
section: guides
date: 2025-11-05
---

# **Build Your Own Privacy-First Speed Radar with Open-Source Tools**

### A DIY traffic logger that keeps data local, skips the camera, and helps your neighborhood get safer streets.

**Difficulty**: Intermediate | **Time**: 2-4 hours | **Cost**: ~$XXX-YYY

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

### Hardware

| Part                    | Example Model          | Notes                                   |
| ----------------------- | ---------------------- | --------------------------------------- |
| Doppler Radar Sensor    | OmniPreSense OPS243A   | Outputs JSON via USB/serial.            |
| Microcontroller/SBC     | Raspberry Pi 4 or 3B+  | Or similar Linux-capable board.         |
| Power Supply            | 12V 2A adapter or HAT  | Stable voltage is important—see notes.  |
| Enclosure & Mount       | Waterproof project box | Optional: suction mount, tripod, etc.   |
| Optional Serial Adapter | FTDI USB-to-Serial     | Needed if not using GPIO/UART directly. |
| SD Card                 | 16GB+                  | For Pi OS and data logging.             |

### Tools

- Basic screwdrivers, drill, adhesive
- Computer for flashing/config
- Optional: multimeter for testing connections

---

## **Step-by-Step Build Guide**

### **Step 1: Mount the Radar Sensor**

Secure the radar sensor inside your enclosure or mount it directly on a bracket. Make sure it's:

- **Facing directly at oncoming or passing traffic**: The Doppler effect works by measuring the change in frequency of radio waves bouncing off moving objects. For accurate readings, the sensor needs a clear view of vehicles as they approach or recede.
- **Positioned at a fixed angle (ideally 20°–45° off-axis)**: Mounting perpendicular to traffic (90°) won't work—Doppler radar measures the component of motion toward or away from the sensor. A slight angle ensures you're measuring meaningful velocity.
- **Clear of obstructions (no walls, poles, or trees blocking view)**: Radio waves can be absorbed or reflected by obstacles, reducing accuracy or causing false readings.

**Why mount higher?** Mounting the sensor 3-6 feet off the ground helps reduce false positives from small objects like animals, bouncing balls, or blowing debris. It also provides a cleaner line of sight to vehicle traffic.

---

### **Step 2: Connect the Sensor to the Raspberry Pi**

Wire up the sensor’s pins or USB cable depending on your model:

- **USB version (e.g. OPS243A)**: Plug it directly into the Pi’s USB port.
- **UART version (e.g. OPS7243C)**: Connect to GPIO pins as follows:

| Radar Pin | Pi GPIO Pin | Description           |
| --------- | ----------- | --------------------- |
| VCC       | 5V (Pin 2)  | Power (check voltage) |
| GND       | GND (Pin 6) | Ground                |
| TX        | RX (Pin 10) | Data from sensor      |
| RX        | TX (Pin 8)  | Data to sensor        |

📌 **Note**: If you see the sensor rebooting constantly or not outputting data, it may be undervolted. Consider using a HAT or USB-to-serial adapter with isolated 12V rail.

---

### **Step 3: (Optional) Flash Firmware & Set Output Mode**

The OmniPreSense OPS243 radar sensor can output data in multiple formats. Most sensors ship configured for CSV output, but our software works best with **JSON output** for easier parsing and better data structure.

Here's how to check and update the output mode:

1. **Connect via terminal**

   ```bash
   stty -F /dev/ttyUSB0 19200 cs8 -parenb -cstopb
   screen /dev/ttyUSB0 19200
   ```

2. **Configure sensor** (type these two-character commands quickly):

   ```
   OJ    # Enable JSON output mode
   US    # Set units to MPH
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
