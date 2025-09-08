package main

import (
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"
)

func main() {
	// Listen on UDP port 2368
	addr, err := net.ResolveUDPAddr("udp", ":2368")
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	fmt.Println("UDP listener started on port 2368")

	var packetCount int64
	var byteCount int64

	// Statistics goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			packets := atomic.SwapInt64(&packetCount, 0)
			bytes := atomic.SwapInt64(&byteCount, 0)
			if packets > 0 {
				fmt.Printf("Received: %d packets/sec, %.1f KB/sec\n",
					packets, float64(bytes)/1024)
			}
		}
	}()

	// Main receive loop
	buffer := make([]byte, 65536)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Read error: %v", err)
			continue
		}

		atomic.AddInt64(&packetCount, 1)
		atomic.AddInt64(&byteCount, int64(n))
	}
}
