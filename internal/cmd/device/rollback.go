package device

import (
	"flag"
)

func runRollback(args []string) error {
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	handled, err := parseCommandFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	return ctlManager.RunRollback()
}
