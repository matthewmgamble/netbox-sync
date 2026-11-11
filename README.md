# NetBox Sync - Juniper Mist Integration

Synchronize Juniper Mist inventory and configuration data into NetBox Community Edition.

## Features

- **One-way synchronization** from Mist → NetBox
- **Smart site mapping** with Canadian province auto-detection
- **Idempotent operations** - safe to run repeatedly
- **Change detection** using checksums
- **Comprehensive logging** with structured JSON output
- **Docker-ready** for easy deployment
- **File-based state tracking** for sync history

## Current Status: Sprint 1-2 Complete

✅ Framework and authentication
✅ Site synchronization with mapping rules
✅ Device type and manufacturer synchronization
✅ Device synchronization
✅ Region and site group hierarchy management
✅ Structured logging and state tracking

## Quick Start

### Prerequisites

- Go 1.21+ (for local development)
- Docker and Docker Compose (for containerized deployment)
- Mist API token and organization ID
- NetBox instance with API access

### Configuration

1. Copy example configs:
```bash
cp configs/config.example.yaml configs/config.yaml
cp configs/mapping.example.yaml configs/mapping.yaml
```

2. Edit `configs/config.yaml`:
```yaml
mist:
  api_token: ${MIST_API_TOKEN}
  organization_id: "your-org-id"

netbox:
  url: https://netbox.example.com
  token: ${NETBOX_API_TOKEN}
```

3. Edit `configs/mapping.yaml` to configure site→region/site group mappings

4. Set environment variables:
```bash
export MIST_API_TOKEN="your-mist-token"
export NETBOX_API_TOKEN="your-netbox-token"
```

### Run Locally

```bash
# Build
go build -o netbox-sync ./cmd/sync

# Run
./netbox-sync -config ./configs/config.yaml
```

### Run with Docker

```bash
# Build image
docker build -t netbox-sync .

# Run once
docker run --rm \
  -v $(pwd)/configs:/config:ro \
  -v $(pwd)/data:/data \
  -e MIST_API_TOKEN="${MIST_API_TOKEN}" \
  -e NETBOX_API_TOKEN="${NETBOX_API_TOKEN}" \
  netbox-sync
```

### Run with Docker Compose

```bash
# Run once
docker-compose run --rm netbox-sync

# Schedule with cron (add to crontab)
0 */6 * * * cd /path/to/netbox-sync && docker-compose run --rm netbox-sync
```

## Site Mapping

The mapping system handles the complex task of organizing Mist sites into NetBox's region and site group hierarchies.

### Mapping Priority

1. **Explicit mappings** in `mapping.yaml` (highest priority)
2. **Pattern rules** matching address/name
3. **Default mappings** (fallback)

### Example Mapping Rules

```yaml
# Auto-detect Canadian provinces
rules:
  - pattern: ".*(ON|Ontario).*"
    region: "Canada/Ontario"
    site_group: "Canadian Sites/Central"

  - pattern: ".*(BC|British Columbia).*"
    region: "Canada/British Columbia"
    site_group: "Canadian Sites/Western"

# Explicit site mappings
sites:
  "mist-site-uuid-abc123":
    netbox_site_name: "Toronto HQ"
    region: "Canada/Ontario"
    site_group: "Headquarters"
    status: "active"
```

See `configs/mapping.example.yaml` for the complete Canadian province configuration.

## Architecture

```
┌─────────────────┐          ┌──────────────────┐
│  Juniper Mist   │          │  NetBox          │
│  Cloud API      │          │  Community       │
└────────┬────────┘          └────────▲─────────┘
         │                            │
         │ HTTPS                      │ HTTPS
         │                            │
         │         ┌──────────────────┴────────┐
         │         │  NetBox Sync Engine       │
         └────────►│                            │
                   │  • Discover from Mist     │
                   │  • Normalize data         │
                   │  • Apply mappings         │
                   │  • Compare checksums      │
                   │  • Sync to NetBox         │
                   └───────────────────────────┘
```

### Sync Process

