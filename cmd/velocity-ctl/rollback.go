package main

import (
	"flag"
)

func runRollback(args []string) error {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	return ctlManager.RunRollback()
}
