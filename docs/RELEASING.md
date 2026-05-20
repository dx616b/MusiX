# Releasing and Docker images

For maintainers. End-user setup is in the [README](../README.md).

## Image tags

| Tag | When | Use |
|-----|------|-----|
| `X.Y.Z` | Git tag `vX.Y.Z` (must match `VERSION.txt`) | Production — pin in compose |
| `latest` | Same as last `v*` release | Convenience only |
| `edge` | After CI passes on `main` | Preview / bleeding edge |
| `sha-<git>` | Every edge and release build | Immutable build for one commit |

Prod should pin a semver tag (or `image@sha256:…` from the release workflow job summary), not `edge` or `latest`, unless you accept moving targets.

## Ship a release

1. Merge to `main`.
2. Bump [`VERSION.txt`](../VERSION.txt).
3. Tag and push:

```bash
git tag v0.2.2
git push origin v0.2.2
```

4. GitHub Actions publishes `dx616b/musix:0.2.2`, `latest`, and `sha-<commit>`.
5. On prod, update the pinned image and redeploy.

## Preview stack (optional)

Separate port and SQLite volume from prod:

```bash
cp docker-compose.preview.yml.example docker-compose.preview.yml
cp config/config.docker.yaml.example config/config.preview.yaml
docker compose -f docker-compose.preview.yml pull
docker compose -f docker-compose.preview.yml up -d
```

Default image: `dx616b/musix:edge` on **http://localhost:8081**.

## Workflows

- **CI** — PRs and pushes to `main`
- **Docker edge** — after green CI on `main`
- **Docker release** — on `v*` tags