1. **Discover** - Fetch objects from Mist API
2. **Normalize** - Convert to internal data models
3. **Apply Mapping** - Resolve regions/site groups
4. **Compare** - Check checksums for changes
5. **Apply** - Create/update only changed objects

## State Tracking

Sync state is persisted in `data/sync-state.json`:

```json
{
  "last_sync": "2026-09-26T10:30:00Z",
  "status": "completed",
  "duration_seconds": 45,
  "objects": {
    "sites": {
      "created": 2,
      "updated": 3,
      "unchanged": 10
    }
  },
  "tracked_objects": {
    "sites": {
      "mist-site-1": {
        "netbox_id": 42,
        "checksum": "sha256:abc123..."
      }
    }
  }
}
```

## Logging

Structured JSON logging to stdout (configurable in config.yaml):

```json
{
  "timestamp": "2026-09-26T10:30:00Z",
  "level": "info",
  "message": "Created site",
  "site": "Toronto HQ"
}
```

Levels: `debug`, `info`, `warn`, `error`

## Development

### Project Structure

```
netbox-sync/
├── cmd/sync/              # Main application
├── internal/
│   ├── client/            # API clients (Mist, NetBox)
│   ├── config/            # Configuration management
│   ├── models/            # Internal data models
│   ├── sync/              # Sync engine
│   ├── state/             # State tracking
│   └── logger/            # Logging
├── configs/               # Example configurations
├── Dockerfile
└── docker-compose.yml
```

### Testing

```bash
# Run tests
go test ./...

# Run with verbose output
go test -v ./...

# Test against live APIs (requires config)
go test -tags=integration ./...
```

### Build

```bash
# Local build
go build -o netbox-sync ./cmd/sync

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o netbox-sync-linux ./cmd/sync

# Cross-compile for Windows

GOOS=windows GOARCH=amd64 go build -o netbox-sync.exe ./cmd/sync

$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o netbox-sync.exe ./cmd/sync  

# Build Docker image
docker build -t netbox-sync:latest .
```

## Configuration Reference

### config.yaml

See `configs/config.example.yaml` for complete documentation.

Key settings:
- `mist.api_token` - Mist API token
- `mist.organization_id` - Mist organization UUID
- `netbox.url` - NetBox instance URL
- `netbox.token` - NetBox API token
- `sync.enabled.*` - Control what gets synced
- `filter.exclude_sites` - Sites to skip
- `logging.level` - Log verbosity

### mapping.yaml

See `configs/mapping.example.yaml` for complete documentation.

Key sections:
- `defaults` - Fallback region/site group
- `rules` - Pattern-based auto-mapping
- `sites` - Explicit site mappings
- `regions` - Region hierarchy definition
- `site_groups` - Site group hierarchy
- `custom_fields` - Custom field definitions

## Troubleshooting

### Connection Issues

```bash
# Test Mist API connectivity
curl -H "Authorization: Token YOUR_MIST_TOKEN" \
  https://api.mist.com/api/v1/orgs/YOUR_ORG_ID

# Test NetBox API connectivity
curl -H "Authorization: Token YOUR_NETBOX_TOKEN" \
  https://your-netbox/api/status/
```

### Mapping Issues

Enable debug logging in `config.yaml`:
```yaml
logging:
  level: debug
  log_api_calls: true
```

Check which mapping rule matched:
```bash
./netbox-sync -config ./configs/config.yaml 2>&1 | grep "apply_mapping"
```

### State Reset

To force a full re-sync:
```bash
rm data/sync-state.json
./netbox-sync -config ./configs/config.yaml
```

## Roadmap

### Sprint 3-4 (Future)
- Interface synchronization
- IP address synchronization
- Virtual chassis support

### Sprint 5 (Future)
- VLAN synchronization
- Wireless network synchronization

### Sprint 6-8 (Future)
- Incremental sync optimization
- Deletion handling
- Web UI for mapping management
- Performance tuning
- Comprehensive testing

### Phase 4 (Future)
- Additional vendor support (Meraki, Palo Alto, etc.)
- Generic sync framework
- Plugin architecture

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

For issues and questions:
- GitHub Issues: https://github.com/matthewmgamble/netbox-sync/issues
