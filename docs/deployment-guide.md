# Deployment Guide

This guide explains how to deploy and manage velocity.report installations using the new `velocity-deploy` tool.

## Overview

`velocity-deploy` is a comprehensive deployment manager that replaces the previous `scripts/setup-radar-host.sh` script with enhanced capabilities:

- **Local and Remote Deployment**: Deploy to localhost or remote hosts via SSH
- **Upgrade Management**: Safely upgrade with automatic backup and rollback
- **Health Monitoring**: Check service status and API health
- **Backup/Restore**: Create and manage backups
- **Configuration Management**: View and edit service configuration

## Quick Start

### 1. Build the Tools

```bash
# Build velocity-deploy
make build-deploy

# Build velocity.report radar binary for your target
make build-radar-linux        # For Raspberry Pi (ARM64)
# or
make build-radar-local        # For local testing
```

### 2. Install Locally

```bash
./velocity-deploy install --binary ./app-radar-linux-arm64
```

This will:
- Install binary to `/usr/local/bin/velocity-report`
- Create service user `velocity`
- Set up data directory `/var/lib/velocity-report/`
- Install and start systemd service

### 3. Install on Remote Raspberry Pi

```bash
# Copy SSH key to Pi (one-time setup)
ssh-copy-id pi@192.168.1.100

# Deploy
./velocity-deploy install \
  --target pi@192.168.1.100 \
  --ssh-key ~/.ssh/id_rsa \
  --binary ./app-radar-linux-arm64
```

## Common Tasks

### Check Service Status

```bash
# Local
./velocity-deploy status

# Remote
./velocity-deploy status --target pi@192.168.1.100
```

### Health Check

```bash
./velocity-deploy health --target pi@192.168.1.100
```

This checks:
- Systemd service is running
- No excessive errors in logs
- API endpoint is responding
- Database file exists

### Upgrade to New Version

```bash
# Build new binary
make build-radar-linux

# Upgrade (creates backup automatically)
./velocity-deploy upgrade --binary ./app-radar-linux-arm64 --target pi@192.168.1.100
```

The upgrade process:
1. Creates backup of current installation
2. Stops service
3. Installs new binary
4. Starts service
5. Verifies health

### Rollback After Failed Upgrade

```bash
./velocity-deploy rollback --target pi@192.168.1.100
```

### Create Backup

```bash
./velocity-deploy backup --target pi@192.168.1.100 --output ./backups
```

Creates timestamped backup with:
- Binary
- Database
- Service file
- Metadata

### View Configuration

```bash
./velocity-deploy config --show --target pi@192.168.1.100
```

Shows:
- Service file
- Data directory
- Service status
- Recent logs

### Edit Configuration

```bash
./velocity-deploy config --edit --target pi@192.168.1.100
```

Allows you to modify service configuration, such as:
- API port
- Speed units
- Timezone
- Feature flags

## Migration from setup-radar-host.sh

If you previously used `scripts/setup-radar-host.sh`:

1. **No migration needed**: Existing installations work with the new tool
2. **Can upgrade**: Use `velocity-deploy upgrade` to update
3. **Configuration preserved**: All settings remain the same

### Comparison

| Feature | setup-radar-host.sh | velocity-deploy |
|---------|---------------------|-----------------|
| Local install | ✓ | ✓ |
| Remote install | ✗ | ✓ |
| Upgrade | ✗ | ✓ |
| Backup | ✗ | ✓ |
| Rollback | ✗ | ✓ |
| Health check | ✗ | ✓ |
| Config management | ✗ | ✓ |

## Make Targets

For convenience, several Make targets are available:

```bash
# Build the deploy tool
make build-deploy

# Install locally (builds binary and deploy tool if needed)
make deploy-install

# Upgrade local installation
make deploy-upgrade

# Check status
make deploy-status

# Health check
make deploy-health
```

## SSH Setup for Remote Deployment

### Prerequisites

1. **SSH Access**:
   ```bash
   ssh-copy-id pi@192.168.1.100
   ```

2. **Sudo Without Password** (recommended for automation):
   On the target host:
   ```bash
   echo "pi ALL=(ALL) NOPASSWD: ALL" | sudo tee /etc/sudoers.d/pi
   ```

   > **⚠️ Security Warning**: Using `NOPASSWD: ALL` grants unlimited sudo access without password verification. For production environments, consider restricting sudo to specific commands:
   > ```bash
   > # More secure: Only allow specific commands needed by velocity-deploy
   > echo "pi ALL=(ALL) NOPASSWD: /bin/systemctl, /bin/cp, /bin/mv, /bin/mkdir, /bin/chown, /bin/chmod, /bin/cat, /bin/test, /bin/rm, /usr/bin/journalctl, /bin/ls, /bin/du, /usr/bin/stat, /usr/sbin/useradd" | sudo tee /etc/sudoers.d/pi
   > ```

