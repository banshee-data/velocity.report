# velocity-deploy - Deployment Manager

`velocity-deploy` is a comprehensive deployment and management tool for velocity.report installations. It handles installation, upgrades, monitoring, and maintenance of velocity.report services on both local and remote hosts.

## Features

- **Installation**: Set up velocity.report service on local or remote hosts
- **Upgrades**: Safely upgrade to new versions with automatic backup
- **Rollback**: Revert to previous version if upgrade fails
- **Monitoring**: Check service status and health
- **Backup**: Create backups of binary and database
- **Configuration**: View and edit service configuration
- **Remote Management**: Deploy and manage via SSH

## Installation

Build the tool:

```bash
make build-deploy
```

This creates `velocity-deploy` binary in the repository root.

## Usage

### Install on Local Host

```bash
# Build the radar binary first
make build-radar-linux

# Install locally
./velocity-deploy install --binary ./velocity-report-linux-arm64
```

### Install on Remote Raspberry Pi

```bash
# Build for ARM64
make build-radar-linux

# Deploy to remote Pi
./velocity-deploy install \
  --target pi@192.168.1.100 \
  --ssh-key ~/.ssh/id_rsa \
  --binary ./velocity-report-linux-arm64
```

### Check Service Status

```bash
# Local
./velocity-deploy status

# Remote
./velocity-deploy status --target pi@192.168.1.100 --ssh-key ~/.ssh/id_rsa
```

### Health Check

```bash
# Comprehensive health check including API endpoint
./velocity-deploy health --target pi@192.168.1.100
```

The health check verifies:

- Systemd service is running
- No excessive errors in logs
- API endpoint is responding
- Database file exists
- Service uptime

### Upgrade Installation

```bash
# Build new binary
make build-radar-linux

# Upgrade (creates backup automatically)
./velocity-deploy upgrade \
  --target pi@192.168.1.100 \
  --binary ./velocity-report-linux-arm64
```

The upgrade process:

1. Checks current installation
2. Creates backup of binary and database
3. Stops service
4. Installs new binary
5. Starts service
6. Verifies health

### Rollback to Previous Version

If an upgrade fails or causes issues:

```bash
./velocity-deploy rollback --target pi@192.168.1.100
```

This restores the most recent backup.

### Backup Database and Configuration

```bash
# Create backup
./velocity-deploy backup \
  --target pi@192.168.1.100 \
  --output ./backups
```

Creates timestamped backup directory containing:

- Binary
- Database
- Service file
- Metadata file

### View Configuration

```bash
./velocity-deploy config --show --target pi@192.168.1.100
```

Shows:

- Service file contents
- Data directory listing
- Service status
- Recent logs

### Edit Configuration

```bash
./velocity-deploy config --edit --target pi@192.168.1.100
```

Interactive editor for service configuration, allowing you to modify:

- API port (`--listen`)
- Speed units (`--units`)
- Timezone (`--timezone`)
- Enable/disable features

## Command Reference

### Global Flags

| Flag         | Description                               | Default             |
| ------------ | ----------------------------------------- | ------------------- |
| `--target`   | Target host (user@host or hostname)       | `localhost`         |
| `--ssh-user` | SSH username                              | Current user        |
| `--ssh-key`  | Path to SSH private key                   | None (uses default) |
| `--dry-run`  | Show what would be done without executing | `false`             |

### Commands

#### install

Install velocity.report service.

**Required flags:**

- `--binary`: Path to velocity-report binary

**Optional flags:**

- `--db-path`: Path to existing database to migrate

**Example:**

```bash
velocity-deploy install --binary ./velocity-report-linux-arm64 --db-path ./sensor_data.db
```

#### upgrade

Upgrade to a new version.

**Required flags:**

- `--binary`: Path to new velocity-report binary

**Optional flags:**

- `--no-backup`: Skip backup before upgrade

**Example:**

```bash
velocity-deploy upgrade --binary ./velocity-report-linux-arm64
```

#### status

Check systemd service status.

**Example:**

```bash
velocity-deploy status --target pi@192.168.1.100
```

#### health

Perform comprehensive health check.

**Optional flags:**

- `--api-port`: API server port (default: 8080)

**Example:**

```bash
velocity-deploy health --target pi@192.168.1.100 --api-port 8080
```

#### rollback

Rollback to previous version from backup.

**Example:**

```bash
velocity-deploy rollback --target pi@192.168.1.100
```

#### fix

Diagnose and repair broken installations. Automatically detects and fixes common issues:

- Missing or incorrectly configured service user
- Missing or wrong permissions on data directory
- Database in wrong location (moves to `/var/lib/velocity-report/`)
- Missing or outdated binary
- Missing systemd service file
- Service not enabled
- Incorrect service configuration (missing `--db-path`)
- Database schema version mismatch

**Optional flags:**

- `--binary`: Path to velocity-report binary (for fixing missing/broken binary)
- `--build-from-source`: Build binary from source on the server (requires Go and build tools)
- `--repo-url`: Git repository URL for source code (default: `https://github.com/banshee-data/velocity.report`)

**Source Code Management:**

The `fix` command automatically clones/updates the source code repository to `/opt/velocity-report/`. This is essential for:

- Running Python PDF generation scripts
- Building from source on the server
- Ensuring tools and utilities are available

**Build from Source:**

When `--build-from-source` is specified, the fixer will:

1. Check for Go installation on the server
2. Install `libpcap-dev` if available (for pcap support)
3. Build the binary directly on the server
4. Install the built binary

This is especially useful for:

