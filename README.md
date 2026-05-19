# MusiX

Music torrent acquisition: search **Prowlarr** and **Jackett**, queue downloads in **Transmission**, track jobs in **SQLite**. No Postgres. No Navidrome integration — point Navidrome at Transmission’s download folder yourself.

**Repository:** [github.com/dx616b/MusiX](https://github.com/dx616b/MusiX)

```bash
git clone git@github.com:dx616b/MusiX.git
cd MusiX
```

## Stack

| Component | Role |
|-----------|------|
| MusiX (Go + React) | Search UI, download queue, SQLite job history |
| Prowlarr / Jackett | Music indexer search |
| Transmission | Downloads to disk |
| Navidrome (your setup) | Scans the download directory independently |

## Quick start (local)

```bash
cp config/config.yaml.example config/config.yaml
# Match StreamX-style URLs/keys, or local :9696 / :9117 / :9091

go run ./cmd/server

cd web && npm install && npm run dev
```

- API: http://localhost:8080  
- UI (dev): http://localhost:5175  

## Config

See `config/config.yaml.example`. You can also edit Prowlarr, Jackett, and Transmission from the **Settings** page in the UI; saves go to `data/settings.yaml` (or `SETTINGS_FILE`) and apply without restart.

Environment overrides (applied after the YAML files): `PROWLARR_URL`, `PROWLARR_API_KEY`, `JACKETT_URL`, `JACKETT_API_KEY`, `TRANSMISSION_URL`, `TRANSMISSION_USER`, `TRANSMISSION_PASS`, `MUSIX_SQLITE` (or legacy `MUSICX_SQLITE`), `SETTINGS_FILE`.

Each indexer `url` must use the **apiKey** from that same host.

## CI

Pull requests run [GitHub Actions](.github/workflows/ci.yml): Go lint/test/build (`CGO_ENABLED=0`), web `npm ci` + build, and Dockerfile Hadolint. Merges to `main` also run [docker-publish](.github/workflows/docker-publish.yml) (`dx616b/musix` on Docker Hub; requires `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` repo secrets).

## Docker

### Docker (external Prowlarr / Jackett / Transmission)

```bash
cp config/config.docker.yaml.example config/config.docker.yaml
# Edit URLs and API keys for your indexers and Transmission
cp .env.example .env   # optional: override secrets only

docker compose up --build -d
```

App: http://localhost:8080

### Standalone (bundled Prowlarr + Jackett + Transmission)

```bash
cp config/config.bundled.yaml.example config/config.docker.yaml
docker compose -f docker-compose.yml -f docker-compose.bundled.yml up --build -d
```

Mount `./downloads` to the same path Navidrome scans.

## API

- `GET /api/search?q=artist+album`
- `GET /api/searches` — recent search history (`?limit=50`, optional `?includeResults=true`)
- `GET /api/searches?q=artist` — one saved search with stored results
- `GET /api/downloads`
- `POST /api/downloads` `{ "title", "magnet", "query?", "infoHash?", "indexer?" }`
- `GET /api/health`
- `GET /api/torrent/preview?magnet=…&title=…` — file list via anacrolix DHT/metadata (optional `infoHash`)
- `GET /api/torrent/stream?path=…&magnet=…` — stream one file (HTTP Range, for in-browser play)

Env: `TORRENT_MAGNET_METADATA_TIMEOUT_SECS` (default 90), `TORRENT_MAGNET_METADATA_DISABLED=1` to turn off.
