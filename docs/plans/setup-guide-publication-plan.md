# Setup guide publication plan

- **Status:** Active
- **Canonical:** [setup.md](../../public_html/src/guides/setup.md)

Checklist of remaining items before the setup guide at [public_html/src/guides/setup.md](../../public_html/src/guides/setup.md) is ready for public consumption.

## Content placeholders

The guide currently contains `[PLACEHOLDER]` markers where visual assets are needed. These are not optional: a guide about mounting hardware on poles without photos is a guide about trust, and trust requires evidence.

- [ ] **Hero image**: Completed infrastructure deployment (weatherproof enclosure mounted on utility pole)
- [ ] **Wiring diagram**: RS232 connections between OPS7243 sensor and Waveshare HAT, colour-coded wires, pin labels
- [ ] **Sensor config screenshot**: Terminal showing `OJ`, `UM`, `OM`, `A!` commands and JSON output
- [ ] **Dashboard screenshot**: Web dashboard showing real-time detections, histogram, time-of-day patterns
- [ ] **Mounting angle diagram**: Top-down view of street showing sensor position and 20–45° off-axis angle
- [ ] **Pole mount photo**: Weatherproof enclosure on utility pole, close-up of hose clamp mounting
- [ ] **PDF report sample**: Page from a real report showing histogram, p50/p85/p98, time-of-day chart
- [ ] **Advocacy photo**: Community member presenting PDF report at council meeting (or staged equivalent)

**Image format guidance**: Use `.webp` for photos (smaller, modern browsers all support it). Use `.svg` for diagrams where possible. Place images in `public_html/src/images/guides/setup/`. Alt text on every image; accessibility is not decoration.

## Pi image release

The guide now assumes the Pi image is the primary installation method. These items must be in place:

- [ ] **Image file published**: `.img.xz` uploaded to GitHub Releases with correct SHA-256
- [ ] **os-list-velocity.json updated**: `extract_sha256` field replaced with real checksum (currently `PLACEHOLDER`)
- [ ] **Release URL live**: Download URL in the guide resolves to a real file
- [ ] **Raspberry Pi Imager tested**: Verify the custom repository URL works in stock Imager
- [ ] **First boot tested**: Flash → boot → SSH → `systemctl status velocity-report` shows `active (running)`
- [ ] **velocity-ctl tested**: `sudo velocity-ctl upgrade --check` works on a fresh image

## Readability review

- [ ] **Fresh-eyes read**: Have someone who has never touched the project follow the guide start to finish
- [ ] **Time estimate validated**: Confirm 2–4 hours is realistic (clock someone doing it)
- [ ] **Troubleshooting coverage**: Walk through each failure mode mentioned and verify the fix works
- [ ] **Link check**: Every internal and external link resolves (especially OmniPreSense product links and Discord)
- [ ] **Mobile rendering**: Preview the Eleventy-rendered page on a phone; lots of people read docs on phones while standing next to hardware

## Cross-Reference updates

When the guide changes, other documents may need to match:

- [ ] **README.md**: Update any references to the setup process or installation method
- [ ] **ARCHITECTURE.md**: Verify paths and component descriptions still match
- [ ] **TROUBLESHOOTING.md**: Ensure the troubleshooting guide covers the same failure modes
- [ ] **tools/pdf-generator/README.md**: Verify report generation instructions are consistent
- [ ] **image/README.md**: Ensure the image README and the setup guide do not contradict each other

## Publicising

Once content and assets are complete:

- [ ] **Eleventy build passes**: `cd public_html && npx @11ty/eleventy` produces the page without errors
- [ ] **SEO metadata**: Check front matter `title` and `description` are useful (not keyword-stuffed)
- [ ] **Open Graph tags**: If the Eleventy template supports them, verify the social preview looks sensible
- [ ] **Discord announcement**: Post in the community channel with a direct link
- [ ] **GitHub Release notes**: Reference the guide in the next release changelog
- [ ] **README link**: Ensure the main README links to the published guide URL

## Voice notes

The guide is currently written at mid-Terry (Dial 2): clear, warm, personality present but restrained. If the project wants a more distinctive voice for the public launch, the full-Terry (Dial 3) alternatives below could replace specific sections. These are optional; mid-Terry is the safer choice for a guide people will follow while holding a screwdriver.

### Full-Terry alternatives (optional)

**Introduction: current (mid)**:

> Measuring vehicle speeds is the first step toward safer streets. Without data, the conversation tends to stall at "it feels fast" versus "the speed limit is fine": and feelings, however justified, do not survive contact with a council agenda.

**Introduction: full Terry**:

> Every street has a speed limit. Most streets also have a second, informal speed limit: the one drivers actually observe, which tends to be higher, faster, and considerably more dangerous. The first step toward fixing this is proving it exists, which requires something more persuasive than a firmly worded letter. This guide shows you how to build that something.

---

**Before You Begin: current (mid)**:

> Patience for sensor configuration (it can be finicky, which is a polite word for it)

**Before You Begin: full Terry**:

> Patience for sensor configuration (the sensor knows exactly what it wants; it just communicates this through the medium of silence and garbled output)

---

**Step 1: Flash the Pi Image; current (mid)**:

> The image also pre-configures serial port settings, UART overlays, and the service user. These are the things you would normally spend thirty minutes getting wrong, so they arrive done.

**Step 1: Flash the Pi Image; full Terry**:

> The image also pre-configures serial port settings, UART overlays, and the service user: the sort of items that, without pre-configuration, transform a pleasant afternoon project into a quiet argument with `/boot/config.txt`. They arrive done. You are welcome.

---

**Public Internet Deployment: current (mid)**:

> The dashboard has no authentication, no HTTPS, and no rate limiting. It was not designed for that, and it will not thank you for the experience.

**Public Internet Deployment: full Terry**:

> The dashboard has no authentication, no HTTPS, and no rate limiting. Putting it on the public internet would be like leaving your front door open and then being surprised when a stranger walks in and rearranges the furniture.

---

**Wrap-Up: current (mid)**:

> The data does not care who collected it: it just needs to be accurate, and now it is.

**Wrap-Up: full Terry**:

> The data does not care who collected it, which is one of the better things about data. It just needs to be accurate, and now: assuming you have followed the steps and the sensor is not pointing at a hedge; it is.

---

Pick the ones that earn their keep. Leave the rest at mid.
