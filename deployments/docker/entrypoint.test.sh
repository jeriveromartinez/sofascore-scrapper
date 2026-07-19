#!/bin/sh
set -e

if [ "$BOOTSTRAP_ONLY" = "true" ]; then
  /app/sofascore-scrapper bootstrap-invitation > /shared/invite.txt 2>/dev/null
  echo "token: $(cat /shared/invite.txt)"
  exit 0
fi

exec /app/sofascore-scrapper
