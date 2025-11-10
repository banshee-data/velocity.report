package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	flag.Usage = printUsage
	flag.Parse()

	if flag.NArg() < 1 {
		printUsage()
		os.Exit(1)
	}

	command := flag.Arg(0)
	args := flag.Args()[1:]

	switch command {
	case "install":
		handleInstall(args)
	case "upgrade":
		handleUpgrade(args)
	case "status":
		handleStatus(args)
	case "health":
		handleHealth(args)
	case "rollback":
		handleRollback(args)
	case "backup":
		handleBackup(args)
	case "config":
		handleConfig(args)
	case "version":
		fmt.Printf("velocity-deploy version %s\n", version)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`velocity-deploy - Deployment manager for velocity.report

Usage: velocity-deploy <command> [options]

Commands:
  install    Install velocity.report service on a host
  upgrade    Upgrade velocity.report to a newer version
  status     Check service status
  health     Perform health check on running service
  rollback   Rollback to previous version
  backup     Backup database and configuration
  config     Manage deployment configuration
  version    Show velocity-deploy version
  help       Show this help message

Common Flags:
  --target <host>      Target host (default: localhost)
  --ssh-user <user>    SSH user for remote deployment (default: current user)
  --ssh-key <path>     SSH private key path
  --config <file>      Configuration file path
  --dry-run            Show what would be done without executing

Examples:
  # Install locally
  velocity-deploy install --binary ./app-radar-linux-arm64

  # Install on remote Pi
  velocity-deploy install --target pi@192.168.1.100 --ssh-key ~/.ssh/id_rsa --binary ./app-radar-linux-arm64

  # Check status of remote service
  velocity-deploy status --target pi@192.168.1.100

  # Upgrade local installation
  velocity-deploy upgrade --binary ./app-radar-linux-arm64

  # Health check
  velocity-deploy health --target localhost

For more information, see: https://github.com/banshee-data/velocity.report`)
}

func handleInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host for installation")
	sshUser := fs.String("ssh-user", os.Getenv("USER"), "SSH user")
	sshKey := fs.String("ssh-key", "", "SSH private key path")
	binaryPath := fs.String("binary", "", "Path to velocity-report binary (required)")
	dbPath := fs.String("db-path", "", "Path to existing database to migrate")
	dryRun := fs.Bool("dry-run", false, "Show what would be done")
	fs.Parse(args)

	if *binaryPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --binary flag is required")
		fs.Usage()
		os.Exit(1)
	}

	installer := &Installer{
		Target:     *target,
		SSHUser:    *sshUser,
		SSHKey:     *sshKey,
		BinaryPath: *binaryPath,
		DBPath:     *dbPath,
		DryRun:     *dryRun,
	}

	if err := installer.Install(); err != nil {
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		os.Exit(1)
	}
}

func handleUpgrade(args []string) {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", os.Getenv("USER"), "SSH user")
	sshKey := fs.String("ssh-key", "", "SSH private key path")
	binaryPath := fs.String("binary", "", "Path to new velocity-report binary (required)")
	dryRun := fs.Bool("dry-run", false, "Show what would be done")
	noBackup := fs.Bool("no-backup", false, "Skip backup before upgrade")
	fs.Parse(args)

	if *binaryPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --binary flag is required")
		fs.Usage()
		os.Exit(1)
	}

	upgrader := &Upgrader{
		Target:     *target,
		SSHUser:    *sshUser,
		SSHKey:     *sshKey,
		BinaryPath: *binaryPath,
		DryRun:     *dryRun,
		NoBackup:   *noBackup,
	}

	if err := upgrader.Upgrade(); err != nil {
		fmt.Fprintf(os.Stderr, "Upgrade failed: %v\n", err)
		os.Exit(1)
	}
}

func handleStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", os.Getenv("USER"), "SSH user")
	sshKey := fs.String("ssh-key", "", "SSH private key path")
	fs.Parse(args)

	monitor := &Monitor{
		Target:  *target,
		SSHUser: *sshUser,
		SSHKey:  *sshKey,
	}

	status, err := monitor.GetStatus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get status: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(status)
}

func handleHealth(args []string) {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", os.Getenv("USER"), "SSH user")
	sshKey := fs.String("ssh-key", "", "SSH private key path")
	apiPort := fs.Int("api-port", 8080, "API server port")
	fs.Parse(args)

	monitor := &Monitor{
		Target:  *target,
		SSHUser: *sshUser,
		SSHKey:  *sshKey,
		APIPort: *apiPort,
	}

	health, err := monitor.CheckHealth()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
		os.Exit(1)
	}

	if !health.Healthy {
		fmt.Printf("Service is UNHEALTHY: %s\n", health.Message)
		os.Exit(1)
	}

	fmt.Printf("Service is HEALTHY\n%s\n", health.Details)
}

func handleRollback(args []string) {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", os.Getenv("USER"), "SSH user")
	sshKey := fs.String("ssh-key", "", "SSH private key path")
	dryRun := fs.Bool("dry-run", false, "Show what would be done")
	fs.Parse(args)

	rollback := &Rollback{
		Target:  *target,
		SSHUser: *sshUser,
		SSHKey:  *sshKey,
		DryRun:  *dryRun,
	}

	if err := rollback.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Rollback failed: %v\n", err)
		os.Exit(1)
	}
}

func handleBackup(args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", os.Getenv("USER"), "SSH user")
	sshKey := fs.String("ssh-key", "", "SSH private key path")
	outputDir := fs.String("output", ".", "Output directory for backup")
	fs.Parse(args)

	backup := &Backup{
		Target:    *target,
		SSHUser:   *sshUser,
		SSHKey:    *sshKey,
		OutputDir: *outputDir,
	}

	if err := backup.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Backup failed: %v\n", err)
		os.Exit(1)
	}
}

func handleConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", os.Getenv("USER"), "SSH user")
	sshKey := fs.String("ssh-key", "", "SSH private key path")
	show := fs.Bool("show", false, "Show current configuration")
	edit := fs.Bool("edit", false, "Edit configuration")
	fs.Parse(args)

	cfg := &ConfigManager{
		Target:  *target,
		SSHUser: *sshUser,
		SSHKey:  *sshKey,
	}

	if *show {
		if err := cfg.Show(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to show config: %v\n", err)
			os.Exit(1)
		}
	} else if *edit {
		if err := cfg.Edit(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to edit config: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Use --show or --edit flag")
		fs.Usage()
		os.Exit(1)
	}
}
