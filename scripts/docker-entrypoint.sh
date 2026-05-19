#!/bin/sh
set -e
# Named volumes mount over /app/data as root:root; the app runs as musix (uid 1000).
mkdir -p /app/data /app/downloads
chown -R musix:musix /app/data /app/downloads
exec su-exec musix "$@"
