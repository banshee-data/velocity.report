package radar

import (
	"bufio"
	"context"
	"io"
	"log"

	"go.bug.st/serial"
)

type RadarPortInterface interface {
	Events() <-chan string
	Monitor(ctx context.Context) error
	SendCommand(command string)
	Close() error
}

type MockRadarPort struct {
	Data       io.Reader
	EventsChan chan string
	// commands chan string
}

func (m *MockRadarPort) Events() <-chan string {
	return m.EventsChan
}

func (m *MockRadarPort) SendCommand(command string) {
	log.Printf("got command %q", command)
}

func (m *MockRadarPort) Monitor(ctx context.Context) error {
	scan := bufio.NewScanner(m.Data)

	for scan.Scan() {
		line := scan.Text()
		m.EventsChan <- line
	}

	<-ctx.Done()
	return nil
}

func (m *MockRadarPort) Close() error {
	return nil
}

type RadarPort struct {
	serial.Port
	events   chan string
	commands chan string
}

func NewRadarPort(portName string) (*RadarPort, error) {
	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: 1,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, err
	}

	events := make(chan string)
	commands := make(chan string)

	return &RadarPort{port, events, commands}, nil
}

// Events returns a chanel for receiving parsed from monitoring the radar serial
// port.
func (p *RadarPort) Events() <-chan string {
	return p.events
}

// Close closes the serial port.
func (p *RadarPort) Close() error {
	if err := p.Port.Close(); err != nil {
		return err
	}
	return nil
}

func (p *RadarPort) SendCommand(command string) {
	// send command to the serial port
	p.commands <- command
}

func (p *RadarPort) writeCommand(command string) error {
	_, err := p.Port.Write([]byte(command))
	if err != nil {
		log.Printf("❌ Error writing to port: %v", err)
		return err
	}
	return nil
}

// Monitor reads from the serial port and sends lines to the events channel.
func (p *RadarPort) Monitor(ctx context.Context) error {
	defer p.Close()
	scan := bufio.NewScanner(p.Port)

	// combination of for & select is the concurrent "while true" loop that
	// awaits for many possible events but executes only one at a time.
	for {
		select {
		// check if the context is done
		// and exit the loop if it is
		case <-ctx.Done():
			return nil
		// check if there is a command to send
		// and send it to the serial port
		case command := <-p.commands:
			if err := p.writeCommand(command); err != nil {
				log.Printf("❌ Error writing command to port: %v", err)
			}
		// otherwise, read from the serial port
		// and send the line to the events channel
		default:
			if !scan.Scan() {
				return scan.Err()
			}
			line := scan.Text()
			log.Printf("🔍 Full Serial Line: [%s]", line)

			select {
			case p.events <- line:
				// Message sent successfully
			case <-ctx.Done():
				return nil
			}
		}
	}
}
