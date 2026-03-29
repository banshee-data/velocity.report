package main

import (
	"flag"
)

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	return ctlManager.RunStatus()
}
