package version

import "fmt"

var (
	// Version is the current application version
	Version = "dev"
	// GitSHA is the git commit SHA
	GitSHA = "unknown"
	// BuildTime is the build timestamp
	BuildTime = "unknown"
)

// Print writes multi-line version information to stdout with aligned labels.
// The binary name is supplied by the caller.
func Print(binary string) {
	fmt.Printf("%s  v%s\n", binary, Version)
	fmt.Printf(" git sha:  %s\n", GitSHA)
	fmt.Printf("   built:  %s\n", BuildTime)
}
