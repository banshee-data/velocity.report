package main

import (
	"flag"
)

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	outputDir := fs.String("output", "", "Directory to store backups")
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, err := ctlManager.RunBackup(*outputDir)
	return err
}
