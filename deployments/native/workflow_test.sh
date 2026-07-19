#!/usr/bin/env bash
set -euo pipefail

workflow=.github/workflows/deploy.yml

require() {
  grep -Fq "$1" "$workflow" || {
    echo "missing deploy workflow requirement: $1" >&2
    exit 1
  }
}

reject() {
  if grep -Fq "$1" "$workflow"; then
    echo "forbidden deploy workflow content: $1" >&2
    exit 1
  fi
}

require 'workflow_run:'
require 'workflows: [CI]'
require 'branches: [main]'
require "github.event.workflow_run.conclusion == 'success'"
require 'github.event.workflow_run.head_sha'
require 'runs-on: [self-hosted, iptv]'
require 'cancel-in-progress: false'
require 'deployments/native/deploy.sh build/iptv web/dist'
require 'systemctl --user show iptv.service'
reject 'docker/'
reject 'CONTAINER_REGISTRY'
reject 'REGISTRY_PASSWORD'

echo 'deploy workflow contract passed'
