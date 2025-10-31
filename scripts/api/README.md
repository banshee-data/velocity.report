# Local API helper scripts

Small shell helpers for exercising the local monitor API used during PCAP replay debugging.

Usage examples:

```bash
# start a PCAP replay (absolute path required)
./start_pcap.sh /Users/david/code/sensor_data/lidar/break-80k.pcapng

# fetch recent snapshots
./get_snapshots.sh

# fetch grid status
./get_grid_status.sh
```

These scripts assume the local monitor runs on http://127.0.0.1:8081 and require `jq` for pretty JSON output.
