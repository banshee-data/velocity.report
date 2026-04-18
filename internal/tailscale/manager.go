// Package tailscale provides an in-process manager that drives the
// Tailscale daemon (tailscaled) on the device.  The daemon ships
// installed-but-masked in the velocity.report image; this package is
// responsible for unmasking and starting it on user opt-in, presenting
// the interactive login URL to the web UI, and applying the standard
// device policy (Tailscale SSH on, tailscale serve on for the local
// HTTP server) once the node is up on the tailnet.
//
// All daemon RPC goes through a LocalClient interface that wraps
// tailscale.com/client/local against the tailscaled Unix socket
// (/var/run/tailscale/tailscaled.sock by default).  The interface exists
// so tests can substitute a fake without standing up a real daemon, and
// so the socket path can be overridden for non-default deployments.
// Access from the non-root velocity service user is granted via
// `tailscale set --operator=velocity`, which velocity-ctl invokes
// immediately after enabling tailscaled.
//
// systemctl operations (unmask/enable/start/stop/mask) are not reachable
// over the local API, so we shell out via sudo to a narrow allowlist
// configured in /etc/sudoers.d/020_velocity-nopasswd.  The set of
// permitted subcommands is a closed enum (sudoAction) so the argv
// surface cannot widen by accident.
package tailscale

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os/exec"
	"sync"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

// LocalServeHTTPTarget is what `tailscale serve` proxies to.  The Go
// HTTP server binds here; tailscale serve terminates TLS at the tailnet
// edge and forwards plaintext to this address.
const LocalServeHTTPTarget = "http://127.0.0.1:8080"

// loginURLMaxAge bounds how long a cached BrowseToURL is considered
// fresh.  Tailscale-issued auth URLs expire on the coordination server
// side after a short window; surfacing a stale one to the UI just
// produces a confusing 4xx page when the user clicks it.
const loginURLMaxAge = 5 * time.Minute

// LocalClient is the slice of tailscale.com/client/local.Client that
// this package depends on.  Pulled into an interface so tests can
// substitute a fake; keep the surface narrow and add methods only as
// needed.
type LocalClient interface {
	Status(ctx context.Context) (*ipnstate.Status, error)
	StatusWithoutPeers(ctx context.Context) (*ipnstate.Status, error)
	GetPrefs(ctx context.Context) (*ipn.Prefs, error)
	EditPrefs(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error)
	GetServeConfig(ctx context.Context) (*ipn.ServeConfig, error)
	SetServeConfig(ctx context.Context, cfg *ipn.ServeConfig) error
	StartLoginInteractive(ctx context.Context) error
	WatchIPNBus(ctx context.Context, mask ipn.NotifyWatchOpt) (BusWatcher, error)
}

// BusWatcher is the slice of *local.IPNBusWatcher we depend on.
type BusWatcher interface {
	Next() (ipn.Notify, error)
	Close() error
}

// realLocalClient adapts *local.Client to the LocalClient interface.
// The real *local.IPNBusWatcher already satisfies BusWatcher.
type realLocalClient struct {
	c *local.Client
}

func (r *realLocalClient) Status(ctx context.Context) (*ipnstate.Status, error) {
	return r.c.Status(ctx)
}
func (r *realLocalClient) StatusWithoutPeers(ctx context.Context) (*ipnstate.Status, error) {
	return r.c.StatusWithoutPeers(ctx)
}
func (r *realLocalClient) GetPrefs(ctx context.Context) (*ipn.Prefs, error) {
	return r.c.GetPrefs(ctx)
}
func (r *realLocalClient) EditPrefs(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error) {
	return r.c.EditPrefs(ctx, mp)
}
func (r *realLocalClient) GetServeConfig(ctx context.Context) (*ipn.ServeConfig, error) {
	return r.c.GetServeConfig(ctx)
}
func (r *realLocalClient) SetServeConfig(ctx context.Context, cfg *ipn.ServeConfig) error {
	return r.c.SetServeConfig(ctx, cfg)
}
func (r *realLocalClient) StartLoginInteractive(ctx context.Context) error {
	return r.c.StartLoginInteractive(ctx)
}
func (r *realLocalClient) WatchIPNBus(ctx context.Context, mask ipn.NotifyWatchOpt) (BusWatcher, error) {
	return r.c.WatchIPNBus(ctx, mask)
}

