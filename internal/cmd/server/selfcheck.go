package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"runtime"
	"time"

	"github.com/banshee-data/velocity.report/internal/version"
)

// runSelfCheck exercises the runtime paths that historically break in
// static / musl / non-glibc builds: DNS resolution (cgo getaddrinfo),
// UDP socket creation, and libpcap.a linkage. Each check prints PASS or
// FAIL with a one-line diagnostic; the overall exit code is non-zero if
// any critical check failed.
//
// "Critical" = anything that would make the binary non-functional on a
// real target. Best-effort checks that depend on network connectivity
// (external DNS) report failures as warnings, not errors, so the check
// is usable in offline CI runners.
func runSelfCheck(out io.Writer) int {
	r := &selfCheckReport{out: out}

	fmt.Fprintf(out, "velocity-report self-check\n")
	fmt.Fprintf(out, "  version: %s (%s)\n", version.Version, version.GitSHA)
	fmt.Fprintf(out, "  go:      %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	r.run("dns-localhost", true, checkDNSLocalhost)
	r.run("dns-external", false, checkDNSExternal) // best-effort; offline CI is OK
	r.run("udp-bind", true, checkUDPBind)
	r.run("tcp-bind", true, checkTCPBind)
	selfCheckLibpcap(r)

	fmt.Fprintf(out, "\nresult: %d ok, %d failed (%d warnings)\n", r.passed, r.failed, r.warned)
	if r.failed > 0 {
		return 1
	}
	return 0
}

type selfCheckReport struct {
	out                    io.Writer
	passed, failed, warned int
}

// run executes one check. If critical is true, a failure increments the
// fail counter; if not, it's logged as a warning instead.
func (r *selfCheckReport) run(name string, critical bool, fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := fn(ctx)
	switch {
	case err == nil:
		r.passed++
		fmt.Fprintf(r.out, "  PASS %s\n", name)
	case critical:
		r.failed++
		fmt.Fprintf(r.out, "  FAIL %s: %v\n", name, err)
	default:
		r.warned++
		fmt.Fprintf(r.out, "  WARN %s: %v\n", name, err)
	}
}

// checkDNSLocalhost resolves "localhost" — must work on every system,
// regardless of internet connectivity. Exercises Go's resolver enough
// to catch totally-broken static builds without depending on the
// network.
func checkDNSLocalhost(ctx context.Context) error {
	addrs, err := net.DefaultResolver.LookupHost(ctx, "localhost")
	if err != nil {
		return fmt.Errorf("LookupHost(localhost): %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("LookupHost(localhost) returned no addresses")
	}
	return nil
}

// checkDNSExternal resolves a known public name to exercise the full
// getaddrinfo path (musl's resolver in a static build). Reported as a
// warning rather than a failure so offline CI / firewalled runners
// don't trip on it.
func checkDNSExternal(ctx context.Context) error {
	addrs, err := net.DefaultResolver.LookupHost(ctx, "one.one.one.one")
	if err != nil {
		return fmt.Errorf("LookupHost(one.one.one.one): %w (offline?)", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("LookupHost(one.one.one.one) returned no addresses")
	}
	return nil
}

// checkUDPBind opens and closes a UDP socket on an ephemeral port.
// Catches missing socket() / bind() syscalls in a broken static build.
func checkUDPBind(_ context.Context) error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	if err != nil {
		return fmt.Errorf("ListenUDP: %w", err)
	}
	return conn.Close()
}

// checkTCPBind mirrors UDP for the TCP listen path used by the HTTP API.
func checkTCPBind(_ context.Context) error {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("Listen(tcp): %w", err)
	}
	return l.Close()
}
