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
./velocity-deploy install --binary ./app-radar-linux-arm64
```

### Install on Remote Raspberry Pi

```bash
# Build for ARM64
make build-radar-linux

# Deploy to remote Pi
./velocity-deploy install \
  --target pi@192.168.1.100 \
  --ssh-key ~/.ssh/id_rsa \
  --binary ./app-radar-linux-arm64
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
  --binary ./app-radar-linux-arm64
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

| Flag | Description | Default |
|------|-------------|---------|
| `--target` | Target host (user@host or hostname) | `localhost` |
| `--ssh-user` | SSH username | Current user |
| `--ssh-key` | Path to SSH private key | None (uses default) |
| `--dry-run` | Show what would be done without executing | `false` |

### Commands

#### install

Install velocity.report service.

**Required flags:**
- `--binary`: Path to velocity-report binary

**Optional flags:**
- `--db-path`: Path to existing database to migrate

**Example:**
```bash
velocity-deploy install --binary ./app-radar-linux-arm64 --db-path ./sensor_data.db
```

#### upgrade

Upgrade to a new version.

**Required flags:**
- `--binary`: Path to new velocity-report binary

**Optional flags:**
- `--no-backup`: Skip backup before upgrade

**Example:**
```bash
velocity-deploy upgrade --binary ./app-radar-linux-arm64
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

For remote deployment, ensure:

1. **SSH access** is configured:
   ```bash
   ssh-copy-id pi@192.168.1.100
   ```

2. **Sudo access** without password (for pi user):
   ```bash
   # On the target host
   echo "pi ALL=(ALL) NOPASSWD: ALL" | sudo tee /etc/sudoers.d/pi
   ```

3. **SSH key** is available:
   ```bash
   velocity-deploy install --ssh-key ~/.ssh/id_rsa --target pi@192.168.1.100 --binary ./app-radar-linux-arm64
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
./velocity-deploy install --binary ./app-radar-linux-arm64 --target localhost
```

## Future Enhancements

Planned features:

- [ ] Multi-host orchestration
- [ ] Configuration file for managing multiple sites
- [ ] Automatic version checking and updates
- [ ] Prometheus metrics export
- [ ] Email notifications for health issues
- [ ] Web UI for deployment management

## Contributing

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for development guidelines.

## License

Apache License 2.0 - See [LICENSE](../../LICENSE) for details.
