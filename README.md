<p align="center">
  <img src="frontend/public/gator128px.svg" alt="Gator logo" width="128" height="128" />
</p>

# Gator

A self-hosted web app for managing VPN routing, site-to-site WireGuard tunnels, and firewall rules on OPNsense.

Website: https://usegator.dev

Gator gives you a clean dashboard to deploy WireGuard VPN configs, set up per-protocol/per-service selective routing (e.g. route only Netflix or gaming traffic through your VPN), manage site-to-site tunnels to remote VPS nodes, and keep track of what's happening on your firewall -- all through a browser UI instead of clicking through OPNsense menus.

Some documentation is still missing while the project is moving quickly. More setup, deployment, and operational guides will be added soon.

## Features

- **VPN Management** -- Import WireGuard configs (e.g. from Mullvad), deploy to OPNsense with one click
- **Selective Routing** -- Route specific services (Netflix, YouTube, gaming, etc.) through VPN while keeping everything else on your normal connection
- **Site-to-Site Tunnels** -- Create and manage WireGuard tunnels between your OPNsense box and remote Linux servers (e.g. Hetzner VPS) with automated SSH setup
- **Live Dashboard** -- Real-time system stats, gateway health, WireGuard peer status, and resource usage via Server-Sent Events
- **Firewall Rule Management** -- View, create, and clean up filter rules and NAT rules
- **Migration Assistant** -- Adopt and convert legacy OPNsense rules into Gator-managed rules
- **Reconciler** -- Background drift detection that flags when firewall state doesn't match what Gator expects
- **Backup/Restore** -- Download and manage OPNsense configuration backups
- **Multi-Instance** -- Manage multiple OPNsense boxes from a single Gator install

## Prerequisites

- **Go 1.25+**
- **Node.js 20+** (for building the frontend)
- **OPNsense** firewall with API access enabled

## Quick Start

```bash
# Clone
git clone https://github.com/anothaDev/gator.git
cd gator/app

# Build frontend
cd frontend && npm install && npm run build && cd ..

# Run
go run main.go
```

Open `http://localhost:8080` and connect your OPNsense instance.

## Build

### Standard build

For local builds that serve the frontend from `frontend/dist` on disk:

```bash
cd frontend && npm install && npm run build && cd ..
go build -o gator .
./gator
```

### Single binary release

For end users, Gator can be shipped as a single Go binary with the built frontend embedded inside it:

```bash
cd frontend && npm install && npm run build && cd ..
go build -tags release -o gator .
./gator
```

That release binary serves the frontend directly from the executable, so users do not need Node.js or a separate frontend process at runtime.

### Docker

Gator also ships cleanly as a single-container deployment.

```bash
docker compose up -d
```

The default container port is `8080`, and persistent app data is stored in the `gator-data` volume.

To run Gator on a custom port, set `GATOR_PORT` before starting the container:

```bash
GATOR_PORT=9090 docker compose up --build -d
```

If you need absolute callback URLs for routing features, also set `GATOR_URL`:

```bash
GATOR_PORT=9090 GATOR_URL=http://192.168.1.50:9090 docker compose up -d
```

The default `compose.yaml` pulls the prebuilt image from `ghcr.io/anothadev/gator:latest`, which is published automatically from GitHub Actions.

If you want to build the Docker image from source yourself, build the frontend first so `frontend/dist` exists, then run:

```bash
cd frontend && npm install && npm run build && cd ..
docker build -t gator:local .
docker run --rm -p 8080:8080 -v gator-data:/data gator:local
```

## Development

Gator supports two development flows:

### Air only

If you already have a built frontend in `frontend/dist`, you can run the app entirely through Go with Air:

```bash
air
```

That serves the frontend from Go on `http://localhost:8080` and hot-reloads backend changes.

### Air + Vite

If you want frontend hot-reload while editing the UI, run the Go backend and Vite dev server separately:

```bash
# Terminal 1: Go backend with Air hot-reload
air

# Terminal 2: Frontend dev server (proxies API to :8080)
cd frontend && npm run dev
```

The frontend dev server runs on `http://localhost:3000` and proxies `/api/*` requests to the Go backend on `:8080`.

Default builds still serve the frontend from `frontend/dist` on disk, while `-tags release` builds use the embedded assets.

On GitHub, container builds are automated too:

- `CI` validates backend tests, frontend build, and embedded release builds
- `Container` builds the Docker image on pull requests and automatically publishes it to `ghcr.io/anothadev/gator` on pushes to `main` and version tags

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server listen port |
| `DATABASE_PATH` | `data/gator.db` | SQLite database file path |
| `GATOR_URL` | auto-detected | Base URL for callbacks (e.g. `http://192.168.1.50:8080`) |

## Project Structure

```
app/
  main.go                    # Entry point, route registration, background jobs
  internal/
    handlers/                # HTTP handlers (OPNsense API, VPN, tunnels, routing)
    models/                  # Data types
    storage/                 # SQLite persistence layer
    sshclient/               # SSH client for tunnel management
    routes/                  # Route registration
  frontend/
    src/
      pages/                 # SolidJS page components
      components/            # Shared UI components
      lib/                   # API client, utilities
```

## How It Works

Gator talks to your OPNsense firewall through its REST API. It creates and manages WireGuard peers, servers, gateways, firewall filter rules, NAT rules, and aliases on your behalf. All state is tracked in a local SQLite database so Gator knows which resources it owns and can detect drift.

For site-to-site tunnels, Gator also SSHs into remote Linux servers to configure the other end of the WireGuard tunnel.

## Security Note

Gator is designed to run on your **local network** alongside your firewall. It now includes a built-in admin password and session-based auth middleware, and all management routes require authentication after initial setup.

That said, it is still best treated as a trusted-network tool:

- On first run, setup and auth bootstrap are intentionally open so the first admin password can be created.
- Gator currently has a single local admin account, not a full multi-user or internet-hardened auth system.
- If you deploy it with Docker or behind a reverse proxy, set the admin password immediately and avoid exposing it directly to the public internet unless you add your own outer access controls.

## License

This project is licensed under the GNU General Public License v3.0. See [LICENSE](LICENSE) for details.
