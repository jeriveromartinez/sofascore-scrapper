#!/bin/sh
# Test entrypoint: in BOOTSTRAP_ONLY mode, seed an invite token into
# /shared for tests that need to register the first admin. Otherwise
# exec the backend.
#
# The token is written to the shared volume only. It is never echoed to
# stdout/stderr, so it does not end up in the container logs.

set -e

if [ "$BOOTSTRAP_ONLY" = "true" ]; then
  attempt=1
  while [ "$attempt" -le 30 ]; do
    if /app/scores365-scrapper bootstrap-invitation > /shared/invite.txt; then
      exit 0
    fi
    echo "bootstrap-invitation failed (attempt ${attempt}/30), retrying" >&2
    rm -f /shared/invite.txt
    attempt=$((attempt + 1))
    sleep 2
  done
  echo "bootstrap-invitation failed after 30 attempts" >&2
  exit 1
fi

exec /app/scores365-scrapper