// SystemdActor performs the privileged systemd lifecycle operations
// (unmask/enable/start/stop/mask) that the velocity service user can't
// invoke directly.  The default implementation shells out to
// `sudo velocity-ctl tailscale ...`.
type SystemdActor interface {
	EnableTailscaled(ctx context.Context) error
	DisableTailscaled(ctx context.Context) error
}

type sudoSystemdActor struct{}

func (sudoSystemdActor) EnableTailscaled(ctx context.Context) error {
	return runSudoCtl(ctx, sudoActionEnable)
}

func (sudoSystemdActor) DisableTailscaled(ctx context.Context) error {
	return runSudoCtl(ctx, sudoActionDisable)
}

// Manager owns the LocalClient and a single long-lived IPN bus
// subscriber.  Construct with New and call Start once before serving
// HTTP.
type Manager struct {
	lc      LocalClient
	systemd SystemdActor

	mu          sync.RWMutex
	browseURL   string    // most recent BrowseToURL from the IPN bus, if any
	urlSetAt    time.Time // when browseURL was set; used for staleness eviction
	loginInProg bool      // true after StartLoginInteractive until the node reaches Running
	policyErr   string    // most recent applyDevicePolicy error message, cleared on success
	enableEpoch uint64    // bumped by Enable; applyDevicePolicy only runs once per epoch

	// runCtx scopes the background bus-watcher goroutine.
	runCtx    context.Context
	runCancel context.CancelFunc
	wg        sync.WaitGroup
	started   bool

	// rand source for backoff jitter (seeded per-Manager so multiple
	// devices don't synchronise their reconnects).
	rng *rand.Rand
}

// Option configures a Manager at construction time.
type Option func(*Manager)

// WithLocalClient injects a custom LocalClient (e.g. one bound to a
// non-default socket path, or a fake in tests).
func WithLocalClient(lc LocalClient) Option {
	return func(m *Manager) { m.lc = lc }
}

// WithSystemdActor injects a custom SystemdActor (used in tests so we
// don't actually shell out to sudo).
func WithSystemdActor(a SystemdActor) Option {
	return func(m *Manager) { m.systemd = a }
}

// New constructs a Manager that talks to the default tailscaled socket
// and shells out to sudo for systemd operations.  Options override the
// defaults.
func New(opts ...Option) *Manager {
	m := &Manager{
		lc:      &realLocalClient{c: &local.Client{}},
		systemd: sudoSystemdActor{},
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Start launches a background goroutine that subscribes to the IPN bus
// and caches BrowseToURL events so the UI can fetch the most recent
// login URL without racing the daemon.  Safe to call when tailscaled is
// not running — the watcher reconnects on its own.  Calling Start more
// than once on the same Manager is a no-op (the first call wins).
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.runCtx, m.runCancel = context.WithCancel(ctx)
	m.wg.Add(1)
	m.mu.Unlock()
	go m.watchLoop()
}

// Stop ends the bus-watcher goroutine.  Safe to call multiple times,
// including before Start.
func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.runCancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
}

