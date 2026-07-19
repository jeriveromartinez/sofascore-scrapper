#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <executable> <web-dist>" >&2
  exit 2
fi

source_binary=$1
source_dashboard=$2
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
deploy_root=${DEPLOY_ROOT:-/opt/iptv}
service_name=${SERVICE_NAME:-iptv.service}
health_url=${HEALTH_URL:-http://127.0.0.1:8080/health/ready}
systemctl_bin=${SYSTEMCTL_BIN:-systemctl}
curl_bin=${CURL_BIN:-curl}
python_bin=${PYTHON_BIN:-python3}
health_attempts=${HEALTH_ATTEMPTS:-30}
health_delay=${HEALTH_DELAY_SECONDS:-2}

state_dir="$deploy_root/.deploy"
stage_dir="$state_dir/stage"
previous_dir="$state_dir/previous"
current_binary="$deploy_root/iptv"
current_dashboard="$deploy_root/web/dist"
atomic_exchange="$script_dir/atomic_exchange.py"
rollback_required=false
dashboard_exchanged=false

cleanup() {
  rm -rf "$stage_dir" "$state_dir/previous.new"
  rm -f "$deploy_root/.iptv.new" "$deploy_root/.iptv.rollback"
  if [[ "$dashboard_exchanged" == false ]]; then
    rm -rf "$deploy_root/web/.dist.new"
  fi
  rm -rf "$deploy_root/web/.dist.rollback"
}

wait_for_health() {
  local attempt
  local http_status
  for ((attempt = 1; attempt <= health_attempts; attempt++)); do
    if "$systemctl_bin" --user is-active --quiet "$service_name" \
      && http_status=$("$curl_bin" --fail --silent --show-error --max-time 5 \
        --output /dev/null --write-out '%{http_code}' "$health_url") \
      && [[ "$http_status" == 200 ]]; then
      return 0
    fi
    sleep "$health_delay"
  done

  "$systemctl_bin" --user status "$service_name" --no-pager || true
  return 1
}

restore_previous() {
  [[ -f "$previous_dir/iptv" ]] || return 1
  [[ -d "$previous_dir/web-dist" ]] || return 1

  if [[ "$dashboard_exchanged" == true ]]; then
    "$python_bin" "$atomic_exchange" "$current_dashboard" "$deploy_root/web/.dist.new" || return
    dashboard_exchanged=false
  fi
  install -m 0755 "$previous_dir/iptv" "$deploy_root/.iptv.rollback" || return
  mv -f "$deploy_root/.iptv.rollback" "$current_binary" || return
  "$systemctl_bin" --user restart "$service_name" || return
  wait_for_health || return
}

handle_error() {
  local status=$?
  trap - ERR

  if [[ "$rollback_required" == true ]]; then
    echo "deployment failed; restoring previous release" >&2
    if restore_previous; then
      echo "previous release restored" >&2
    else
      echo "rollback failed; inspect $service_name immediately" >&2
    fi
  fi

  exit "$status"
}

trap cleanup EXIT
trap handle_error ERR

[[ -f "$source_binary" ]] || { echo "missing executable: $source_binary" >&2; exit 1; }
[[ -d "$source_dashboard" ]] || { echo "missing dashboard: $source_dashboard" >&2; exit 1; }
[[ -f "$source_dashboard/index.html" ]] || { echo "dashboard index is missing" >&2; exit 1; }
[[ -f "$atomic_exchange" ]] || { echo "atomic exchange helper is missing: $atomic_exchange" >&2; exit 1; }
command -v "$python_bin" >/dev/null || { echo "missing required command: $python_bin" >&2; exit 1; }
[[ -d "$deploy_root" && -w "$deploy_root" ]] || { echo "$deploy_root is not writable" >&2; exit 1; }
[[ -d "$deploy_root/apk_storage" && -w "$deploy_root/apk_storage" ]] || { echo "apk storage is not writable" >&2; exit 1; }
[[ -d "$deploy_root/image_storage" && -w "$deploy_root/image_storage" ]] || { echo "image storage is not writable" >&2; exit 1; }
[[ -f "$current_binary" ]] || { echo "current executable is missing" >&2; exit 1; }
[[ -d "$current_dashboard" ]] || { echo "current dashboard is missing" >&2; exit 1; }

mkdir -p "$state_dir" "$deploy_root/web"
rm -rf "$stage_dir" "$state_dir/previous.new"
mkdir -p "$stage_dir" "$state_dir/previous.new"
install -m 0755 "$source_binary" "$stage_dir/iptv"
cp -a "$source_dashboard" "$stage_dir/web-dist"
cp -a "$current_binary" "$state_dir/previous.new/iptv"
cp -a "$current_dashboard" "$state_dir/previous.new/web-dist"
rm -rf "$previous_dir"
mv "$state_dir/previous.new" "$previous_dir"

rollback_required=true
install -m 0755 "$stage_dir/iptv" "$deploy_root/.iptv.new"
rm -rf "$deploy_root/web/.dist.new"
cp -a "$stage_dir/web-dist" "$deploy_root/web/.dist.new"
mv -f "$deploy_root/.iptv.new" "$current_binary"
"$python_bin" "$atomic_exchange" "$current_dashboard" "$deploy_root/web/.dist.new"
dashboard_exchanged=true
"$systemctl_bin" --user restart "$service_name"
wait_for_health
rollback_required=false
dashboard_exchanged=false

echo "native deployment completed"
