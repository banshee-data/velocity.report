package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cmd := exec.Command("systemctl", "status", serviceName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		// systemctl status exits non-zero when the service is not running.
		// Show the output regardless and return a clear message.
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "\nService is not running (exit code %d).\n", exitErr.ExitCode())
			return nil
		}
		return fmt.Errorf("running systemctl: %w", err)
	}
	return nil
}