// watchLoop maintains a long-lived IPN bus subscription so we can react
// to login URLs and state changes without polling.  When the daemon is
// not running we back off (with jitter) and retry.
func (m *Manager) watchLoop() {
	defer m.wg.Done()
	defer func() {
		// A panic here would otherwise leave the manager dead with no
		// log trace, which the user only notices when the UI stops
		// updating.  Log loudly and let the goroutine exit; the next
		// process restart will recover.
		if r := recover(); r != nil {
			log.Printf("tailscale: watchLoop panic: %v", r)
		}
	}()

	const baseBackoff = 1 * time.Second
	const maxBackoff = 30 * time.Second
	backoff := baseBackoff

	for {
		if m.runCtx.Err() != nil {
			return
		}

		watcher, err := m.lc.WatchIPNBus(m.runCtx, ipn.NotifyInitialState|ipn.NotifyInitialPrefs|ipn.NotifyInitialNetMap)
		if err != nil {
			// Daemon almost certainly not running yet.  Back off (with
			// jitter so a fleet of devices doesn't synchronise) and retry.
			select {
			case <-m.runCtx.Done():
				return
			case <-time.After(backoffWithJitter(backoff, m.rng)):
			}
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}

		// Reset backoff only after we have observed at least one
		// notification — otherwise a daemon that opens the watch and
		// dies immediately resets us to baseBackoff and we hammer.
		madeProgress := m.consumeBus(watcher)
		_ = watcher.Close()
		if madeProgress {
			backoff = baseBackoff
		}
	}
}

// backoffWithJitter returns d ± up-to-25% so multiple Managers don't
// synchronise their reconnect attempts.
func backoffWithJitter(d time.Duration, rng *rand.Rand) time.Duration {
	if d <= 0 {
		return d
	}
	jitter := time.Duration(rng.Int63n(int64(d / 2)))
	return d - d/4 + jitter
}

// consumeBus reads notifications until the watcher errors out (typically
// because tailscaled stopped or restarted).  Returns true if at least
// one notification was successfully read, so the caller knows whether
// to reset the backoff.
func (m *Manager) consumeBus(w BusWatcher) bool {
	progressed := false
	for {
		n, err := w.Next()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("tailscale: IPN bus watcher ended: %v", err)
			}
			return progressed
		}
		progressed = true

		if n.BrowseToURL != nil {
			m.mu.Lock()
			m.browseURL = *n.BrowseToURL
			m.urlSetAt = time.Now()
			m.mu.Unlock()
		}

		if n.State != nil {
			st := *n.State
			m.mu.Lock()
			runEpoch := uint64(0)
			runPolicy := false
			if st == ipn.Running {
				// Successful login — clear the cached URL and the
				// in-progress flag so the UI no longer offers it.
				m.browseURL = ""
				m.urlSetAt = time.Time{}
				m.loginInProg = false
				// Run device policy at most once per Enable: the very
				// first Running transition after enableEpoch was bumped.
				if m.enableEpoch > 0 {
					runEpoch = m.enableEpoch
					m.enableEpoch = 0
					runPolicy = true
				}
			}
			if st == ipn.NoState || st == ipn.Stopped {
				m.loginInProg = false
			}
			m.mu.Unlock()

			if runPolicy {
				go m.applyDevicePolicy(runEpoch)
			}
		}
	}
}

// applyDevicePolicy enforces velocity.report defaults: Tailscale SSH on,
// and `tailscale serve` publishing the local HTTP server on :443 of the
// tailnet name.  Each helper is individually idempotent, but we still
// want this to fire only once per Enable so a network blip can't clobber
// an operator's manual `tailscale serve` edits.
//
// The MagicDNS name is not always populated on the very first Running
// notification, so enableServe retries briefly while DNSName is empty.
func (m *Manager) applyDevicePolicy(epoch uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var firstErr error
	if err := m.enableSSH(ctx); err != nil {
		log.Printf("tailscale: failed to enable SSH (epoch=%d): %v", epoch, err)
		firstErr = err
	}
	if err := m.enableServeWithRetry(ctx); err != nil {
		log.Printf("tailscale: failed to publish via tailscale serve (epoch=%d): %v", epoch, err)
		if firstErr == nil {
			firstErr = err
		}
	}

	m.mu.Lock()
	if firstErr != nil {
		m.policyErr = firstErr.Error()
	} else {
		m.policyErr = ""
	}
	m.mu.Unlock()
}

