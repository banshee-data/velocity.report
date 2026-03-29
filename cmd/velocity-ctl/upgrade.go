package main

import "flag"

func runUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	checkOnly := fs.Bool("check", false, "Check for updates without applying")
	binaryFile := fs.String("binary", "", "Apply a local binary file (offline upgrade)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return ctlManager.RunUpgrade(*checkOnly, *binaryFile)
}
