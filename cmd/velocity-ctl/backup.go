package main

import (
	"flag"
)

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	outputDir := fs.String("output", "", "Directory to store backups")
	handled, err := parseCommandFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	_, err = ctlManager.RunBackup(*outputDir)
	return err
}
