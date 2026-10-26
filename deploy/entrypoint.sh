#!/bin/sh
# Runs lobby directly, or under Litestream when LITESTREAM_REPLICA_URL is set:
# restore the database on first boot if a replica exists, then replicate
# continuously while lobby runs as Litestream's child process.
set -e

if [ -n "$LITESTREAM_REPLICA_URL" ]; then
  litestream restore -config /etc/litestream.yml -if-db-not-exists -if-replica-exists /data/lobby.db
  exec litestream replicate -config /etc/litestream.yml -exec "lobby serve"
fi

exec lobby serve
