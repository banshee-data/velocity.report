package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

const version = "0.1.0"

var DebugMode bool

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
	case "fix":
		handleFix(args)
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
		fmt.Printf("velocity-deploy version %s\\n", version)
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
                       Can be a hostname, IP, or SSH config host alias
  --ssh-user <user>    SSH user for remote deployment
                       Defaults to ~/.ssh/config or current user
  --ssh-key <path>     SSH private key path
                       Defaults to ~/.ssh/config
  --config <file>      Configuration file path
  --dry-run            Show what would be done without executing

SSH Config Support:
  velocity-deploy automatically reads ~/.ssh/config for host configuration.
  If a host is defined in your SSH config, the tool will use:
    - HostName (IP or domain)
    - User
    - IdentityFile (SSH key)

  Command-line flags override SSH config values.

Examples:
  # Install locally
  velocity-deploy install --binary ./app-radar-linux-arm64

  # Install using SSH config host alias
  velocity-deploy install --target mypi --binary ./app-radar-linux-arm64

  # Install on remote Pi with explicit credentials
  velocity-deploy install --target pi@192.168.1.100 --ssh-key ~/.ssh/id_rsa --binary ./app-radar-linux-arm64

  # Check status using SSH config
  velocity-deploy status --target mypi

  # Upgrade local installation
  velocity-deploy upgrade --binary ./app-radar-linux-arm64

  # Health check on remote host
  velocity-deploy health --target mypi

For more information, see: https://github.com/banshee-data/velocity.report`)
}

func handleInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host for installation")
	sshUser := fs.String("ssh-user", "", "SSH user (defaults to ~/.ssh/config or current user)")
	sshKey := fs.String("ssh-key", "", "SSH private key path (defaults to ~/.ssh/config)")
	binaryPath := fs.String("binary", "", "Path to velocity-report binary (required)")
	dbPath := fs.String("db-path", "", "Path to existing database to migrate")
	dryRun := fs.Bool("dry-run", false, "Show what would be done")
	fs.Parse(args)

	if *binaryPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --binary flag is required. Specify the path to the velocity-report binary (e.g., --binary ./app-radar-linux-arm64)")
		fs.Usage()
		os.Exit(1)
	}

	// Resolve SSH config
	resolvedHost, resolvedUser, resolvedKey, identityAgent, err := ResolveSSHTarget(*target, *sshUser, *sshKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve SSH config: %v\n", err)
		os.Exit(1)
	}

	// Use current user if still not set
	if resolvedUser == "" {
		resolvedUser = os.Getenv("USER")
	}

	installer := &Installer{
		Target:        resolvedHost,
		SSHUser:       resolvedUser,
		SSHKey:        resolvedKey,
		IdentityAgent: identityAgent,
		BinaryPath:    *binaryPath,
		DBPath:        *dbPath,
		DryRun:        *dryRun,
	}

	if err := installer.Install(); err != nil {
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		os.Exit(1)
	}
}

func handleUpgrade(args []string) {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", "", "SSH user (defaults to ~/.ssh/config or current user)")
	sshKey := fs.String("ssh-key", "", "SSH private key path (defaults to ~/.ssh/config)")
	binaryPath := fs.String("binary", "", "Path to new velocity-report binary (required)")
	dryRun := fs.Bool("dry-run", false, "Show what would be done")
	noBackup := fs.Bool("no-backup", false, "Skip backup before upgrade")
	fs.Parse(args)

	if *binaryPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --binary flag is required. Specify the path to the velocity-report binary (e.g., --binary ./app-radar-linux-arm64)")
		fs.Usage()
		os.Exit(1)
	}

	// Resolve SSH config
	resolvedHost, resolvedUser, resolvedKey, identityAgent, err := ResolveSSHTarget(*target, *sshUser, *sshKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve SSH config: %v\n", err)
		os.Exit(1)
	}
	if resolvedUser == "" {
		resolvedUser = os.Getenv("USER")
	}

	upgrader := &Upgrader{
		Target:        resolvedHost,
		SSHUser:       resolvedUser,
		SSHKey:        resolvedKey,
		IdentityAgent: identityAgent,
		BinaryPath:    *binaryPath,
		DryRun:        *dryRun,
		NoBackup:      *noBackup,
	}

	if err := upgrader.Upgrade(); err != nil {
		fmt.Fprintf(os.Stderr, "Upgrade failed: %v\n", err)
		os.Exit(1)
	}
}

