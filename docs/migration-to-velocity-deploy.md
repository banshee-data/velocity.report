# Migration Guide: setup-radar-host.sh → velocity-deploy

This guide helps you migrate from the legacy `scripts/setup-radar-host.sh` installation script to the new `velocity-deploy` management tool.

## Why Migrate?

The new `velocity-deploy` tool provides:

- **Remote Deployment**: Deploy to multiple Raspberry Pi devices via SSH
- **Upgrade Management**: Safe upgrades with automatic backup and rollback
- **Health Monitoring**: Comprehensive health checks for service and API
- **Configuration Management**: Edit service configuration without manual file editing
- **Backup/Restore**: Create and restore backups on demand
- **Dry-Run Mode**: Test deployments without making changes

## Compatibility

✅ **Fully backward compatible**: Existing installations work with the new tool without any changes required.

The new tool:
- Uses the same installation paths
- Creates the same service user
- Installs the same systemd service
- Manages the same database location

## Quick Comparison

| Task | Old Method | New Method |
|------|-----------|------------|
| **Initial Install (Local)** | `sudo ./scripts/setup-radar-host.sh` | `./velocity-deploy install --binary ./app-radar-linux-arm64` |
| **Initial Install (Remote)** | SSH manually, clone repo, run script | `./velocity-deploy install --target pi@192.168.1.100 --binary ./app-radar-linux-arm64` |
| **Upgrade** | Stop service, copy binary, start service | `./velocity-deploy upgrade --binary ./app-radar-linux-arm64` |
| **Check Status** | `sudo systemctl status velocity-report` | `./velocity-deploy status` |
| **Health Check** | Manual checks | `./velocity-deploy health` |
| **Backup** | Manual copy | `./velocity-deploy backup` |
| **Rollback** | Not available | `./velocity-deploy rollback` |

## Migration Steps

### Step 1: Build the New Tool

```bash
cd /path/to/velocity.report
make build-deploy
```

This creates the `velocity-deploy` binary in your repository root.

### Step 2: Test with Dry Run (Optional)

Test the tool without making changes:

```bash
# Build a test binary first
make build-radar-linux

# Test installation (no changes made)
./velocity-deploy install --binary ./app-radar-linux-arm64 --dry-run
```

### Step 3: Manage Existing Installation

If you already have velocity.report installed via the old script:

```bash
# Check current status
./velocity-deploy status

# Run health check
./velocity-deploy health

# Create a backup
./velocity-deploy backup --output ./backups
```

### Step 4: Perform First Upgrade

```bash
# Build new version
make build-radar-linux

# Upgrade (creates backup automatically)
./velocity-deploy upgrade --binary ./app-radar-linux-arm64
```

### Step 5: Configure for Remote Hosts (Optional)

If managing remote Raspberry Pi devices:

```bash
# Set up SSH key (one-time)
ssh-copy-id pi@192.168.1.100

# Test connection
ssh pi@192.168.1.100 'sudo systemctl status velocity-report'

# Deploy or upgrade
./velocity-deploy upgrade \
  --target pi@192.168.1.100 \
  --ssh-key ~/.ssh/id_rsa \
  --binary ./app-radar-linux-arm64
```

## Common Migration Scenarios

### Scenario 1: Fresh Installation

**Before (Old Method):**
```bash
# On the Raspberry Pi
git clone https://github.com/banshee-data/velocity.report.git
cd velocity.report
make build-radar-linux
sudo ./scripts/setup-radar-host.sh
```

**After (New Method):**
```bash
# On your development machine
make build-radar-linux
make build-deploy

# Deploy to remote Pi
./velocity-deploy install \
  --target pi@192.168.1.100 \
  --ssh-key ~/.ssh/id_rsa \
  --binary ./app-radar-linux-arm64
```

### Scenario 2: Upgrading Existing Installation

**Before (Old Method):**
```bash
# On the Raspberry Pi
cd velocity.report
git pull
make build-radar-linux
sudo systemctl stop velocity-report
sudo cp app-radar-linux-arm64 /usr/local/bin/velocity-report
sudo systemctl start velocity-report
```

**After (New Method):**
```bash
# On your development machine
make build-radar-linux
./velocity-deploy upgrade \
  --target pi@192.168.1.100 \
  --binary ./app-radar-linux-arm64
```

### Scenario 3: Multiple Raspberry Pi Devices

**Before (Old Method):**
```bash
# Repeat for each device
ssh pi@192.168.1.100 'cd velocity.report && git pull && make build-radar-linux && sudo ./scripts/setup-radar-host.sh'
ssh pi@192.168.1.101 'cd velocity.report && git pull && make build-radar-linux && sudo ./scripts/setup-radar-host.sh'
ssh pi@192.168.1.102 'cd velocity.report && git pull && make build-radar-linux && sudo ./scripts/setup-radar-host.sh'
```

