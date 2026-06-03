// Package radar holds the catalogue of documented OmniPreSense OPS24x serial
// API commands (per AN-010-Z).
//
// The catalogue is advisory, not a security boundary. The OPS24x command set is
// config/query-only and non-destructive — there is no firmware-flash command —
// so the POST /admin/radar/command HTTP handler forwards any command to the
// sensor. Commands not found in the catalogue are still forwarded; the handler
// logs an advisory warning so a typo is visible without blocking legitimate but
// undocumented or newly added commands. Access is restricted by binding to
// localhost (and planned API auth), not by filtering command strings.
//
// KnownCommands is enumerable so a dashboard can present a command dropdown; it
// is exposed over HTTP via GET /api/commands.
package radar

// Command is a single documented OPS24x API command: a two-character code and a
// human-readable description sourced from AN-010-Z.
type Command struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// KnownCommands is the catalogue of documented OPS24x API commands. Every code
// is exactly two characters. It is advisory (see package doc): unknown commands
// are warned about, not rejected.
var KnownCommands = []Command{
	// Module Information
	{"??", "Query overall module information"},
	{"?R", "Read reset reason"},
	{"?Z", "Read speed resolution"},
	{"?z", "Read range resolution"},
	{"?P", "Read sensor part number"},
	{"?N", "Read serial number"},
	{"?D", "Read build date"},
	{"L?", "Read sensor label"},
	{"?V", "Read firmware version"},
	{"?B", "Read firmware build number"},

	// Speed and Range Units
	{"U?", "Query current speed (velocity) units"},
	{"UC", "Set speed units to centimetres per second"},
	{"UF", "Set speed units to feet per second"},
	{"UK", "Set speed units to kilometres per hour"},
	{"UM", "Set speed units to metres per second"},
	{"US", "Set speed units to miles per hour"},
	{"u?", "Query current range units"},
	{"uM", "Set range units to metres"},
	{"uC", "Set range units to centimetres"},
	{"uF", "Set range units to feet"},
	{"uI", "Set range units to inches"},
	{"uY", "Set range units to yards"},

	// Data Precision
	{"F?", "Query the current decimal precision setting"},

	// Sampling Rate and Buffer Size
	{"SI", "Set sampling rate to 1K samples/second"},
	{"SV", "Set sampling rate to 5K samples/second"},
	{"SX", "Set sampling rate to 10K samples/second (also S1)"},
	{"S2", "Set sampling rate to 20K samples/second"},
	{"SL", "Set sampling rate to 50K samples/second"},
	{"SC", "Set sampling rate to 100K samples/second"},
	{"S>", "Set buffer size to 1024 samples"},
	{"S<", "Set buffer size to 512 samples"},
	{"S[", "Set buffer size to 256 samples"},
	{"S(", "Set buffer size to 128 samples"},

	// Speed/Range Resolution Control
	{"X1", "Resolution control: X1 (default)"},
	{"X2", "Resolution control: X2"},
	{"X4", "Resolution control: X4"},
	{"X8", "Resolution control: X8"},

	// Filtering & Direction
	{"R?", "Query current speed filter settings"},
	{"r?", "Query current range filter settings"},
	{"R+", "Report inbound direction only"},
	{"R-", "Report outbound direction only"},
	{"R|", "Clear any directional filtering"},

	// Peak Speed Averaging
	{"K+", "Enable peak speed averaging"},
	{"K-", "Disable peak speed averaging"},

	// Frequency (UART) Commands
	{"?F", "Query current frequency output"},
	{"T?", "Query current transmitter frequency"},

	// Data Output Settings
	{"O?", "Query speed output settings"},
	{"o?", "Query range output settings"},
	{"OS", "Enable speed reporting (Doppler)"},
	{"Os", "Disable speed reporting (Doppler)"},
	{"OD", "Enable range reporting (FMCW)"},
	{"Od", "Disable range reporting (FMCW)"},
	{"oD", "Enable range reporting (FMCW, lowercase form)"},
	{"od", "Disable range reporting (FMCW, lowercase form)"},
	{"OB", "Enable binary (hex) output"},
	{"Ob", "Disable binary (hex) output"},
	{"OF", "Enable FFT output (Doppler mode)"},
	{"of", "Enable FFT output (FMCW mode)"},
	{"OG", "Enable object sensor white light (on IG detection)"},
	{"Og", "Disable object sensor white light"},
	{"OC", "Enable processing-activity lights (Doppler)"},
	{"Oc", "Disable processing-activity lights (Doppler)"},
	{"oC", "Enable processing-activity lights (FMCW)"},
	{"oc", "Disable processing-activity lights (FMCW)"},
	{"OH", "Enable human-readable timestamp output"},
	{"Oh", "Disable human-readable timestamp output"},
	{"OJ", "Enable JSON output mode"},
	{"Oj", "Disable JSON output mode"},
	{"OL", "Turn LED control on"},
	{"Ol", "Turn LED control off"},
	{"OM", "Enable magnitude reporting (Doppler)"},
	{"Om", "Disable magnitude reporting (Doppler)"},
	{"oM", "Enable magnitude reporting (FMCW)"},
	{"om", "Disable magnitude reporting (FMCW)"},
	{"ON", "Enable detected-object (max-speed) reporting mode"},
	{"On", "Disable detected-object reporting mode"},
	{"OP", "Enable phase data output (speed)"},
	{"Op", "Disable phase data output (speed)"},
	{"oP", "Enable phase data output (range)"},
	{"op", "Disable phase data output (range)"},
	{"OR", "Enable raw ADC output (Doppler)"},
	{"oR", "Enable raw ADC output (FMCW)"},
	{"OT", "Enable time reporting"},
	{"Ot", "Disable time reporting"},
	{"OU", "Enable units reporting with each data output (Doppler)"},
	{"Ou", "Disable units reporting with each data output (Doppler)"},
	{"oU", "Enable units reporting with each data output (FMCW)"},
	{"ou", "Disable units reporting with each data output (FMCW)"},
	{"OV", "Order speed reports by largest value first"},
	{"Ov", "Restore speed report ordering by signal magnitude"},
	{"oV", "Order range reports by largest value first"},
	{"ov", "Restore range report ordering by signal magnitude"},
	{"O/", "Order speed reports by smallest value first"},
	{"o/", "Order range reports by smallest value first"},
	{"OY", "Enable combined range-and-speed filter"},
	{"Oy", "Disable combined range-and-speed filter (Doppler)"},
	{"oy", "Disable combined range-and-speed filter (FMCW)"},
	{"OZ", "Activate the USB overflow watchdog"},
	{"Oz", "Revert the USB overflow watchdog to default behaviour"},

	// Blank Data Reporting
	{"B?", "Query the current blank data reporting setting"},
	{"BZ", "Report zero value when blanking"},
	{"BL", "Report blank lines"},
	{"BS", "Report a space"},
	{"BC", "Report with a comma"},
	{"BT", "Report with a timestamp"},
	{"BV", "Turn off blank data reporting"},

	// UART Interface Control
	{"I?", "Query current baud rate"},
	{"I1", "Set baud rate to 9,600"},
	{"I2", "Set baud rate to 19,200 (default)"},
	{"I3", "Set baud rate to 57,600"},
	{"I4", "Set baud rate to 115,200"},
	{"I5", "Set baud rate to 230,400"},
	{"IS", "Select RS-232/UART interface output"},
	{"Is", "Switch back to UART output"},

	// Object Detection Interrupt
	{"IG", "Enable object detection interrupt"},
	{"Ig", "Disable object detection interrupt"},

	// Simple Counter Commands
	{"N?", "Query object count"},
	{"N!", "Reset object count"},
	{"N>", "Set count start threshold"},
	{"N<", "Set count end threshold"},
	{"N#", "Query count without reset"},
	{"N@", "Query count settings"},

	// Clock
	{"C?", "Query sensor clock (time since power-on)"},

	// Power & Transmit Settings
	{"PA", "Set active power mode"},
	{"PI", "Set idle power mode"},
	{"PP", "Initiate a single pulse (after setting idle mode)"},
	{"P7", "Set transmit power to -9 dB"},
	{"P6", "Set transmit power to -6 dB"},
	{"P5", "Set transmit power to -4 dB"},
	{"P4", "Set transmit power to -2.5 dB"},
	{"P3", "Set transmit power to mid-level (-1.4 dB)"},
	{"P2", "Set transmit power to -0.8 dB"},
	{"P1", "Set transmit power to -0.4 dB"},
	{"P0", "Set maximum transmit power (alias for PX)"},
	{"PX", "Set maximum transmit power (alias for P0)"},
	{"PW", "Control WiFi power"},

	// Duty Cycle / Hibernate
	{"W?", "Query short delay time (duty cycle)"},
	{"W0", "Set delay to 0 ms"},
	{"WI", "Set delay to 1 ms"},
	{"WV", "Set delay to 5 ms"},
	{"WX", "Set delay to 10 ms"},
	{"WL", "Set delay to 50 ms"},
	{"WC", "Set delay to 100 ms"},
	{"WD", "Set delay to 500 ms"},
	{"WM", "Set delay to 1000 ms"},
	{"Z?", "Query current sleep/hibernate setting"},
	{"Z0", "Set sleep time to 0 seconds (normal operation)"},
	{"ZI", "Set sleep time to 1 second"},
	{"ZV", "Set sleep time to 5 seconds"},
	{"ZX", "Set sleep time to 10 seconds"},
	{"ZL", "Set sleep time to 50 seconds"},
	{"ZC", "Set sleep time to 100 seconds"},
	{"Z2", "Set sleep time to 200 seconds"},
	{"Z+", "Enable hibernate mode (OPS243-C)"},
	{"Z-", "Disable hibernate mode (OPS243-C)"},

	// Magnitude Control
	{"M?", "Query current speed magnitude setting (Doppler)"},

	// Alerts & Averaging
	{"Y?", "Query alert and averaging settings (speed alerts, OPS243-A)"},
	{"y?", "Query alert settings for FMCW sensors (range alerts)"},
	{"Y+", "Enable speed averaging (Doppler)"},
	{"Y-", "Disable speed averaging (Doppler)"},
	{"y+", "Enable range averaging (FMCW)"},
	{"y-", "Disable range averaging (FMCW)"},

	// Persistent Memory
	{"A!", "Save current configuration to persistent memory"},
	{"A?", "Query persistent memory settings"},
	{"A.", "Read current settings from persistent memory"},
	{"AX", "Reset flash settings to factory defaults"},
}

// IsKnownCommand reports whether cmd is a documented command in KnownCommands.
// Unknown commands are not rejected — callers forward them to the sensor and
// log an advisory warning. See the package doc for why the catalogue is
// advisory rather than an allowlist.
func IsKnownCommand(cmd string) bool {
	for _, c := range KnownCommands {
		if c.Code == cmd {
			return true
		}
	}
	return false
}
