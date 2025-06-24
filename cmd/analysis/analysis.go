package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	tk "modernc.org/tk9.0"
)

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		log.Printf("Unsupported platform: %s", runtime.GOOS)
		return
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}

func main() {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("failed to create listener for HTTP server %v", err)
	}
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Hello, World!")
		})
		log.Printf("HTTP server listening on %s", ln.Addr().String())
		if err := http.Serve(ln, mux); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://localhost:%d", port)
	button := tk.Button(
		tk.Txt("Open Browser"),
		tk.Command(func() {
			openBrowser(serverURL)
		}),
	)
	tk.Pack(button)
	tk.App.Wait()
}
