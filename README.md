# MusiX

Search music torrents with **Prowlarr** and **Jackett**, queue downloads in **Transmission**, and track jobs in **SQLite**.

## Run with Docker

```bash
git clone git@github.com:dx616b/MusiX.git
cd MusiX

cp config/config.docker.yaml.example config/config.docker.yaml
```

Edit `config/config.docker.yaml` with your Prowlarr, Jackett, and Transmission URLs and API keys (or set them later under **Settings** in the app).

```bash
docker compose up --build -d
```

Open **http://localhost:8080**

Pre-built image: `docker pull dx616b/musix:latest`

If logs show `unable to open database file` for `/app/data/musicx.db`, fix data volume ownership then restart:

```bash
docker compose run --rm --user root --entrypoint sh musix -c "chown -R musix:musix /app/data /app/downloads"
docker compose up -d --build
```

### All-in-one (optional)

Runs MusiX plus local Prowlarr, Jackett, and Transmission:

```bash
cp config/config.bundled.yaml.example config/config.docker.yaml
docker compose -f docker-compose.yml -f docker-compose.bundled.yml up --build -d
```

Set API keys from the Prowlarr/Jackett UIs on ports 9696 and 9117, or via `.env` (see `.env.example`).

## Run locally

```bash
cp config/config.yaml.example config/config.yaml
# Edit Prowlarr, Jackett, and Transmission

cd web && npm install && npm run build && cd ..

CGO_ENABLED=0 go run ./cmd/server
```

Open **http://localhost:8080**

## Configuration

| Where | Purpose |
|-------|---------|
| `config/config.yaml` | Base config (local) |
| `config/config.docker.yaml` | Base config (Docker, gitignored) |
| **Settings** in the UI | Overrides saved to `data/settings.yaml` |

Optional env vars (override YAML): `PROWLARR_URL`, `PROWLARR_API_KEY`, `JACKETT_URL`, `JACKETT_API_KEY`, `TRANSMISSION_URL`, `TRANSMISSION_USER`, `TRANSMISSION_PASS`, `MUSIX_SQLITE`, `SETTINGS_FILE`.

Torrent preview/stream: `TORRENT_MAGNET_METADATA_TIMEOUT_SECS` (default 90), `TORRENT_MAGNET_METADATA_DISABLED=1` to disable.

<img width="1904" height="976" alt="musix" src="https://github.com/user-attachments/assets/fff9cdfa-f36c-497f-836b-a875886cc2c8" />