func (m *Manager) enableSSH(ctx context.Context) error {
	prefs, err := m.lc.GetPrefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}
	if prefs.RunSSH {
		return nil
	}
	mp := &ipn.MaskedPrefs{
		Prefs:     ipn.Prefs{RunSSH: true},
		RunSSHSet: true,
	}
	if _, err := m.lc.EditPrefs(ctx, mp); err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}
	return nil
}

// enableServeWithRetry calls enableServe, retrying on the specific
// "MagicDNS not ready" error that's common on the first Running event.
func (m *Manager) enableServeWithRetry(ctx context.Context) error {
	const attempts = 6
	const interval = 1 * time.Second
	var err error
	for range attempts {
		err = m.enableServe(ctx)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errMagicDNSNotReady) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return err
}

var errMagicDNSNotReady = errors.New("tailscale: MagicDNS name not yet available")

func (m *Manager) enableServe(ctx context.Context) error {
	cfg, err := m.lc.GetServeConfig(ctx)
	if err != nil {
		return fmt.Errorf("get serve config: %w", err)
	}
	if cfg == nil {
		cfg = &ipn.ServeConfig{}
	}

	// Resolve the node's MagicDNS FQDN.  SetWebHandler keys the serve
	// map by HostPort; tailscaled matches inbound TLS connections
	// against this map using the SNI, which carries the *full*
	// MagicDNS name (e.g. velocity-vm.tailfoo.ts.net).  Passing the
	// short name here makes the lookup miss with "no webserver
	// configured for name/port" and TLS handshakes fail.
	st, err := m.lc.StatusWithoutPeers(ctx)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}
	host := nodeFQDN(st)
	if host == "" {
		return errMagicDNSNotReady
	}

	cfg.SetWebHandler(
		&ipn.HTTPHandler{Proxy: LocalServeHTTPTarget},
		host,
		443,
		"/",
		true, // useTLS
		"",   // mds (service name) — not a Tailscale Service
	)

	if err := m.lc.SetServeConfig(ctx, cfg); err != nil {
		return fmt.Errorf("set serve config: %w", err)
	}
	return nil
}

// stripTrailingDot returns dns without its trailing dot, if any.
func stripTrailingDot(dns string) string {
	if dns == "" {
		return ""
	}
	if dns[len(dns)-1] == '.' {
		return dns[:len(dns)-1]
	}
	return dns
}

// nodeFQDN returns the device's MagicDNS name without the trailing
// dot (e.g. "velocity-vm.tailfoo.ts.net"), or "" if the status carries
// no DNSName.
func nodeFQDN(st *ipnstate.Status) string {
	if st == nil || st.Self == nil {
		return ""
	}
	return stripTrailingDot(st.Self.DNSName)
}

// nodeShortName returns the bare node name (without the tailnet
// suffix), e.g. "velocity" from "velocity.tailfoo.ts.net".
func nodeShortName(st *ipnstate.Status) string {
	dns := nodeFQDN(st)
	if dns == "" {
		return ""
	}
	for i, r := range dns {
		if r == '.' {
			return dns[:i]
		}
	}
	return dns
}

// ----- Public API used by the HTTP layer ----------------------------------

// Status is the snapshot returned to the UI.
type Status struct {
	// DaemonRunning is true when the manager can reach tailscaled.
	DaemonRunning bool `json:"daemon_running"`
	// BackendState is the IPN backend state, e.g. "NoState",
	// "NeedsLogin", "Starting", "Running".  Empty when the daemon
	// is unreachable.
	BackendState string `json:"backend_state"`
	// LoginURL is the most recent interactive login URL, if any.
	// Cleared once the node reaches Running or when the URL is
	// older than loginURLMaxAge.
	LoginURL string `json:"login_url,omitempty"`
	// LoginInProgress is true between Enable() and the node
	// reaching Running.  The UI uses this to keep the QR code
	// visible across the bus reconnects that happen during login.
	LoginInProgress bool `json:"login_in_progress"`
	// Hostname is the device's tailnet short name (e.g. "velocity").
	Hostname string `json:"hostname,omitempty"`
	// MagicDNS is the fully-qualified MagicDNS name (e.g.
	// "velocity.tailfoo.ts.net") when up, otherwise empty.
	MagicDNS string `json:"magic_dns,omitempty"`
	// TailnetName is the human-readable tailnet name when known.
	TailnetName string `json:"tailnet_name,omitempty"`
	// PeerCount is the number of peers visible from this node.
	PeerCount int `json:"peer_count"`
	// PolicyError is the most recent error from applyDevicePolicy
	// (SSH or serve setup), if any.  Empty when the last apply
	// succeeded or no apply has run.
	PolicyError string `json:"policy_error,omitempty"`
}

