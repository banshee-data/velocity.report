// Package timeutil provides a testable abstraction over time operations.
package timeutil

import (
	"sync"
	"time"
)

// Clock provides an abstraction over time operations for testability.
type Clock interface {
	// Now returns the current time.
	Now() time.Time

	// Since returns the duration since t.
	Since(t time.Time) time.Duration

	// Until returns the duration until t.
	Until(t time.Time) time.Duration

	// Sleep pauses for the specified duration.
	Sleep(d time.Duration)

	// After waits for the duration to elapse and then sends the current time.
	After(d time.Duration) <-chan time.Time

	// NewTimer creates a new Timer that will send the current time
	// on its channel after at least duration d.
	NewTimer(d time.Duration) Timer

	// NewTicker returns a new Ticker containing a channel that will
	// send the time with a period specified by the duration argument.
	NewTicker(d time.Duration) Ticker
}

// Timer represents a single event timer.
type Timer interface {
	// C returns the channel on which the time is delivered.
	C() <-chan time.Time

	// Stop prevents the Timer from firing.
	Stop() bool

	// Reset changes the timer to expire after duration d.
	Reset(d time.Duration) bool
}

// Ticker holds a channel that delivers "ticks" of a clock at intervals.
type Ticker interface {
	// C returns the channel on which the ticks are delivered.
	C() <-chan time.Time

	// Stop turns off a ticker.
	Stop()

	// Reset stops a ticker and resets its period to the specified duration.
	Reset(d time.Duration)
}

// RealClock implements Clock using the standard time package.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time {
	return time.Now()
}

// Since returns the time elapsed since t.
func (RealClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// Until returns the duration until t.
func (RealClock) Until(t time.Time) time.Duration {
	return time.Until(t)
}

// Sleep pauses the current goroutine for at least the duration d.
func (RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

// After waits for the duration to elapse and then sends the current time.
func (RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// NewTimer creates a new Timer.
func (RealClock) NewTimer(d time.Duration) Timer {
	return &realTimer{timer: time.NewTimer(d)}
}

// NewTicker returns a new Ticker.
func (RealClock) NewTicker(d time.Duration) Ticker {
	return &realTicker{ticker: time.NewTicker(d)}
}

type realTimer struct {
	timer *time.Timer
}

func (t *realTimer) C() <-chan time.Time { return t.timer.C }
func (t *realTimer) Stop() bool          { return t.timer.Stop() }
func (t *realTimer) Reset(d time.Duration) bool {
	return t.timer.Reset(d)
}

type realTicker struct {
	ticker *time.Ticker
}

func (t *realTicker) C() <-chan time.Time { return t.ticker.C }
func (t *realTicker) Stop()               { t.ticker.Stop() }
func (t *realTicker) Reset(d time.Duration) {
	t.ticker.Reset(d)
}

// MockClock is a manually controlled clock for testing.
type MockClock struct {
	mu      sync.Mutex
	now     time.Time
	sleeps  []time.Duration
	timers  []*MockTimer
	tickers []*MockTicker
}

// NewMockClock creates a new MockClock set to the given time.
func NewMockClock(t time.Time) *MockClock {
	return &MockClock{now: t}
}

// Now returns the mocked current time.
func (c *MockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Set sets the mock clock to a specific time.
func (c *MockClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// Advance moves the mock clock forward by the given duration
// and fires any expired timers/tickers.
func (c *MockClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	timers := c.timers
	tickers := c.tickers
	c.mu.Unlock()

	// Fire expired timers
	for _, t := range timers {
		t.checkAndFire(now)
	}

	// Fire expired tickers
	for _, t := range tickers {
		t.checkAndFire(now)
	}
}

// Since returns the duration since t.
func (c *MockClock) Since(t time.Time) time.Duration {
	return c.Now().Sub(t)
}

// Until returns the duration until t.
func (c *MockClock) Until(t time.Time) time.Duration {
	return t.Sub(c.Now())
}

// Sleep records the sleep duration but returns immediately.
func (c *MockClock) Sleep(d time.Duration) {
	c.mu.Lock()
	c.sleeps = append(c.sleeps, d)
	c.mu.Unlock()
}

// Sleeps returns all recorded sleep durations.
func (c *MockClock) Sleeps() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]time.Duration, len(c.sleeps))
	copy(result, c.sleeps)
	return result
}

// After returns a channel that receives the time after duration d.
func (c *MockClock) After(d time.Duration) <-chan time.Time {
	timer := c.NewTimer(d)
	return timer.C()
}

// NewTimer creates a new MockTimer.
func (c *MockClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()

	t := &MockTimer{
		ch:       make(chan time.Time, 1),
		deadline: c.now.Add(d),
		duration: d,
	}
	c.timers = append(c.timers, t)
	return t
}

// NewTicker creates a new MockTicker.
func (c *MockClock) NewTicker(d time.Duration) Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()

	t := &MockTicker{
		ch:       make(chan time.Time, 1),
		interval: d,
		nextTick: c.now.Add(d),
		duration: d,
	}
	c.tickers = append(c.tickers, t)
	return t
}

// MockTimer is a manually controlled timer for testing.
type MockTimer struct {
	mu       sync.Mutex
	ch       chan time.Time
	deadline time.Time
	duration time.Duration
	stopped  bool
	fired    bool
}

// C returns the timer channel.
func (t *MockTimer) C() <-chan time.Time {
	return t.ch
}

// Stop prevents the timer from firing.
func (t *MockTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasActive := !t.stopped && !t.fired
	t.stopped = true
	return wasActive
}

// Reset changes the timer to expire after duration d.
func (t *MockTimer) Reset(d time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasActive := !t.stopped && !t.fired
	t.stopped = false
	t.fired = false
	t.duration = d
	return wasActive
}

func (t *MockTimer) checkAndFire(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopped || t.fired {
		return
	}

	if now.After(t.deadline) || now.Equal(t.deadline) {
		t.fired = true
		select {
		case t.ch <- now:
		default:
		}
	}
}

// MockTicker is a manually controlled ticker for testing.
type MockTicker struct {
	mu       sync.Mutex
	ch       chan time.Time
	interval time.Duration
	nextTick time.Time
	duration time.Duration
	stopped  bool
}

// C returns the ticker channel.
func (t *MockTicker) C() <-chan time.Time {
	return t.ch
}

// Stop turns off the ticker.
func (t *MockTicker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
}

// Reset stops a ticker and resets its period to the specified duration.
func (t *MockTicker) Reset(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = false
	t.duration = d
}

// Trigger manually sends a tick with the given time.
func (t *MockTicker) Trigger(now time.Time) {
	select {
	case t.ch <- now:
	default:
	}
}

func (t *MockTicker) checkAndFire(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopped {
		return
	}

	if now.After(t.nextTick) || now.Equal(t.nextTick) {
		select {
		case t.ch <- now:
		default:
		}
		t.nextTick = now.Add(t.interval)
	}
}
