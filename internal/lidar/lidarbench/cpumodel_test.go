//go:build pcap

package lidarbench

import (
	"bytes"
	"log"
	"runtime"
	"strings"
	"testing"
)

// x86CPUInfo is two cores of a hosted runner's /proc/cpuinfo, trimmed.
const x86CPUInfo = `processor	: 0
vendor_id	: AuthenticAMD
cpu family	: 25
model		: 1
model name	: AMD EPYC 7763 64-Core Processor
cpu MHz		: 3243.468

processor	: 1
vendor_id	: AuthenticAMD
model name	: AMD EPYC 7763 64-Core Processor
`

// piCPUInfo is a Raspberry Pi 5's /proc/cpuinfo, trimmed: no "model name",
// the board on a closing "Model" line, and a serial number that must not be
// taken for it.
const piCPUInfo = `processor	: 0
BogoMIPS	: 108.00
Features	: fp asimd evtstrm aes pmull sha1 sha2 crc32 atomics fphp asimdhp cpuid asimdrdm lrcpc dcpop asimddp
CPU implementer	: 0x41
CPU part	: 0xd0b

Revision	: d04170
Serial		: 0123456789abcdef
Model		: Raspberry Pi 5 Model B Rev 1.0
`

func TestParseCPUInfo(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"x86 model name", x86CPUInfo, "AMD EPYC 7763 64-Core Processor"},
		{"Raspberry Pi board", piCPUInfo, "Raspberry Pi 5 Model B Rev 1.0"},
		{"model name before a board line", "Model\t: board\nmodel name\t: core\n", "core"},
		{"neither", "processor\t: 0\nCPU part\t: 0xd0b\n", ""},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseCPUInfo(strings.NewReader(tc.in)); got != tc.want {
				t.Errorf("parseCPUInfo = %q, want %q", got, tc.want)
			}
		})
	}
}

// The platforms the project measures on name their processor: the Mac by
// sysctl, an x86 runner by /proc/cpuinfo. An arm64 Linux host other than a Pi
// may name neither, which leaves the field empty rather than wrong.
func TestGetSystemInfoNamesTheCPU(t *testing.T) {
	if runtime.GOOS != "darwin" && (runtime.GOOS != "linux" || runtime.GOARCH != "amd64") {
		t.Skipf("no CPU model is promised on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if got := getSystemInfo("").CPUModel; got == "" {
		t.Error("CPUModel is empty")
	}
}

// TestIdentityWarnsOnADifferentCPUModel: hosted runners of one class are
// given different CPU generations, so a different model warns, naming both,
// and does not refuse; a baseline that predates the field says nothing.
func TestIdentityWarnsOnADifferentCPUModel(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	info := SystemInfo{GOOS: "linux", GOARCH: "amd64", HostClass: HostClassCI, NumCPU: 4, GoVersion: "go1.27.1"}
	baseline, current := comparableResult(), comparableResult()
	baseline.SystemInfo, current.SystemInfo = info, info
	baseline.SystemInfo.CPUModel = "AMD EPYC 9V74 80-Core Processor"
	current.SystemInfo.CPUModel = "AMD EPYC 7763 64-Core Processor"

	if err := checkWorkloadIdentity(baseline, current, DefaultWorkTolerance); err != nil {
		t.Fatalf("a different CPU model should warn, not refuse: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "9V74") || !strings.Contains(got, "7763") {
		t.Errorf("the warning should name both CPUs, got %q", got)
	}

	buf.Reset()
	baseline.SystemInfo.CPUModel = ""
	if err := checkWorkloadIdentity(baseline, current, DefaultWorkTolerance); err != nil {
		t.Fatalf("a baseline without a CPU model should compare: %v", err)
	}
	if strings.Contains(buf.String(), "7763") {
		t.Errorf("a baseline without a CPU model should not warn about one, got %q", buf.String())
	}
}