// Status returns the current state of the Tailscale integration.  Never
// returns an error — when the daemon is unreachable the returned Status
// has DaemonRunning=false.
func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	// Evict stale URL up front so callers always see a consistent view.
	if m.browseURL != "" && !m.urlSetAt.IsZero() && time.Since(m.urlSetAt) > loginURLMaxAge {
		m.browseURL = ""
		m.urlSetAt = time.Time{}
	}
	url := m.browseURL
	inProg := m.loginInProg
	policyErr := m.policyErr
	m.mu.Unlock()

	out := Status{
		LoginURL:        url,
		LoginInProgress: inProg,
		PolicyError:     policyErr,
	}

	st, err := m.lc.Status(ctx)
	if err != nil {
		// Daemon not running.  Don't surface a stale LoginURL the
		// bus-watcher captured before disable — the UI uses
		// daemon_running as the toggle's source of truth, but other
		// derived state (login_in_progress, login_url) needs to be
		// consistent or the Tailscale card flickers stale enrolment
		// hints after a disable.
		out.LoginURL = ""
		out.LoginInProgress = false
		out.PolicyError = ""
		return out
	}
	out.DaemonRunning = true
	out.BackendState = st.BackendState
	if st.Self != nil {
		out.Hostname = nodeShortName(st)
		out.MagicDNS = nodeFQDN(st)
	}
	if st.CurrentTailnet != nil {
		out.TailnetName = st.CurrentTailnet.Name
	}
	out.PeerCount = len(st.Peer)

	// If we're already Running, suppress any stale login URL.
	if st.BackendState == ipn.Running.String() {
		out.LoginURL = ""
		out.LoginInProgress = false
	}
	return out
}

// Enable unmasks and starts tailscaled.  If the daemon already has a
// stored identity (i.e. the node has been enrolled before and the user
// is just toggling Tailscale back on), this is a no-op past starting
// the daemon — tailscaled restores its prefs from disk and rejoins the
// tailnet on its own.  Only when the daemon has no identity (fresh
// install or after an explicit Logout) do we kick off interactive
// login, in which case Enable waits up to enableLoginURLWait for a
// BrowseToURL to land on the bus before returning so the API response
// already carries the URL when one is needed.
func (m *Manager) Enable(ctx context.Context) error {
	if err := m.systemd.EnableTailscaled(ctx); err != nil {
		return fmt.Errorf("enable tailscaled: %w", err)
	}
	if err := m.waitForDaemon(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("wait for tailscaled: %w", err)
	}

	// Probe state — if tailscaled has stored credentials it will
	// transition NoState → Starting → Running on its own and we don't
	// need (or want) to ask for a fresh login URL.
	st, err := m.lc.Status(ctx)
	if err != nil {
		return fmt.Errorf("query daemon status: %w", err)
	}
	needsLogin := st.BackendState == ipn.NeedsLogin.String() ||
		st.BackendState == ipn.NoState.String()

	// Bump enableEpoch so the next Running transition triggers
	// applyDevicePolicy exactly once.
	m.mu.Lock()
	m.enableEpoch++
	m.mu.Unlock()

	// Make sure WantRunning=true so the daemon actually brings the
	// interface up; this is the field a previous Disable cleared.  If
	// this fails the node will sit in Stopped forever — fail loudly
	// rather than letting the UI show "enabled" over a dead tunnel.
	if _, err := m.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:          ipn.Prefs{WantRunning: true},
		WantRunningSet: true,
	}); err != nil {
		return fmt.Errorf("set WantRunning=true: %w", err)
	}

	if !needsLogin {
		// Authenticated already; daemon will reach Running on its
		// own.  Nothing more to do.
		return nil
	}

	m.mu.Lock()
	m.loginInProg = true
	m.browseURL = ""
	m.urlSetAt = time.Time{}
	m.mu.Unlock()

	if err := m.lc.StartLoginInteractive(ctx); err != nil {
		m.mu.Lock()
		m.loginInProg = false
		m.mu.Unlock()
		return fmt.Errorf("start login: %w", err)
	}

	// Give the bus watcher a brief window to surface the BrowseToURL
	// before we return.  This avoids the API response saying
	// "login_in_progress=true, login_url=" which forces the UI to do a
	// second fetch before it can render the QR code.
	m.waitForLoginURL(ctx, 5*time.Second)
	return nil
}

