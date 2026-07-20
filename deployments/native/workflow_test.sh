#!/usr/bin/env bash
set -euo pipefail

workflow=.github/workflows/deploy.yml
deploy_script=deployments/native/deploy.sh

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

require_deploy() {
  grep -Fq "$1" "$deploy_script" || {
    echo "missing deploy script requirement: $1" >&2
    exit 1
  }
}

require_immutable_actions() {
  local line
  local reference
  while IFS= read -r line; do
    reference=${line#*uses: }
    reference=${reference%% *}
    [[ "$reference" =~ @[0-9a-f]{40}$ ]] || {
      echo "mutable deploy workflow action: $reference" >&2
      exit 1
    }
  done < <(grep -E 'uses: [^[:space:]]+@' "$workflow")
}

require 'workflow_run:'
require 'workflows: [CI]'
require 'branches: [main]'
require "github.event.workflow_run.conclusion == 'success'"
require "github.event.workflow_run.event == 'push'"
require 'github.event.workflow_run.head_repository.full_name == github.repository'
require 'github.event.workflow_run.head_sha'
require 'actions: read'
require 'verify:'
require 'name: Verify deployment eligibility'
require 'runs-on: ubuntu-latest'
require 'sha: ${{ steps.revision.outputs.sha }}'
require 'GH_TOKEN: ${{ github.token }}'
require '[[ "$GITHUB_REF" == "refs/heads/main" ]]'
require 'Authorization: Bearer $GH_TOKEN'
require 'actions/workflows/ci.yml/runs'
require 'head_sha=$sha'
require 'branch=main'
require 'event=push'
require 'status=success'
require '.head_sha == $sha'
require '.status == "completed"'
require '.conclusion == "success"'
require 'git/ref/heads/main'
require '.object.type == "commit"'
require '.object.sha == $sha'
require 'needs: verify'
require "needs.verify.result == 'success'"
require 'ref: ${{ needs.verify.outputs.sha }}'
require 'runs-on: [self-hosted, iptv]'
require 'cancel-in-progress: false'
require 'for package in ca-certificates curl git python3; do'
require 'Run these commands on the production server:'
require 'sudo apt-get update'
require 'sudo apt-get install -y ${missing[*]}'
if grep -Fq 'sudo -n' "$workflow"; then
  echo 'the deployment workflow must not attempt non-interactive sudo' >&2
  exit 1
fi
require 'for command in curl git go node npm python3 systemctl; do'
require 'EXPECTED_SHA: ${{ needs.verify.outputs.sha }}'
require '[[ "$(git rev-parse HEAD)" == "$EXPECTED_SHA" ]]'
require 'git ls-remote --exit-code origin refs/heads/main'
require '[[ "$remote_main_sha" == "$EXPECTED_SHA" ]]'
require 'deployments/native/deploy.sh build/iptv web/dist "$EXPECTED_SHA"'
require_deploy 'expected_sha=$3'
require_deploy '"$git_bin" ls-remote --exit-code origin refs/heads/main'
require_deploy 'release_marker='
require_deploy '"$systemctl_bin" --user --no-block restart "$service_name"'
if grep -Fq 'dashboard_exchanged=' "$deploy_script"; then
  echo 'deploy script must observe dashboard publication from the filesystem' >&2
  exit 1
fi
[[ $(grep -Ec '^require_current_main$' "$deploy_script") -eq 2 ]] || {
  echo 'deploy script must guard immediately before mutation and after readiness' >&2
  exit 1
}
require 'systemctl --user show iptv.service'
require 'runtime_dir="/run/user/$(id -u)"'
require '[[ -S "$runtime_dir/bus" ]]'
require 'export XDG_RUNTIME_DIR="$runtime_dir"'
require 'echo "XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR" >> "$GITHUB_ENV"'
reject '/run/user/1001'
require 'id: runtime'
require 'environment_file=/etc/iptv/iptv.env'
require "grep -Eq '^JWT_SECRET=.+$' \"\$environment_file\""
require 'secrets.token_hex(32)'
require 'sudo tee -a /etc/iptv/iptv.env'
require 'health_port=${api_addr##*:}'
require 'echo "health_url=http://127.0.0.1:${health_port}/health/ready" >> "$GITHUB_OUTPUT"'
require 'HEALTH_URL: ${{ steps.runtime.outputs.health_url }}'
require 'uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4'
require 'uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5'
require 'uses: actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020 # v4'
require_immutable_actions
reject 'docker/'
reject 'CONTAINER_REGISTRY'
reject 'REGISTRY_PASSWORD'
reject 'ref: ${{ steps.revision.outputs.sha }}'
reject 'run: deployments/native/deploy.sh build/iptv web/dist'

echo 'deploy workflow contract passed'
