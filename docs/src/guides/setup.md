---
layout: doc.njk
title: Setup your Citizen Radar
description: Step-by-step guide to assembling and deploying a Citizen Radar for traffic monitoring
section: guides
date: 2025-11-05
---

# **Build Your Own Privacy-First Speed Radar with Open-Source Tools**

### A DIY traffic logger that keeps data local, skips the camera, and helps your neighborhood get safer streets.

**Difficulty**: Intermediate | **Time**: 2-3 hours | **Cost**: ~$200-300

---

## **Introduction**

Ever wonder how fast cars are really going past your house or down your kid's school street? You've probably felt like drivers treat your neighborhood like a racetrack—but without hard data, it's tough to get city officials to take action.

Here's a weekend project that fixes that.

Using an off-the-shelf Doppler radar module (the same tech police use) and open-source software, you can build your own privacy-first traffic logger. No cameras, no license plates, no faces—just speed data, stored locally on a Raspberry Pi.

Think of it as a science project meets civic engagement. You'll wire up some hardware, configure a sensor over serial, build a Go application, and end up with a live web dashboard showing real-time vehicle speeds. After a few days, generate a professional PDF report with industry-standard metrics.

Whether you're a concerned parent, a local activist, or just someone who likes building useful things with a Pi and a soldering iron, this is a meaningful project with real-world impact.

### **Why Speed Matters: The Physics of Safety**

Speed isn't just a number on a sign—it's physics, and physics always wins.

The kinetic energy of a moving vehicle follows this formula:

$$K_E = \frac{1}{2} m v^2$$

Where $m$ is mass and $v$ is velocity. The key insight: energy scales with the _square_ of velocity.

**Real-world impact**:

- A 3,000 lb sedan at **40 mph** carries **four times** the crash energy of the same car at 20 mph
- At **50 mph**, that energy jumps to **6.25 times** what it was at 20 mph
- Even a 5 mph difference (say, 30 mph vs 35 mph) increases crash energy by 36%

For anyone outside the vehicle—pedestrians, cyclists, kids—this exponential relationship is the difference between walking away and not walking at all.

Streets designed for 25 mph but driven at 40? That's not just a little faster—it's 2.56× the destructive force on impact.

**Your radar measures what matters**: actual speeds, not posted limits. You'll capture the real behavior, quantify the risk, and have data that speaks louder than feelings.

---

## **What You'll Build**

By the end of this guide, you'll have:

- A Raspberry Pi-based radar logger that captures vehicle speeds via Doppler radar
- A SQLite database storing all detections locally (no cloud required)
- A live web dashboard showing real-time speeds, histograms, and time-of-day patterns
- The ability to generate professional PDF reports with traffic engineering metrics

**Technical stack**: Go backend, Python PDF generator, Svelte web frontend, SQLite database.

**Privacy by design**: No cameras. No license plates. No facial recognition. Just velocity measurements.

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
| USB Flash Drive         | Any reliable brand     | For backups (optional).                 |

### Tools

- Basic screwdrivers, drill, adhesive
- Computer for flashing/config
- Optional: multimeter, breadboard

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
   stty -F /dev/ttyUSB0 115200
   screen /dev/ttyUSB0 115200
   ```

   This opens a serial connection at 115200 baud (the sensor's default communication speed).

2. **Enter two-character commands** quickly (e.g., `VO` to switch to JSON output).

   The sensor expects complete two-character commands without delay. Type both characters rapidly.

   Common commands:

   - `VO` - Switch to JSON velocity output
   - `FV` - Display firmware version
   - `??` - Show available commands

3. **Verify response**

   The sensor will respond with its current configuration or confirmation of the change.

4. **Why JSON?** JSON output provides structured data with named fields, making it easier to parse, validate, and extend. CSV requires you to remember column positions and doesn't handle new fields gracefully.

5. **Manufacturer Docs**

   For the latest firmware and complete command reference, visit: [https://www.omnipresense.com/support](https://www.omnipresense.com/support)

---

### **Step 4: Verify Data Stream**

With the sensor powered and connected:

- Use `screen` or `cat /dev/ttyUSB0` to verify that output is streaming.
- Expect JSON lines like:

```json
{ "speed": 31.2, "units": "mph", "direction": "approaching" }
```

- If you see nothing, check:
  - Baud rate (115200)
  - Correct port (`/dev/ttyUSB0`, `/dev/serial0`, etc.)
  - Proper wiring (for UART)

---

### **Step 5: Download & Build the Software**

Head to the open-source repository and build the radar application:

```bash
git clone https://github.com/banshee-labs/velocity.report
cd velocity.report
make radar-local
```

This builds `app-radar-local` with pcap support (needed for packet capture and advanced debugging). The build process compiles the Go application into a single binary with no external dependencies.

**Installation options**:

1. **Run from current directory**: `./app-radar-local`
2. **Install system-wide**: `sudo cp app-radar-local /usr/local/bin/`
3. **Run as a service**: Set up a systemd service to start on boot (see below)

**Optional: Set up systemd service for automatic startup**

Create `/etc/systemd/system/velocity-report.service`:

```ini
[Unit]
Description=Velocity Report Traffic Logger
After=network.target