func (m *Manager) waitForLoginURL(ctx context.Context, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		m.mu.RLock()
		have := m.browseURL != ""
		m.mu.RUnlock()
		if have {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Disable brings the node down and stops + masks the daemon.  We do NOT
// call Logout: tailscaled keeps its node key on disk
// (/var/lib/tailscale/tailscaled.state) so a subsequent Enable resumes
// the same tailnet identity without re-auth.  Toggling off is "stop
// tunnelling", not "forget my account".
//
// To fully remove the device from the tailnet the operator should
// remove it via the Tailscale admin console; we expose that as a
// distinct action rather than overloading the disable toggle.
func (m *Manager) Disable(ctx context.Context) error {
	// Ask tailscaled to bring the interface down before we stop the
	// daemon.  This makes "Disable" feel instant from peers' point of
	// view: WantRunning=false propagates to the coordination server
	// before the systemd stop kills the connection.  We log but do not
	// abort on failure — mask-and-stop below is the authoritative
	// disable; this is just an optimisation.
	if _, err := m.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:          ipn.Prefs{WantRunning: false},
		WantRunningSet: true,
	}); err != nil {
		log.Printf("tailscale: EditPrefs(WantRunning=false) returned %v (continuing)", err)
	}

	m.mu.Lock()
	m.browseURL = ""
	m.urlSetAt = time.Time{}
	m.loginInProg = false
	m.policyErr = ""
	m.enableEpoch = 0
	m.mu.Unlock()

	if err := m.systemd.DisableTailscaled(ctx); err != nil {
		return fmt.Errorf("disable tailscaled: %w", err)
	}
	return nil
}

// waitForDaemon polls the local API until it responds or the deadline
// elapses.  Used after Enable() so callers see a live socket before we
// attempt StartLoginInteractive.
func (m *Manager) waitForDaemon(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		probeCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		_, err := m.lc.Status(probeCtx)
		cancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tailscaled did not become ready: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// sudoAction is a closed enum of the systemd lifecycle subcommands we
// expose via velocity-ctl.  Keeping this typed (rather than letting
// callers pass arbitrary strings) means no future refactor can widen
// the sudo argv surface.
type sudoAction int

const (
	sudoActionEnable sudoAction = iota
	sudoActionDisable
)

func (a sudoAction) String() string {
	switch a {
	case sudoActionEnable:
		return "enable-tailscaled"
	case sudoActionDisable:
		return "disable-tailscaled"
	default:
		return "unknown"
	}
}

// runSudoCtl invokes velocity-ctl via sudo for tailscale lifecycle
// operations.  velocity-ctl is in the sudoers allowlist; the actual
// systemctl calls happen there.
func runSudoCtl(ctx context.Context, action sudoAction) error {
	subcmd := action.String()
	if subcmd == "unknown" {
		return fmt.Errorf("tailscale: refusing to invoke sudo with unknown action %d", int(action))
	}
	cmd := exec.CommandContext(ctx, "sudo", "-n", "/usr/local/bin/velocity-ctl", "tailscale", subcmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", subcmd, err, string(out))
	}
	return nil
}