func handleFix(args []string) {
	fs := flag.NewFlagSet("fix", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", "", "SSH user (defaults to ~/.ssh/config or current user)")
	sshKey := fs.String("ssh-key", "", "SSH private key path (defaults to ~/.ssh/config)")
	binaryPath := fs.String("binary", "", "Path to velocity-report binary (optional, for fixing missing binary)")
	dryRun := fs.Bool("dry-run", false, "Show what would be done")
	debug := fs.Bool("debug", false, "Enable debug logging")
	fs.Parse(args)

	DebugMode = *debug

	// Resolve SSH config
	resolvedHost, resolvedUser, resolvedKey, identityAgent, err := ResolveSSHTarget(*target, *sshUser, *sshKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve SSH config: %v\n", err)
		os.Exit(1)
	}
	if resolvedUser == "" {
		resolvedUser = os.Getenv("USER")
	}

	fixer := &Fixer{
		Target:        resolvedHost,
		SSHUser:       resolvedUser,
		SSHKey:        resolvedKey,
		IdentityAgent: identityAgent,
		BinaryPath:    *binaryPath,
		DryRun:        *dryRun,
	}

	if err := fixer.Fix(); err != nil {
		fmt.Fprintf(os.Stderr, "\nFix completed with errors: %v\n", err)
		os.Exit(1)
	}
}

func handleStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", "", "SSH user (defaults to ~/.ssh/config or current user)")
	sshKey := fs.String("ssh-key", "", "SSH private key path (defaults to ~/.ssh/config)")
	apiPort := fs.Int("api-port", 8080, "API server port")
	timeout := fs.Int("timeout", 30, "Timeout in seconds")
	fs.Parse(args)

	// Resolve SSH config
	resolvedHost, resolvedUser, resolvedKey, identityAgent, err := ResolveSSHTarget(*target, *sshUser, *sshKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve SSH config: %v\n", err)
		os.Exit(1)
	}
	if resolvedUser == "" {
		resolvedUser = os.Getenv("USER")
	}

	monitor := &Monitor{
		Target:        resolvedHost,
		SSHUser:       resolvedUser,
		SSHKey:        resolvedKey,
		IdentityAgent: identityAgent,
		APIPort:       *apiPort,
	}

	// Show spinner while fetching status
	spinner := NewSpinner("Gathering system status...")
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				fmt.Print("\r\033[K") // Clear line
				return
			default:
				fmt.Print(spinner.Next())
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	status, err := monitor.GetStatus(ctx)
	done <- true
	time.Sleep(100 * time.Millisecond) // Give spinner time to clean up

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get status: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(status.FormatStatus())
}

func handleHealth(args []string) {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", "", "SSH user (defaults to ~/.ssh/config or current user)")
	sshKey := fs.String("ssh-key", "", "SSH private key path (defaults to ~/.ssh/config)")
	apiPort := fs.Int("api-port", 8080, "API server port")
	fs.Parse(args)

	// Resolve SSH config
	resolvedHost, resolvedUser, resolvedKey, identityAgent, err := ResolveSSHTarget(*target, *sshUser, *sshKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve SSH config: %v\n", err)
		os.Exit(1)
	}
	if resolvedUser == "" {
		resolvedUser = os.Getenv("USER")
	}

	monitor := &Monitor{
		Target:        resolvedHost,
		SSHUser:       resolvedUser,
		SSHKey:        resolvedKey,
		IdentityAgent: identityAgent,
		APIPort:       *apiPort,
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
	sshUser := fs.String("ssh-user", "", "SSH user (defaults to ~/.ssh/config or current user)")
	sshKey := fs.String("ssh-key", "", "SSH private key path (defaults to ~/.ssh/config)")
	dryRun := fs.Bool("dry-run", false, "Show what would be done")
	fs.Parse(args)

	// Resolve SSH config
	resolvedHost, resolvedUser, resolvedKey, identityAgent, err := ResolveSSHTarget(*target, *sshUser, *sshKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve SSH config: %v\n", err)
		os.Exit(1)
	}
	if resolvedUser == "" {
		resolvedUser = os.Getenv("USER")
	}

	rollback := &Rollback{
		Target:        resolvedHost,
		SSHUser:       resolvedUser,
		SSHKey:        resolvedKey,
		IdentityAgent: identityAgent,
		DryRun:        *dryRun,
	}

	if err := rollback.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Rollback failed: %v\n", err)
		os.Exit(1)
	}
}

func handleBackup(args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", "", "SSH user (defaults to ~/.ssh/config or current user)")
	sshKey := fs.String("ssh-key", "", "SSH private key path (defaults to ~/.ssh/config)")
	outputDir := fs.String("output", ".", "Output directory for backup")
	fs.Parse(args)

	// Resolve SSH config
	resolvedHost, resolvedUser, resolvedKey, identityAgent, err := ResolveSSHTarget(*target, *sshUser, *sshKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve SSH config: %v\n", err)
		os.Exit(1)
	}
	if resolvedUser == "" {
		resolvedUser = os.Getenv("USER")
	}

	backup := &Backup{
		Target:        resolvedHost,
		SSHUser:       resolvedUser,
		SSHKey:        resolvedKey,
		IdentityAgent: identityAgent,
		OutputDir:     *outputDir,
	}

	if err := backup.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Backup failed: %v\n", err)
		os.Exit(1)
	}
}

func handleConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	target := fs.String("target", "localhost", "Target host")
	sshUser := fs.String("ssh-user", "", "SSH user (defaults to ~/.ssh/config or current user)")
	sshKey := fs.String("ssh-key", "", "SSH private key path (defaults to ~/.ssh/config)")
	show := fs.Bool("show", false, "Show current configuration")
	edit := fs.Bool("edit", false, "Edit configuration")
	fs.Parse(args)

	// Resolve SSH config
	resolvedHost, resolvedUser, resolvedKey, identityAgent, err := ResolveSSHTarget(*target, *sshUser, *sshKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve SSH config: %v\n", err)
		os.Exit(1)
	}
	if resolvedUser == "" {
		resolvedUser = os.Getenv("USER")
	}

	cfg := &ConfigManager{
		Target:        resolvedHost,
		SSHUser:       resolvedUser,
		SSHKey:        resolvedKey,
		IdentityAgent: identityAgent,
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
