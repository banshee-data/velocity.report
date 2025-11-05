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

Using an off-the-shelf Doppler radar module (the same tech police use) and open-source software, you can build your own privacy-first traffic logger. No cameras, no license plates, no faces—just speed data, stored locally on a Raspberry Pi.

Think of it as a science project meets civic engagement. You'll wire up some hardware, configure a sensor over serial, build a Go application, and end up with a live web dashboard showing real-time vehicle speeds. After a few days, generate a professional PDF report with industry-standard metrics.

Whether you're a concerned parent, a local activist, or just someone who likes building useful things with a Pi and a soldering iron, this is a meaningful project with real-world impact.

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
   stty -F /dev/ttyUSB0 19200 cs8 -parenb -cstopb
   screen /dev/ttyUSB0 19200
   ```

   This opens a serial connection at 19200 baud with 8 data bits, no parity, and 1 stop bit (the sensor's default communication settings).

2. **Enter two-character commands** quickly (e.g., `OJ` to switch to JSON output).

   The sensor expects complete two-character commands without delay. Type both characters rapidly.

   **Essential commands**:

   - `??` - Query overall module information
   - `?V` - Read firmware version
   - `OJ` - Enable JSON output mode
   - `OM` - Enable magnitude reporting (Doppler)
   - `Om` - Disable magnitude reporting (Doppler)
   - `A!` - Save current configuration to persistent memory
   - `A?` - Query persistent memory settings
   - `AX` - Reset flash settings to factory defaults

   **Additional useful commands**:

   - `?R` - Read reset reason
   - `US` - Set units to miles per hour
   - `R?` - Query current speed filter settings
   - `PA` - Set active power mode

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
{ "magnitude": 1.2, "speed": 3.4 }
```

For detected vehicle objects (transits), you'll see more detailed JSON:

```json
{
  "classifier": "object_outbound",
  "end_time": "1750719826.467",
  "start_time": "1750719826.731",
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

- If you see nothing, check:
  - Baud rate (19200)
  - Correct port (`/dev/ttyUSB0`, `/dev/serial0`, etc.)
  - Proper wiring (for UART)

---

### **Step 5: Download & Install the Software**

On your Raspberry Pi, clone the repository and run the automated setup:

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
sudo systemctl status velocity-report    # Check service status
sudo systemctl restart velocity-report   # Restart the service
sudo journalctl -u velocity-report -f   # View live logs
```

---

### **Step 6: Access the Web Dashboard**

The service starts automatically after installation and runs on port 8000.

**Open your browser and visit**:

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

- Check service is running: `sudo systemctl status velocity-report`
- View logs: `sudo journalctl -u velocity-report -f`
- Find Pi's IP address: `hostname -I`
- Ensure port 8000 isn't blocked by a firewall

---

### **Step 7: Generate PDF Reports**

After collecting several days of data, you can generate professional PDF reports to share with city officials, neighbors, or community groups.

**Using the Web Dashboard**

1. Configure a **Site** tab in the web dashboard
2. Enable **PDF Report Generation**
3. Click **Generate Report**

**Using the Command Line**

The PDF generator uses the repository's unified Python environment. From the repository root:

```bash
make install-python      # One-time setup: creates .venv/ with all dependencies
make pdf-config          # Create configuration template
# Edit config.json with your date range and location
make pdf-report CONFIG=config.json
```

**Note**: The Python environment is created at the repository root (`.venv/`) and is shared across all Python tools including the PDF generator, data visualization scripts, and analysis utilities.

See the [PDF Generator README](https://github.com/banshee-data/velocity.report/tree/main/tools/pdf-generator) for complete instructions.

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

- **GitHub Repository**: [https://github.com/banshee-data/velocity.report](https://github.com/banshee-data/velocity.report)

  - Source code, issue tracker, and contribution guidelines

- **Community Discord**: [https://discord.gg/XXh6jXVFkt](https://discord.gg/XXh6jXVFkt)

  - Get help, share your deployments, discuss traffic safety advocacy

- **Project Website**: [https://velocity.report](https://velocity.report)
  - Documentation, guides, and sample reports

**Further Reading on Traffic Safety**:

- Vision Zero Network: [https://visionzeronetwork.org](https://visionzeronetwork.org)
- NACTO Urban Street Design Guide: [https://nacto.org/publication/urban-street-design-guide/](https://nacto.org/publication/urban-street-design-guide/)
- FHWA Speed Management Guide: [https://safety.fhwa.dot.gov/speedmgt/](https://safety.fhwa.dot.gov/speedmgt/)
