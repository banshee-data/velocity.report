package main

import (
	"log"
	"sync"
	"testing"
	"time"
)

const longData = `{"long_message": "here is a long message, here is a long message, here is a long message, here is a long message, here is a long message, here is a long message, here is a long message, here is a long message, here is a long message, here is a long message, here is a long message"}
12345, 0.2, 0.2
`

func TestSerialReader(t *testing.T) {
	tests := []struct {
		name                  string
		errorMsg              string
		input                 []byte
		expectedCallbackCount int
	}{
		{"good input", "", []byte("12345, 0.0, 0.0\n"), 1},
		{"bad input", "test error", []byte(""), 1},
		{"long input", "", []byte(longData), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock_port := MockSerialPort{errorMessage: tt.errorMsg, buf: tt.input}

			bytesRead := 0
			callbackCount := 0
			mu := sync.Mutex{}

			c := make(chan bool)
			err_chan := make(chan error)

			cb := func(line string) {
				mu.Lock()
				defer mu.Unlock()

				callbackCount++

				log.Printf("callback number (%d) called with: %s", callbackCount, line)
				bytesRead += len(line)

				if callbackCount == tt.expectedCallbackCount {
					c <- true
				}
			}

			go func() {
				err := serialReaderV2(&mock_port, cb)
				if err != nil {
					err_chan <- err
				}
			}()

			timeout := time.After(1 * time.Second)

			select {
			case <-timeout:
				if tt.expectedCallbackCount > callbackCount {
					t.Fatal("timed out")
				}
				if tt.errorMsg != "" {
					t.Fatal("timed out but expected error")
				}
			case <-c:
				if tt.expectedCallbackCount == 0 {
					t.Fatal("callback should not have been called")
				}
			case <-err_chan:
				if tt.errorMsg == "" {
					t.Fatal("unexpected error received")
				}
			}
		})
	}
}
