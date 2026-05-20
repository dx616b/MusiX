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

Pre-built release image: `docker pull dx616b/musix:0.2.1` (pin a version on prod; avoid `:latest` there)

Preview images: `dx616b/musix:edge` (each merge to `main`), `dx616b/musix:nightly` (daily cron). See [Release channels](#release-channels).

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

Torrent preview/stream: `TORRENT_MAGNET_METADATA_TIMEOUT_SECS` (default 90), `TORRENT_MAGNET_METADATA_DISABLED=1` to disable. Session RAM: `TORRENT_SESSION_MAX` (default 2, evicts oldest idle first), `TORRENT_SESSION_LEAK_TTL_MINUTES` (default 5).

## Release channels

| Tag | When | Use |
|-----|------|-----|
| `0.2.1`, … | Git tag `v0.2.1` on `main` | **Production** — pin this in compose |
| `latest` | Same as last release tag | Convenience only; do not auto-deploy prod |
| `edge`, `sha-<git>` | Every push to `main` | Immediate preview |
| `nightly`, `nightly-YYYY-MM-DD` | Daily 03:00 UTC on `main` | Stable preview without prod churn |

Prod stays untouched until you change the pinned image tag in your compose file.

### Ship a release (prod)

1. Merge changes to `main` (CI on PRs; `edge` image updates automatically).
2. Bump [`VERSION.txt`](VERSION.txt) on `main` (e.g. `0.2.2`).
3. Commit, tag, and push:

```bash
git tag v0.2.2
git push origin v0.2.2
```

4. GitHub Actions publishes `dx616b/musix:0.2.2` and `:latest`.
5. On the prod host, set `image: dx616b/musix:0.2.2` and `docker compose pull && docker compose up -d`.

### Run a preview stack

```bash
cp docker-compose.preview.yml.example docker-compose.preview.yml
cp config/config.docker.yaml.example config/config.preview.yaml
# optional: use image: dx616b/musix:edge for bleeding-edge
docker compose -f docker-compose.preview.yml pull
docker compose -f docker-compose.preview.yml up -d
```

Open **http://localhost:8081** (separate SQLite volume from prod).


<img width="1904" height="976" alt="musix" src="https://github.com/user-attachments/assets/fff9cdfa-f36c-497f-836b-a875886cc2c8" />
