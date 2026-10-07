package server

import "testing"

func TestHardenedListenerContainment(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8081", "[::1]:50051", "localhost:8082"} {
		if err := validateHardenedListeners(address, address, address); err != nil {
			t.Fatal(err)
		}
	}
	for _, address := range []string{":50051", "0.0.0.0:8081", "[::]:8082", "192.168.1.2:50051", "example.com:8081", "127.0.0.1:invalid", "127.0.0.1:65536", ""} {
		for position := 0; position < 3; position++ {
			listeners := []string{"127.0.0.1:8082", "127.0.0.1:8081", "localhost:50051"}
			listeners[position] = address
			if err := validateHardenedListeners(listeners[0], listeners[1], listeners[2]); err == nil {
				t.Fatalf("accepted %q in position %d", address, position)
			}
		}
	}
}
