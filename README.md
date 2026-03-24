# awoslog-stratux

Push live ADS-B aircraft data from a [Stratux](https://github.com/b3nn0/stratux) receiver to [awoslog.com](http://awoslog.com), where they appear as red aircraft on the map in real time.

## What It Does

`stratux-pusher` is a small Go binary that runs on the Stratux Raspberry Pi. It reads the SBS/BaseStation data stream from the Stratux receiver, aggregates per-aircraft state, and POSTs a JSON snapshot to the awoslog.com server every 3 seconds.

```
Stratux ADS-B Receiver
    │  SBS data on port 30003
    ▼
stratux-pusher (this binary)
    │  Parses SBS, aggregates state, POSTs every 3s
    ▼
awoslog.com
    │  Displays red aircraft on the map via SSE
    ▼
Your browser — red icons = your local ADS-B
```

Aircraft received by your Stratux appear in **red** on the awoslog.com map, distinct from the blue/gray aircraft sourced from the global ADS-B network. This lets you see:

- Aircraft with blocked registrations (LADD/PIA) that don't appear in public APIs
- Low-altitude traffic below network coverage
- Any aircraft in your local airspace, unfiltered and undelayed

## Requirements

- A [Stratux](https://github.com/b3nn0/stratux) ADS-B receiver (any version with SBS output on port 30003)
- Go 1.22+ on your build machine (for cross-compilation)
- SSH access to the Stratux Raspberry Pi
- An awoslog.com server instance (public or self-hosted)

## Quick Start

```bash
git clone https://github.com/dgallant0x007/awoslog-stratux.git
cd awoslog-stratux
./deploy.sh 192.168.0.119 http://your-awoslog-server:8080 stratux-home
```

That single command builds the ARM64 binary, copies it to the Pi, installs a systemd service, and starts it. Red aircraft should appear on the awoslog.com map within seconds.

## Building

```bash
./build.sh
```

This cross-compiles a static ARM64 binary (`stratux-pusher`) for the Raspberry Pi. No CGO, no external dependencies.

Or manually:

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o stratux-pusher .
```

## Deploying

```bash
./deploy.sh <pi-host> [awoslog-server] [source-name]
```

| Argument | Default | Description |
|----------|---------|-------------|
| `pi-host` | (required) | SSH target — IP or `user@host` |
| `awoslog-server` | `http://awoslog.com` | URL of the awoslog server |
| `source-name` | `stratux-home` | Unique identifier for this receiver |

### Examples

```bash
# Local network
./deploy.sh 192.168.0.119 http://192.168.0.107:8080 stratux-home

# Public server
./deploy.sh pi@10.0.0.50 https://awoslog.com my-remote-stratux

# Defaults (awoslog.com, source "stratux-home")
./deploy.sh 192.168.0.119
```

The deploy script:
1. Cross-compiles the binary (if not already built)
2. Copies it to the Pi via scp
3. Installs to `/usr/local/bin/stratux-pusher`
4. Creates and enables a systemd service
5. Starts the service and verifies it's running

## Managing the Service

On the Stratux Pi:

```bash
sudo systemctl status stratux-pusher      # Check status
sudo journalctl -u stratux-pusher -f      # Follow logs
sudo systemctl restart stratux-pusher     # Restart
sudo systemctl stop stratux-pusher        # Stop
```

The service starts automatically on boot and restarts on failure.

## Uninstalling

```bash
ssh pi@192.168.0.119 'sudo systemctl stop stratux-pusher && \
  sudo systemctl disable stratux-pusher && \
  sudo rm /etc/systemd/system/stratux-pusher.service /usr/local/bin/stratux-pusher && \
  sudo systemctl daemon-reload'
```

## Command-Line Flags

If running manually instead of via the systemd service:

```bash
stratux-pusher [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-sbs` | `localhost:30003` | SBS host:port to connect to |
| `-server` | `http://awoslog.com` | awoslog server URL |
| `-source` | `stratux-home` | Source identifier for this receiver |
| `-interval` | `3s` | How often to push aircraft state |
| `-key` | (empty) | Optional API key (if server requires it) |

## API

The pusher sends a POST request to the awoslog server:

### POST /api/stratux/push

```json
{
    "source_id": "stratux-home",
    "aircraft": [
        {
            "hex": "A12345",
            "callsign": "UAL2411",
            "lat": 38.393,
            "lon": -108.248,
            "altitude": 34000,
            "speed": 416,
            "heading": 235.0,
            "vertical_rate": -500
        }
    ]
}
```

Aircraft with no position (Mode S only — hex and altitude but no lat/lon) are included so the server can cross-reference them against the global ADS-B network.

**Response:** `{"status":"ok","accepted":5}`

If the server has `AWOSLOG_STRATUX_KEY` set, include the key in the `X-Stratux-Key` header.

## How the SBS Parser Works

The Stratux receiver outputs SBS/BaseStation format on TCP port 30003 — one CSV line per message. The pusher reads three message types:

| Type | Data | Fields |
|------|------|--------|
| MSG,1 | Identification | Callsign |
| MSG,3 | Airborne position | Latitude, longitude, altitude |
| MSG,4 | Airborne velocity | Ground speed, heading, vertical rate |

Messages are aggregated per aircraft (keyed by ICAO hex code) to build a complete picture from the individual message fragments. Aircraft not seen for 60 seconds are removed.

## Multiple Receivers

Multiple Stratux devices can push to the same awoslog server simultaneously. Give each a unique `source-name` and the server merges them, deduplicating by hex code.

```bash
./deploy.sh 192.168.0.119 https://awoslog.com stratux-home
./deploy.sh 192.168.0.120 https://awoslog.com stratux-hangar
./deploy.sh 192.168.0.121 https://awoslog.com stratux-south-field
```

## License

MIT