**After (New Method):**
```bash
# Create a simple script
cat > deploy-all.sh << 'EOF'
#!/bin/bash
HOSTS=(
  "pi@192.168.1.100"
  "pi@192.168.1.101"
  "pi@192.168.1.102"
)

for host in "${HOSTS[@]}"; do
  echo "Upgrading $host..."
  ./velocity-deploy upgrade --target "$host" --binary ./app-radar-linux-arm64
done
EOF

chmod +x deploy-all.sh
./deploy-all.sh
```

## Feature-by-Feature Migration

### Health Monitoring

**Before:**
```bash
# Manual checks
ssh pi@192.168.1.100 'sudo systemctl status velocity-report'
ssh pi@192.168.1.100 'sudo journalctl -u velocity-report -n 20'
ssh pi@192.168.1.100 'curl http://localhost:8080/api/config'
```

**After:**
```bash
# Single command
./velocity-deploy health --target pi@192.168.1.100
```

### Backup Creation

**Before:**
```bash
# Manual backup
ssh pi@192.168.1.100 'sudo cp /usr/local/bin/velocity-report ~/velocity-report.backup'
ssh pi@192.168.1.100 'sudo cp /var/lib/velocity-report/sensor_data.db ~/sensor_data.db.backup'
scp pi@192.168.1.100:~/velocity-report.backup ./
scp pi@192.168.1.100:~/sensor_data.db.backup ./
```

**After:**
```bash
# Automated backup
./velocity-deploy backup --target pi@192.168.1.100 --output ./backups
```

### Configuration Changes

**Before:**
```bash
# Edit service file manually
ssh pi@192.168.1.100 'sudo nano /etc/systemd/system/velocity-report.service'
ssh pi@192.168.1.100 'sudo systemctl daemon-reload'
ssh pi@192.168.1.100 'sudo systemctl restart velocity-report'
```

**After:**
```bash
# Interactive editor
./velocity-deploy config --edit --target pi@192.168.1.100
```

## Rollback Strategy

The new tool provides automatic rollback capability.

**Example: Upgrade with Rollback**

```bash
# Upgrade to new version
./velocity-deploy upgrade --target pi@192.168.1.100 --binary ./app-radar-linux-arm64

# If something goes wrong...
./velocity-deploy health --target pi@192.168.1.100
# Output: Service is UNHEALTHY: API endpoint not responding

# Rollback to previous version
./velocity-deploy rollback --target pi@192.168.1.100

# Verify
./velocity-deploy health --target pi@192.168.1.100
# Output: Service is HEALTHY
```

## Frequently Asked Questions

### Q: Do I need to reinstall if I used the old script?

**A:** No. The new tool works with existing installations without any changes.

### Q: Can I still use the old script?

**A:** Yes, `scripts/setup-radar-host.sh` is still available for local installations. However, we recommend migrating to `velocity-deploy` for new deployments.

### Q: Will the new tool work with my database?

**A:** Yes. Both tools use the same database location and format. Your data is safe.

### Q: What if something goes wrong during upgrade?

**A:** The upgrade process creates automatic backups. Use `velocity-deploy rollback` to restore the previous version.

### Q: Can I use both tools on the same system?

**A:** Yes, but we recommend using one consistently. The new tool provides more features and safety checks.

### Q: How do I migrate my deployment scripts?

**A:** Replace calls to `scripts/setup-radar-host.sh` with `velocity-deploy` commands. See examples above.

## Makefile Updates

The Makefile now includes new targets:

```bash
# Build targets
make build-deploy              # Build velocity-deploy tool
make build-deploy-linux        # Build for Linux ARM64

# Deployment targets
make deploy-install            # Install locally
make deploy-upgrade            # Upgrade local installation
make deploy-status             # Check status
make deploy-health             # Run health check

# Legacy (still available)
make setup-radar              # Old installation method
```

## Getting Help

- **Documentation**: [cmd/deploy/README.md](../cmd/deploy/README.md)
- **Deployment Guide**: [deployment-guide.md](deployment-guide.md)
- **Main README**: [README.md](../README.md)
- **Discord**: [velocity.report community](https://discord.gg/XXh6jXVFkt)

## Next Steps

1. Build the tool: `make build-deploy`
2. Test with dry-run: `./velocity-deploy install --dry-run --binary ./app-radar-linux-arm64`
3. Try status check: `./velocity-deploy status`
4. Create a backup: `./velocity-deploy backup`
5. Update your deployment scripts to use new tool

## Timeline

- **Now**: Both tools available, new tool recommended
- **Future**: Old script deprecated (but still functional)
- **No Breaking Changes**: Existing installations continue to work

Welcome to the improved deployment experience!
