#!/bin/sh
# Test entrypoint: in BOOTSTRAP_ONLY mode, seed an invite token into
# /shared for tests that need to register the first admin. Otherwise
# exec the backend.

set -e

if [ "$BOOTSTRAP_ONLY" = "true" ]; then
  /app/sofascore-scrapper bootstrap-invitation > /shared/invite.txt 2>/dev/null
  echo "token: $(cat /shared/invite.txt)"
  exit 0
fi

exec /app/sofascore-scrapper