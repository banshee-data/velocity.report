package main

// Define allow list of two character commands
var allowedCommands = []string{
	"??", // Query overall module information
	"?R", // Read Reset Reason
	"?Z", // Read Speed Resolution
	"?z", // Read Range Resolution
	"?P", // Read Sensor Part Number
	"?N", // Read Serial Number
	"?D", // Read Build Date
	"L?", // Read Sensor Label
	"?V", // Read Firmware Version
	"?B", // Read Firmware Build Number

	// Speed and Range Units
	"U?", // Query current speed (velocity) units
	"UC", // Set units to centimeters per second
	"UF", // Set units to feet per second
	"UK", // Set units to kilometers per hour
	"UM", // Set units to meters per second
	"US", // Set units to miles per hour
	"u?", // Query current range units
	"uM", // Set range units to meters
	"uC", // Set range units to centimeters
	"uF", // Set range units to feet
	"uI", // Set range units to inches
	"uY", // Set range units to yards

	// Data Precision
	"F?", // Query the current decimal precision setting

	// Sampling Rate and Buffer Size
	"SI", // Set sampling rate to 1K samples/second
	"SV", // Set sampling rate to 5K samples/second
	"SX", // Set sampling rate to 10K samples/second (also "S1")
	"S2", // Set sampling rate to 20K samples/second
	"SL", // Set sampling rate to 50K samples/second
	"SC", // Set sampling rate to 100K samples/second
	"S>", // Set buffer size to 1024 samples
	"S<", // Set buffer size to 512 samples
	"S[", // Set buffer size to 256 samples
	"S(", // Set buffer size to 128 samples

	// Speed/Range Resolution Control
	"X1", // Resolution control: X1 (default)
	"X2", // Resolution control: X2
	"X4", // Resolution control: X4
	"X8", // Resolution control: X8

	// Filtering & Direction
	"R?", // Query current speed filter settings
	"r?", // Query current range filter settings
	"R+", // Set to report inbound direction only
	"R-", // Set to report outbound direction only
	"R|", // Clear any directional filtering

	// Peak Speed Averaging
	"K+", // Enable peak speed averaging
	"K-", // Disable peak speed averaging

	// Frequency (UART) Commands
	"?F", // Query current frequency output
	"T?", // Query current transmitter frequency

	// Data Output Settings
	"O?", // Query output settings
	"OS", // Enable speed reporting
	"Os", // Disable speed reporting
	"OD", // Enable range reporting
	"Od", // Disable range reporting
	"OB", // Enable binary (hex) output
	"Ob", // Disable binary (hex) output
	"OF", // Enable FFT output (Doppler mode)
	"of", // Enable FFT output (FMCW mode)
	"OL", // Turn LED control on
	"Ol", // Turn LED control off
	"OM", // Enable magnitude reporting (Doppler)
	"Om", // Disable magnitude reporting (Doppler)
	"oM", // Enable magnitude reporting (FMCW)
	"om", // Disable magnitude reporting (FMCW)
	"OP", // Enable phase data output (speed)
	"Op", // Disable phase data output (speed)
	"oP", // Enable phase data output (range)
	"op", // Disable phase data output (range)
	"OR", // Enable raw ADC output (Doppler)
	"oR", // Enable raw ADC output (FMCW)
	"OT", // Enable time reporting
	"Ot", // Disable time reporting
	"OU", // Enable units reporting with each data output
	"Ou", // Disable units reporting with each data output
	"OZ", // Activate the USB overflow watchdog
	"Oz", // Revert the USB overflow watchdog to default behavior

	// Blank Data Reporting
	"B?", // Query the current blank data reporting setting
	"BZ", // Report zero value when blanking
	"BL", // Report blank lines
	"BS", // Report a space
	"BC", // Report with a comma
	"BT", // Report with a timestamp
	"BV", // Turn off blank data reporting

	// UART Interface Control
	"I?", // Query current baud rate
	"I1", // Set baud rate to 9,600
	"I2", // Set baud rate to 19,200 (default)
	"I3", // Set baud rate to 57,600
	"I4", // Set baud rate to 115,200
	"I5", // Set baud rate to 230,400
	"IS", // Select RS-232/UART interface output
	"Is", // Switch back to UART output

	// Object Detection Interrupt
	"IG", // Enable object detection interrupt
	"Ig", // Disable object detection interrupt

	// Simple Counter Commands
	"N?", // Query object count
	"N!", // Reset object count
	"N>", // Set count start threshold
	"N<", // Set count end threshold
	"N#", // Query count without reset
	"N@", // Query count settings

	// Clock
	"C?", // Query sensor clock (time since power-on)

	// Power & Transmit Settings
	"PA", // Set active power mode
	"PI", // Set idle power mode
	"PP", // Initiate a single pulse (after setting idle mode)
	"P7", // Set transmit power to -9 dB
	"P6", // Set transmit power to -6 dB
	"P5", // Set transmit power to -4 dB
	"P4", // Set transmit power to -2.5 dB
	"P3", // Set transmit power to mid-level (-1.4 dB)
	"P2", // Set transmit power to -0.8 dB
	"P1", // Set transmit power to -0.4 dB
	"P0", // Set maximum transmit power (alias for PX)
	"PX", // Set maximum transmit power (alias for P0)
	"PW", // Control WiFi power

	// Duty Cycle / Hibernate
	"W?", // Query short delay time (duty cycle)
	"W0", // Set delay to 0 ms
	"WI", // Set delay to 1 ms
	"WV", // Set delay to 5 ms
	"WX", // Set delay to 10 ms
	"WL", // Set delay to 50 ms
	"WC", // Set delay to 100 ms
	"WD", // Set delay to 500 ms
	"WM", // Set delay to 1000 ms
	"Z?", // Query current sleep/hibernate setting
	"Z0", // Set sleep time to 0 seconds (normal operation)
	"ZI", // Set sleep time to 1 second
	"ZV", // Set sleep time to 5 seconds
	"ZX", // Set sleep time to 10 seconds
	"ZL", // Set sleep time to 50 seconds
	"ZC", // Set sleep time to 100 seconds
	"Z2", // Set sleep time to 200 seconds
	"Z+", // Enable hibernate mode (OPS243-C)
	"Z-", // Disable hibernate mode (OPS243-C)

	// Magnitude Control
	"M?", // Query current speed magnitude setting (Doppler)
	// "M>", // Set low speed magnitude filter (Doppler)
	// "M<", // Set high speed magnitude filter (Doppler)
	// "m?", // Query current range magnitude setting (FMCW)
	// "m>", // Set low range magnitude filter (FMCW)
	// "m<", // Set high range magnitude filter (FMCW)

	// Alerts & Averaging
	"Y?", // Query alert and averaging settings (speed alerts for OPS243-A)
	"y?", // Query alert settings for FMCW sensors (range alerts)
	"Y+", // Enable speed averaging (Doppler)
	"Y-", // Disable speed averaging (Doppler)
	"y+", // Enable range averaging (FMCW)
	"y-", // Disable range averaging (FMCW)

	// Persistent Memory
	"A!", // Save current configuration to persistent memory
	"A?", // Query persistent memory settings
	"A.", // Read current settings from persistent memory
	"AX", // Reset flash settings to factory defaults
}
