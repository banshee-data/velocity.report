// Serialmux provides an abstraction over a serial port with the ability for
// multiple clients to subscribe to events from the serial port and send
// commands to a single serial port device.
package serialmux

import (
	"bufio"
	"bytes"
	"context"
	crand "crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"tailscale.com/tsweb"
)

var ErrWriteFailed = fmt.Errorf("failed to write to serial port")

//go:embed templates/*
var adminTemplateFS embed.FS

var sendCommandTemplate = template.Must(template.ParseFS(adminTemplateFS, "templates/send-command.html.tmpl"))

// SerialMux is a generic serial port multiplexer that allows multiple clients to
// subscribe to events from a single serial port.
type SerialMux[T SerialPorter] struct {
	port         T
	subscribers  map[string]chan string
	subscriberMu sync.Mutex
	commandMu    sync.Mutex
	closing      bool
	closingMu    sync.Mutex
}

// SerialMuxInterface defines the interface for the SerialMux type.
type SerialMuxInterface interface {
	// Subscribe creates a new channel for receiving line events from the serial
	// port. The channel ID is used to identify the unique channel when
	// unsubscribing.
	Subscribe() (string, chan string)
	// Unsubscribe removes a channel from the list of subscribers.
	Unsubscribe(string)
	// SendCommand writes the provided command to the serial port.
	SendCommand(string) error
	// Monitor reads lines from the serial port and sends them to the
	// appropriate channels.
	Monitor(context.Context) error
	// Close closes all subscribed channels and closes the serial port.
	Close() error

	Initialize() error

	// AttachAdminRoutes attaches admin debugging endpoints to the given HTTP
	// mux served at /debug/. These routes are accessible only over
	// localhost/via Tailscale and are not publicly accessible.
	AttachAdminRoutes(*http.ServeMux)
}

// NewSerialMux creates a SerialMux instance backed by a serial port at the
// given path.
func NewSerialMux[T SerialPorter](port T) *SerialMux[T] {
	return &SerialMux[T]{
		port:         port,
		subscribers:  make(map[string]chan string),
		subscriberMu: sync.Mutex{},
		commandMu:    sync.Mutex{},
	}
}

// randomID generates a random channel ID (8 byte random hex encoded value)
func randomID() string {
	b := make([]byte, 8)
	crand.Read(b)
	return hex.EncodeToString(b)
}

func (s *SerialMux[T]) Subscribe() (string, chan string) {
	id := randomID()
	ch := make(chan string)
	s.subscriberMu.Lock()
	defer s.subscriberMu.Unlock()
	s.subscribers[id] = ch
	return id, ch
}

// Unsubscribe removes a subscriber from the serial mux.
func (s *SerialMux[T]) Unsubscribe(id string) {
	s.subscriberMu.Lock()
	defer s.subscriberMu.Unlock()
	if ch, ok := s.subscribers[id]; ok {
		close(ch)
		delete(s.subscribers, id)
	}
}

// Initialize syncs the clock and TZ offset to the device and sets some default
// output modes to ensure that we can parse the results.
func (s *SerialMux[T]) Initialize() error {
	// sync the clock to the current UNIX time
	command := fmt.Sprintf("C=%d", time.Now().Unix())
	if err := s.SendCommand(command); err != nil {
		return fmt.Errorf("failed to synchronize clock: %w", err)
	}

	// set the TZ name and offset based on current local to format timestamps
	tzName, tsOffsetSeconds := time.Now().Local().Zone()
	command = fmt.Sprintf("CZ%s%d", tzName, tsOffsetSeconds/60/60)
	if err := s.SendCommand(command); err != nil {
		return fmt.Errorf("failed to set timezone: %w", err)
	}

	for _, command := range []string{
		"AX",     // reset to factory defaults
		"OJ",     // set output format to JSON
		"OS",     // enable speed reporting from doppler radar
		"oD",     // enable range reporting from FMCW radar
		"OM",     // enable magnitude of speed measurement
		"oM",     // enable magnitude of range measurement
		"OH",     // enable human-readable timestamp w/ event
		"OC",     // enable object detection
		"^/+0.0", // set inbound direction angle to 0 degrees (for cosine correction)
		"^/-0.0", // set outbound direction angle to 0 degrees (for cosine correction)
	} {
		if err := s.SendCommand(command); err != nil {
			return fmt.Errorf("failed to send start command %q: %w", command, err)
		}
	}

	return nil
}