- Linux-specific builds with pcap support (cross-compilation is challenging)
- Ensuring binary matches the exact target architecture
- Development and testing scenarios

**Examples:**

```bash
# Basic fix - diagnose and repair common issues
velocity-deploy fix --target pi@192.168.1.100

# Fix with binary replacement
velocity-deploy fix --target pi@192.168.1.100 --binary ./velocity-report-linux-arm64

# Build from source on server (requires Go)
velocity-deploy fix --target pi@192.168.1.100 --build-from-source

# Use custom repository URL
velocity-deploy fix --target pi@192.168.1.100 --repo-url https://github.com/myorg/velocity.report

# Dry run to see what would be fixed
velocity-deploy fix --target pi@192.168.1.100 --dry-run
```

**What Gets Checked:**

1. ✅ Service user `velocity` exists
2. ✅ Data directory `/var/lib/velocity-report` exists with correct permissions
3. ✅ Source code repository cloned to `/opt/velocity-report`
4. ✅ Binary exists at `/usr/local/bin/velocity-report` and is executable
5. ✅ Systemd service file exists
6. ✅ Service is enabled
7. ✅ Database exists in correct location
8. ✅ Service configured with `--db-path` flag
9. ⚠️ Database schema is up to date (warning only)

#### backup

Create backup of installation.

**Optional flags:**

- `--output`: Output directory for backup (default: current directory)

**Example:**

```bash
velocity-deploy backup --target pi@192.168.1.100 --output ./backups
```

#### config

Manage configuration.

**Required flags (one of):**

- `--show`: Display current configuration
- `--edit`: Edit configuration interactively

**Example:**

```bash
velocity-deploy config --show
velocity-deploy config --edit
```

#### version

Show velocity-deploy version.

**Example:**

```bash
velocity-deploy version
```

## Architecture

### Installation Paths

- **Binary**: `/usr/local/bin/velocity-report`
- **Data Directory**: `/var/lib/velocity-report/`
- **Database**: `/var/lib/velocity-report/sensor_data.db`
- **Service File**: `/etc/systemd/system/velocity-report.service`
- **Backups**: `/var/lib/velocity-report/backups/`

### Service User

The service runs as dedicated user `velocity` with:

- No shell access (`/usr/sbin/nologin`)
- Ownership of data directory
- Restricted permissions

### Systemd Service

Standard systemd service with:

- Automatic restart on failure
- Journal logging
- Dependency on network

## SSH Configuration

`velocity-deploy` automatically reads `~/.ssh/config` for host configuration, making remote deployments easier.

### Using SSH Config

Define your hosts in `~/.ssh/config`:

```ssh-config
Host mypi
    HostName 192.168.1.100
    User pi
    IdentityFile ~/.ssh/id_rsa_pi

Host production
    HostName velocity.example.com
    User admin
    IdentityFile ~/.ssh/id_rsa_prod
    Port 2222
```

Then deploy using the host alias:

```bash
# Uses all config from SSH config file
velocity-deploy install --target mypi --binary ./velocity-report-linux-arm64

# Check status
velocity-deploy status --target mypi

# Upgrade
velocity-deploy upgrade --target production --binary ./velocity-report-linux-arm64
```

### Manual SSH Configuration

If not using SSH config, ensure:

1. **SSH access** is configured:

   ```bash
   ssh-copy-id pi@192.168.1.100
   ```

2. **Sudo access** without password (for pi user):

   ```bash
   # On the target host
   echo "pi ALL=(ALL) NOPASSWD: ALL" | sudo tee /etc/sudoers.d/pi
   ```

3. **Provide credentials explicitly**:
   ```bash
   velocity-deploy install --ssh-key ~/.ssh/id_rsa --target pi@192.168.1.100 --binary ./velocity-report-linux-arm64
   ```

### Override SSH Config

Command-line flags override SSH config values:

```bash
# Use SSH config host but override user
velocity-deploy status --target mypi --ssh-user admin

# Use SSH config but override key
velocity-deploy install --target mypi --ssh-key ~/.ssh/different_key --binary ./velocity-report-linux-arm64
```

## Troubleshooting

### Installation fails with "permission denied"

Ensure you're running with appropriate privileges:

- Local: May need `sudo` for system directories
- Remote: Target user needs sudo access

### Health check fails on API endpoint

Check:

1. Service is running: `velocity-deploy status`
2. Firewall allows port 8080
3. API port matches config: `--api-port`

### Rollback fails with "no backups found"

Backups are created automatically during upgrades. If none exist:

1. Create backup first: `velocity-deploy backup`
2. Or reinstall: `velocity-deploy install`

### Remote deployment hangs

Check:

1. SSH connectivity: `ssh user@host`
2. Network latency
3. Target host resources (disk space, memory)

## Migration from setup-radar-host.sh

If you previously used `scripts/setup-radar-host.sh`:

1. The new tool is fully compatible
2. Existing installations work as-is
3. Can upgrade using: `velocity-deploy upgrade`
4. Configuration is preserved

## Development

### Building

```bash
# Build for local development
go build -o velocity-deploy ./cmd/deploy

# Build for Linux ARM64 (Raspberry Pi)
GOOS=linux GOARCH=arm64 go build -o velocity-deploy-linux-arm64 ./cmd/deploy
```

### Testing

Create a test environment:

```bash
# Use dry-run mode
./velocity-deploy install --binary ./test-binary --dry-run

# Test on local machine first
./velocity-deploy install --binary ./velocity-report-linux-arm64 --target localhost
```

## License

Apache License 2.0 - See [LICENSE](../../LICENSE) for details.
