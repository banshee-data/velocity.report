package deploy

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SSHConfig represents parsed SSH configuration for a host.
type SSHConfig struct {
	Host          string
	HostName      string
	User          string
	IdentityFile  string
	IdentityAgent string
	Port          string
}

// ParseSSHConfig reads and parses ~/.ssh/config for the given host.
func ParseSSHConfig(host string) (*SSHConfig, error) {
	return ParseSSHConfigFrom(host, "")
}

// ParseSSHConfigFrom reads and parses an SSH config file for the given host.
// If configPath is empty, uses ~/.ssh/config.
func ParseSSHConfigFrom(host, configPath string) (*SSHConfig, error) {
	var homeDir string
	if configPath == "" {
		// Check HOME environment variable first (for testing)
		homeDir = os.Getenv("HOME")
		if homeDir == "" {
			var err error
			homeDir, err = os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
		}
		configPath = filepath.Join(homeDir, ".ssh", "config")
	} else {
		homeDir = os.Getenv("HOME")
		if homeDir == "" {
			homeDir, _ = os.UserHomeDir()
		}
	}

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist, return nil without error
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open SSH config: %w", err)
	}
	defer file.Close()

	return parseSSHConfigReader(host, file, homeDir)
}

// parseSSHConfigReader parses SSH config from a reader for testability.
func parseSSHConfigReader(host string, file *os.File, homeDir string) (*SSHConfig, error) {
	var currentHost string
	config := &SSHConfig{Host: host}
	inMatchingHost := false

	foundMatch := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first whitespace
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		keyword := strings.ToLower(parts[0])
		value := strings.Join(parts[1:], " ")

		switch keyword {
		case "host":
			// If we were in a matching host and hit a new Host line, we're done
			if inMatchingHost {
				return config, nil
			}
			// Check if this is the host we're looking for
			currentHost = parts[1]
			inMatchingHost = MatchHost(host, currentHost)
			if inMatchingHost {
				foundMatch = true
			}

		case "hostname":
			if inMatchingHost {
				config.HostName = value
			}

		case "user":
			if inMatchingHost {
				config.User = value
			}

		case "identityfile":
			if inMatchingHost {
				// Expand ~ to home directory
				if strings.HasPrefix(value, "~/") && homeDir != "" {
					value = filepath.Join(homeDir, value[2:])
				}
				config.IdentityFile = value
			}

		case "port":
			if inMatchingHost {
				config.Port = value
			}

		case "identityagent":
			if inMatchingHost {
				// Remove quotes if present
				value = strings.Trim(value, `"`)
				// Expand ~ to home directory
				if strings.HasPrefix(value, "~/") && homeDir != "" {
					value = filepath.Join(homeDir, value[2:])
				}
				config.IdentityAgent = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading SSH config: %w", err)
	}

	// If we didn't find any configuration for this host, return nil
	if !foundMatch {
		return nil, nil
	}

	return config, nil
}

// MatchHost checks if the target host matches the SSH config host pattern.
func MatchHost(target, pattern string) bool {
	// Simple exact match for now
	// TODO: Add support for wildcards (* and ?) if needed
	return target == pattern
}

// ResolveSSHTarget resolves SSH connection details using ~/.ssh/config.
// Returns: hostname, user, keyPath, identityAgent, error.
func ResolveSSHTarget(target, user, keyPath string) (string, string, string, string, error) {
	// If target contains @, split it
	targetHost := target
	targetUser := user
	if strings.Contains(target, "@") {
		parts := strings.SplitN(target, "@", 2)
		targetUser = parts[0]
		targetHost = parts[1]
	}

	// Try to parse SSH config for this host
	config, err := ParseSSHConfig(targetHost)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to parse SSH config: %w", err)
	}

	// If no config found, use provided values
	if config == nil {
		return targetHost, targetUser, keyPath, "", nil
	}

	// Use config values, with command-line overrides
	finalHost := targetHost
	if config.HostName != "" {
		finalHost = config.HostName
	}

	finalUser := targetUser
	if finalUser == "" && config.User != "" {
		finalUser = config.User
	}

	finalKey := keyPath
	if finalKey == "" && config.IdentityFile != "" {
		finalKey = config.IdentityFile
	}

	finalAgent := config.IdentityAgent

	return finalHost, finalUser, finalKey, finalAgent, nil
}
