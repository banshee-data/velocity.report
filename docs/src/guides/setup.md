---
layout: doc.njk
title: Setup your Citizen Radar
description: Step-by-step guide to assembling and deploying a Citizen Radar for traffic monitoring
section: guides
date: 2025-11-05
---

# **Build Your Own Privacy-First Speed Radar with Open-Source Tools**

### A DIY traffic logger that keeps data local, skips the camera, and helps your neighborhood get safer streets.

---

## **Introduction**

Ever wondered how fast cars are really going past your house or down your kid’s school street? Maybe you've felt like drivers treat your neighborhood like a racetrack—but without hard data, it's tough to get city officials to act.

That’s where this DIY project comes in.

Using an off-the-shelf radar module and open-source software, you can build your own privacy-first traffic logger. It collects vehicle speeds, logs them locally, and generates a visual dashboard and PDF report—without using cameras or sending anything to the cloud.

Unlike many surveillance-heavy "smart city" solutions, this project avoids recording faces, license plates, or any personal data. It’s about speed—not identity.

Whether you’re a concerned parent, a local activist, or just a tinkerer with a Raspberry Pi, this is a fun and meaningful build with real-world impact.

---

## **What You’ll Build**

- A small box (powered by a Raspberry Pi or similar SBC) that uses a Doppler radar sensor to detect vehicle speeds.
- Logs are saved locally and can be backed up via USB or secure LAN/cloud.
- A web dashboard lets you browse live data.
- After a few days, generate a printable PDF report of speed trends and violations.
- Entirely offline capable. No Wi-Fi? No problem.
- Fully open-source.

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

Secure the radar sensor inside your enclosure or mount it directly on a bracket. Make sure it’s:

- Facing directly at oncoming or passing traffic
- Positioned at a fixed angle (ideally 20°–45° off-axis)
- Clear of obstructions (no walls, poles, or trees blocking view)

Tip: Mounting higher helps reduce false positives from small objects.

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

Most OPS243 sensors come configured for CSV output. Our software works best with JSON output. Here’s how to check and (optionally) update:

1. **Connect via terminal**

   ```bash
   stty -F /dev/ttyUSB0 115200
   screen /dev/ttyUSB0 115200
   ```

2. **Enter two-character command** quickly (e.g. `VO` to switch to JSON output).
   Sensors expect the full command without delay.

3. **Verify response**
   Use `FV` to check firmware version, `VO` to show current output mode.

4. **Manufacturer Docs**
   For latest firmware, visit: [https://www.omnipresense.com/support](https://www.omnipresense.com/support)

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

Head to the open-source repository:

```bash
git clone https://github.com/banshee-labs/velocity.report
cd velocity.report
make radar-local
```

This builds `app-radar-local` with pcap support. Copy the binary or install into `/usr/local/bin` (or another system path).

Optional: set up a systemd service to run on boot.

---

### **Step 6: Start Logging & Access the Web Dashboard**

1. Start the logger:

```bash
./app-radar-local --input /dev/ttyUSB0
```

The database defaults to `./sensor_data.db`. You can specify a different location with `--db-path`.

2. Open your browser and visit:

```text
http://raspberrypi.local:8000
```

(Or replace with the actual IP address.)

You’ll see:

- Recent vehicle transits
- Speed heatmap
- Histograms and time-of-day graphs

---

### **Step 7: Generate PDF Reports**

After a few days of data collection:

1. Navigate to the **Settings** tab in the web dashboard.
2. Enable **PDF Report Generation**.
3. Click **Generate Report**.

Your report will include:

- Summary statistics (P50, P85, P98, etc.)
- Histograms of vehicle speeds
- Charts of speeds by hour of day
- Print-ready PDF to share with city officials or neighbors

Sample output:

```text
Report: Clarendon Ave (2025-10-05 to 2025-10-08)
- 93 vehicles observed
- 85th percentile: 36.8 mph
- 8% exceeding 40mph
```

---

## **Wrap-Up & Next Steps**

Congratulations—you’ve now built a functional, privacy-respecting traffic logger! You can keep it running longer to collect more data, or relocate it to new trouble spots.

Want to go further?

- Add GPS timestamping
- Aggregate multiple sensors
- Upload anonymized data to a community dashboard
- Add a battery/solar module for true remote deployment

You now have data. Use it.

Print the report, show your neighbors, bring it to a public meeting, and make your voice heard—backed by real evidence.

Let’s build safer streets, together.

---

## **Resources & Links**

- OmniPreSense OPS243 Support: [https://www.omnipresense.com/support](https://www.omnipresense.com/support)
- GitHub Repo: [https://github.com/banshee-labs/velocity.report](https://github.com/banshee-labs/velocity.report)
- Community: [https://discord.gg/yourserver]
- Sample PDF: [https://velocity.report/sample.pdf]
