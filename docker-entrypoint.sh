#!/bin/sh
set -eu

mkdir -p /data/Downloads
chown -R music:music /data

exec gosu music /app/music-lib-web "$@"