[Service]
Type=simple
User=pi
WorkingDirectory=/home/pi/velocity.report
ExecStart=/usr/local/bin/app-radar-local --input /dev/ttyUSB0 --db-path /home/pi/sensor_data.db
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable velocity-report
sudo systemctl start velocity-report
sudo systemctl status velocity-report
```

---

### **Step 6: Start Logging & Access the Web Dashboard**

1. **Start the logger**:

```bash
./app-radar-local --input /dev/ttyUSB0
```

The application will:

- Connect to the radar sensor on the specified serial port
- Create a SQLite database at `./sensor_data.db` (default location)
- Start an embedded web server on port 8000
- Begin logging vehicle detections in real-time

**Command-line options**:

- `--input /dev/ttyUSB0` - Serial port where radar is connected
- `--db-path /path/to/database.db` - Custom database location (optional)
- `--disable-radar` - Run without radar hardware for development/testing

**Understanding the database**: All vehicle detections are stored locally in a SQLite database. This means:

- No cloud uploads—your data stays on your device
- You can back up the database file to preserve your data
- You can query it directly with any SQLite tool
- The database grows over time—plan for ~1-5 MB per day of continuous logging

2. **Open your browser and visit**:

```text
http://raspberrypi.local:8000
```

(Or replace `raspberrypi.local` with the actual IP address of your Pi, e.g., `http://192.168.1.100:8000`)

**What you'll see**:

- **Recent vehicle transits**: Live feed of detected vehicles with timestamps and speeds
- **Speed heatmap**: Visual representation of when fast vehicles pass
- **Histograms**: Distribution of speeds showing how many vehicles travel at each speed
- **Time-of-day graphs**: Patterns showing when traffic is fastest/slowest

**Troubleshooting**:

- If you can't connect, check the Pi's IP address: `hostname -I`
- Make sure port 8000 isn't blocked by a firewall
- On the Pi itself, you can access `http://localhost:8000`

---

### **Step 7: Generate PDF Reports**

After collecting several days of data, you can generate professional PDF reports to share with city officials, neighbors, or community groups.

**Using the Web Dashboard** (coming soon):

1. Navigate to the **Settings** tab in the web dashboard
2. Enable **PDF Report Generation**
3. Click **Generate Report**

**Using the Command Line** (current method):

The PDF generator is a separate Python tool. See the [PDF Generator README](https://github.com/banshee-labs/velocity.report/tree/main/tools/pdf-generator) for complete instructions.

Quick version:

```bash
cd tools/pdf-generator
make install-python      # One-time setup
make pdf-config          # Create configuration template
# Edit config.json with your date range and location
make pdf-report CONFIG=config.json
```

**What's in the report**:

- **Summary statistics**:

  - Median speed (p50) - Half of vehicles travel faster than this
  - 85th percentile (p85) - The traffic engineering standard
  - 98th percentile (p98) - Where the top 2% of speeds begin
  - Maximum speed recorded

- **Visualizations**:

  - Histograms showing the full distribution of vehicle speeds
  - Time-of-day charts revealing when the fastest speeds occur

- **Scientific methodology**:
  - Explanation of Doppler radar principles
  - Discussion of kinetic energy and crash physics
  - Data collection methods and reliability

**Understanding the percentiles**:

The **85th percentile (p85)** is the traffic engineering gold standard. It's the speed at or below which 85% of vehicles travel. Many jurisdictions use p85 to set speed limits because it filters out rare extreme speeders while capturing typical "fast" traffic.

The **98th percentile (p98)** marks where the top 2% of speeds begin. This isn't the one weird outlier doing 60 in a 25 mph zone—it's the threshold where the fastest regular drivers operate. Everything from p98 to the maximum represents your most dangerous traffic.

**Making your case**: Print the report, show your neighbors, bring it to city council. Hard data changes conversations. Instead of "cars go too fast," you can say "the top 2% of drivers exceed 44 mph on a street posted for 25."

---

## **Wrap-Up & Next Steps**

Nice work! You've built a working traffic radar from scratch.

**What you've accomplished**:

- Assembled hardware that does the same job as $10k+ professional traffic counters
- Configured a Doppler radar sensor over serial
- Built and deployed a Go application with embedded web server
- Set up local data collection with SQLite (no cloud, no surveillance)
- Gained access to industry-standard traffic metrics

**Keep it running**: The longer you log, the better your data. A week shows patterns. A month is compelling. Three months across different seasons? That's irrefutable evidence.

**Ideas for expansion**:

- **Multiple sensors**: Compare speeds on different streets or at different times
- **GPS logging**: Track sensor location for mobile deployments
- **Solar power**: Deploy in locations without electrical access (add a battery + solar panel)
- **Data sharing**: Export anonymized datasets for traffic safety research
- **Community network**: Coordinate with neighbors to build comprehensive coverage

**Make it count**:

You've got data now. Use it.

Traffic safety advocacy shouldn't require a six-figure budget or an engineering degree. With $200 in parts and a weekend of hacking, you've built something that produces the same quality metrics cities pay consultants thousands for.

Show your neighbors. File a public records request to compare your data to official counts. Bring your PDF to a city council meeting. Advocate for traffic calming with evidence nobody can dismiss.

Let's build safer streets, one Pi at a time.

---

## **Resources & Links**

- **OmniPreSense OPS243 Support**: [https://www.omnipresense.com/support](https://www.omnipresense.com/support)

  - Firmware updates, datasheets, and configuration guides

- **GitHub Repository**: [https://github.com/banshee-labs/velocity.report](https://github.com/banshee-labs/velocity.report)

  - Source code, issue tracker, and contribution guidelines

- **Community Discord**: [https://discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)

  - Get help, share your deployments, discuss traffic safety advocacy

- **Project Website**: [https://velocity.report](https://velocity.report)
  - Documentation, guides, and sample reports

**Further Reading on Traffic Safety**:

- Vision Zero Network: [https://visionzeronetwork.org](https://visionzeronetwork.org)
- NACTO Urban Street Design Guide: [https://nacto.org/publication/urban-street-design-guide/](https://nacto.org/publication/urban-street-design-guide/)
- FHWA Speed Management Guide: [https://safety.fhwa.dot.gov/speedmgt/](https://safety.fhwa.dot.gov/speedmgt/)