// SendCommand sends a command to the serial port.
func (s *SerialMux[T]) SendCommand(command string) error {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	if !bytes.HasSuffix([]byte(command), []byte("\n")) {
		command += "\n" // ensure command ends with a newline
	}
	n, err := s.port.Write([]byte(command))
	if err != nil {
		return err
	}
	if n != len(command) {
		return ErrWriteFailed
	}
	return nil
}

// Monitor monitors the serial port for events and sends them to subscribers
func (s *SerialMux[T]) Monitor(ctx context.Context) error {
	scan := bufio.NewScanner(s.port)

	lineChan := make(chan string)
	scanErrChan := make(chan error, 1)

	// start a goroutine to read from the serial port & send any lines that are scanned to linesChan.
	// and any errors to the scanErrChan
	//
	// the blocking scan.Scan will not interfere with our outer loop awaiting
	// lines & context cancellation.
	go func() {
		defer close(lineChan)
		for scan.Scan() {
			select {
			case lineChan <- scan.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scan.Err(); err != nil {
			select {
			case scanErrChan <- err:
			case <-ctx.Done():
			}
		}
	}()

	for {
		select {
		// check if the context is done
		// and exit the loop if so
		case <-ctx.Done():
			return ctx.Err()

		case err := <-scanErrChan:
			return err

		case line, ok := <-lineChan:
			// if the channel is closed, we're done reading from the serial port
			if !ok {
				if err := scan.Err(); err != nil {
					return err
				}
				return nil
			}
			// Check if we're closing
			s.closingMu.Lock()
			if s.closing {
				s.closingMu.Unlock()
				return nil
			}
			s.closingMu.Unlock()

			// otherwise take a read lock on the subscriber map
			s.subscriberMu.Lock()
			for _, ch := range s.subscribers {
				select {
				case ch <- line:
				default:
					// if the channel is full/blocking skip so as not to block the outer loop
				}
			}
			s.subscriberMu.Unlock()
		}
	}
}

func (s *SerialMux[T]) Close() error {
	s.closingMu.Lock()
	s.closing = true
	s.closingMu.Unlock()

	s.subscriberMu.Lock()
	defer s.subscriberMu.Unlock()
	for id, ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, id)
	}
	return s.port.Close()
}

func (s *SerialMux[T]) AttachAdminRoutes(mux *http.ServeMux) {
	debug := tsweb.Debugger(mux)

	// Basic command / live tail monitor interface using the below two API endpoints.
	debug.HandleFunc("send-command", "send a command to the serial port", func(w http.ResponseWriter, r *http.Request) {
		buf := bytes.NewBuffer(nil)
		if err := sendCommandTemplate.Execute(buf, nil); err != nil {
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
			return
		}
		io.Copy(w, buf)
	})

	// API endpoint to write command to the serial port
	debug.HandleSilentFunc("send-command-api", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		command := strings.TrimSpace(r.FormValue("command"))
		if command == "" {
			http.Error(w, "Missing command", http.StatusBadRequest)
			return
		}
		if err := s.SendCommand(command); err != nil {
			http.Error(w, "Failed to write command", http.StatusInternalServerError)
			return
		}
		io.WriteString(w, fmt.Sprintf("Wrote command %q to serial port", command))
	})
	// API endpoint to issue Server-Side Events (SSE) in response to lines coming from the serial port.
	debug.HandleSilentFunc("tail", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // Disable buffering for nginx

		id, c := s.Subscribe()
		defer s.Unsubscribe(id)

		// Send initial ping to establish connection
		w.Write([]byte(": ping\n\n"))
		w.(http.Flusher).Flush()

		for {
			select {
			case payload, ok := <-c:
				if !ok {
					// Channel closed, exit gracefully
					return
				}
				_, err := w.Write([]byte(fmt.Sprintf("data: %s\n\n", payload)))
				if err != nil {
					return
				}
				w.(http.Flusher).Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	debug.HandleSilentFunc("tail.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// serve tail.js from adminTemplateFS
		f, err := adminTemplateFS.Open("templates/tail.js")
		if err != nil {
			http.Error(w, "Failed to open tail.js", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		io.Copy(w, f)
	})
}