3. **Test Connection**:
   ```bash
   ssh pi@192.168.1.100 'sudo systemctl status'
   ```

### Security Considerations

- Use SSH key authentication (not passwords)
- Keep private keys secure
- Consider using SSH agent for key management
- Restrict sudo access to specific commands when possible
- Ensure your SSH known_hosts file is properly configured
- Avoid using `--insecure-ssh` flag in production environments

## Troubleshooting

### Installation Fails with "permission denied"

**Problem**: Insufficient permissions to install system files.

**Solution**:
- Local: User needs sudo access
- Remote: Target user needs sudo access

### Health Check Fails on API Endpoint

**Problem**: API is not responding.

**Check**:
1. Service is running: `velocity-deploy status`
2. Port 8080 is accessible (firewall rules)
3. Service logs: `sudo journalctl -u velocity-report.service -n 50`

### Rollback Fails with "no backups found"

**Problem**: No automatic backup was created.

**Solution**:
- Backups are created during upgrades
- Manually create backup first: `velocity-deploy backup`
- Or reinstall: `velocity-deploy install`

### Remote Deployment Hangs

**Problem**: Network or resource issues.

**Check**:
1. SSH connectivity: `ssh user@host`
2. Disk space on target: `df -h`
3. Memory available: `free -h`
4. Check for slow operations in logs

## Advanced Usage

### Dry Run Mode

Test what would happen without making changes:

```bash
./velocity-deploy install --binary ./app-radar-linux-arm64 --dry-run
```

### Skip Backup During Upgrade

Not recommended, but available:

```bash
./velocity-deploy upgrade --binary ./app-radar-linux-arm64 --no-backup
```

### Migrate Existing Database

During installation:

```bash
./velocity-deploy install \
  --binary ./app-radar-linux-arm64 \
  --db-path ./old-sensor_data.db
```

## Architecture

### File Locations

| Item | Path |
|------|------|
| Binary | `/usr/local/bin/velocity-report` |
| Data Directory | `/var/lib/velocity-report/` |
| Database | `/var/lib/velocity-report/sensor_data.db` |
| Service File | `/etc/systemd/system/velocity-report.service` |
| Backups | `/var/lib/velocity-report/backups/` |

### Service User

The service runs as user `velocity`:
- System user (no login shell)
- Owns `/var/lib/velocity-report/`
- Restricted permissions

### Backup Structure

Backups are timestamped and include:

```
velocity-report-backup-20250110-143022/
├── velocity-report          # Binary
├── sensor_data.db          # Database
├── velocity-report.service # Service file
└── README.txt              # Metadata
```

## Multi-Host Management

### Managing Multiple Installations

Create a script or configuration file for your deployments:

```bash
#!/bin/bash
# deploy-all.sh

HOSTS=(
  "pi@192.168.1.100"
  "pi@192.168.1.101"
  "pi@192.168.1.102"
)

for host in "${HOSTS[@]}"; do
  echo "Deploying to $host..."
  ./velocity-deploy upgrade \
    --target "$host" \
    --binary ./app-radar-linux-arm64
done
```

### Health Monitoring Script

```bash
#!/bin/bash
# check-all.sh

HOSTS=(
  "pi@192.168.1.100"
  "pi@192.168.1.101"
)

for host in "${HOSTS[@]}"; do
  echo "Checking $host..."
  ./velocity-deploy health --target "$host"
  echo ""
done
```

## Future Enhancements

Planned features:

- Configuration file for managing multiple sites
- Automatic version checking
- Prometheus metrics export
- Email notifications for health issues
- Web UI for deployment management

## Support

For issues or questions:

1. Check [TROUBLESHOOTING.md](../TROUBLESHOOTING.md)
2. Review logs: `sudo journalctl -u velocity-report.service`
3. Open an issue on GitHub
4. Join Discord: [velocity.report community](https://discord.gg/XXh6jXVFkt)

## See Also

- [cmd/deploy/README.md](../cmd/deploy/README.md) - Detailed CLI reference
- [ARCHITECTURE.md](../ARCHITECTURE.md) - System architecture
- [README.md](../README.md) - Main documentation
