package device

import (
	"flag"
)

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	handled, err := parseCommandFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	return ctlManager.RunStatus()
}
