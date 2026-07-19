#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
deploy_script="$script_dir/deploy.sh"
atomic_exchange="$script_dir/atomic_exchange.py"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

assert_content() {
  local expected=$1
  local file=$2
  [[ -f "$file" ]] || fail "missing $file"
  [[ $(<"$file") == "$expected" ]] || fail "unexpected content in $file"
}

make_fixture() {
  local name=$1
  local root="$tmp/$name/root"
  local artifacts="$tmp/$name/artifacts"
  local bin="$tmp/$name/bin"

  mkdir -p "$root/web/dist" "$root/apk_storage" "$root/image_storage"
  mkdir -p "$artifacts/web-dist" "$bin"
  printf 'old-binary' > "$root/iptv"
  chmod 0755 "$root/iptv"
  printf 'old-dashboard' > "$root/web/dist/index.html"
  printf 'new-binary' > "$artifacts/iptv"
  chmod 0755 "$artifacts/iptv"
  printf 'new-dashboard' > "$artifacts/web-dist/index.html"

  cat > "$bin/systemctl" <<'SYSTEMCTL'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$SYSTEMCTL_LOG"
if [[ "$*" == "--user restart iptv.service" && -n "${DASHBOARD_STATE_LOG:-}" ]]; then
  [[ -f "$DEPLOY_ROOT/web/dist/index.html" && -f "$DEPLOY_ROOT/web/.dist.new/index.html" ]] || exit 1
  printf '%s|%s\n' \
    "$(<"$DEPLOY_ROOT/web/dist/index.html")" \
    "$(<"$DEPLOY_ROOT/web/.dist.new/index.html")" >> "$DASHBOARD_STATE_LOG"
fi
exit 0
SYSTEMCTL
  chmod +x "$bin/systemctl"
}

test_successful_publication() {
  make_fixture success
  local case_dir="$tmp/success"

  cat > "$case_dir/bin/curl" <<'CURL'
#!/usr/bin/env bash
printf '200'
CURL
  chmod +x "$case_dir/bin/curl"

  DEPLOY_ROOT="$case_dir/root" \
  SYSTEMCTL_BIN="$case_dir/bin/systemctl" \
  SYSTEMCTL_LOG="$case_dir/systemctl.log" \
  DASHBOARD_STATE_LOG="$case_dir/dashboard-state.log" \
  CURL_BIN="$case_dir/bin/curl" \
  HEALTH_ATTEMPTS=1 \
  HEALTH_DELAY_SECONDS=0 \
    "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist"

  assert_content new-binary "$case_dir/root/iptv"
  assert_content new-dashboard "$case_dir/root/web/dist/index.html"
  assert_content old-binary "$case_dir/root/.deploy/previous/iptv"
  assert_content old-dashboard "$case_dir/root/.deploy/previous/web-dist/index.html"
  grep -Fq -- '--user restart iptv.service' "$case_dir/systemctl.log" || fail "service was not restarted"
  grep -Fxq 'new-dashboard|old-dashboard' "$case_dir/dashboard-state.log" || fail "dashboard was not atomically exchanged before restart"
  [[ -x "$case_dir/root/iptv" ]] || fail "installed binary is not executable"
  [[ ! -e "$case_dir/root/web/.dist.new" ]] || fail "exchanged dashboard was not cleaned up"
}

test_dashboard_publication_uses_atomic_exchange() {
  [[ -f "$atomic_exchange" ]] || fail "missing atomic exchange helper"
  if grep -Fq 'rm -rf "$current_dashboard"' "$deploy_script"; then
    fail "dashboard publication still removes the live directory"
  fi

  local case_dir="$tmp/atomic"
  mkdir -p "$case_dir/current" "$case_dir/replacement"
  printf 'current' > "$case_dir/current/index.html"
  printf 'replacement' > "$case_dir/replacement/index.html"

  python3 "$atomic_exchange" "$case_dir/current" "$case_dir/replacement"

  assert_content replacement "$case_dir/current/index.html"
  assert_content current "$case_dir/replacement/index.html"

  python3 "$atomic_exchange" "$case_dir/current" "$case_dir/replacement"

  assert_content current "$case_dir/current/index.html"
  assert_content replacement "$case_dir/replacement/index.html"
}

test_non_200_health_restores_previous_release() {
  make_fixture non-200
  local case_dir="$tmp/non-200"

  cat > "$case_dir/bin/curl" <<'CURL'
#!/usr/bin/env bash
count=0
[[ -f "$CURL_COUNT_FILE" ]] && count=$(<"$CURL_COUNT_FILE")
count=$((count + 1))
printf '%s' "$count" > "$CURL_COUNT_FILE"
if [[ "$count" -eq 1 ]]; then
  printf '204'
else
  printf '200'
fi
CURL
  chmod +x "$case_dir/bin/curl"

  if DEPLOY_ROOT="$case_dir/root" \
    SYSTEMCTL_BIN="$case_dir/bin/systemctl" \
    SYSTEMCTL_LOG="$case_dir/systemctl.log" \
    CURL_BIN="$case_dir/bin/curl" \
    CURL_COUNT_FILE="$case_dir/curl.count" \
    HEALTH_ATTEMPTS=1 \
    HEALTH_DELAY_SECONDS=0 \
      "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist"; then
    fail "deployment with non-200 health unexpectedly succeeded"
  fi

  assert_content old-binary "$case_dir/root/iptv"
  assert_content old-dashboard "$case_dir/root/web/dist/index.html"
  [[ ! -e "$case_dir/root/web/.dist.new" ]] || fail "failed dashboard was not cleaned up after rollback"
  [[ $(grep -Fc -- '--user restart iptv.service' "$case_dir/systemctl.log") -eq 2 ]] || fail "non-200 health did not trigger rollback"
}

test_failed_health_restores_previous_release() {
  make_fixture rollback
  local case_dir="$tmp/rollback"

  cat > "$case_dir/bin/curl" <<'CURL'
#!/usr/bin/env bash
count=0
[[ -f "$CURL_COUNT_FILE" ]] && count=$(<"$CURL_COUNT_FILE")
count=$((count + 1))
printf '%s' "$count" > "$CURL_COUNT_FILE"
if [[ "$count" -gt 1 ]]; then
  printf '200'
else
  exit 1
fi
CURL
  chmod +x "$case_dir/bin/curl"

  if DEPLOY_ROOT="$case_dir/root" \
    SYSTEMCTL_BIN="$case_dir/bin/systemctl" \
    SYSTEMCTL_LOG="$case_dir/systemctl.log" \
    CURL_BIN="$case_dir/bin/curl" \
    CURL_COUNT_FILE="$case_dir/curl.count" \
    HEALTH_ATTEMPTS=1 \
    HEALTH_DELAY_SECONDS=0 \
      "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist"; then
    fail "deployment unexpectedly succeeded"
  fi

  assert_content old-binary "$case_dir/root/iptv"
  assert_content old-dashboard "$case_dir/root/web/dist/index.html"
  [[ ! -e "$case_dir/root/web/.dist.new" ]] || fail "failed dashboard was not cleaned up after rollback"
  [[ $(grep -Fc -- '--user restart iptv.service' "$case_dir/systemctl.log") -eq 2 ]] || fail "rollback did not restart service"
}

test_rollback_operation_failure_is_reported() {
  make_fixture rollback-failure
  local case_dir="$tmp/rollback-failure"
  local output

  cat > "$case_dir/bin/curl" <<'CURL'
#!/usr/bin/env bash
count=0
[[ -f "$CURL_COUNT_FILE" ]] && count=$(<"$CURL_COUNT_FILE")
count=$((count + 1))
printf '%s' "$count" > "$CURL_COUNT_FILE"
if [[ "$count" -gt 1 ]]; then
  printf '200'
else
  exit 1
fi
CURL
  chmod +x "$case_dir/bin/curl"

  cat > "$case_dir/bin/install" <<'INSTALL'
#!/usr/bin/env bash
count=0
[[ -f "$INSTALL_COUNT_FILE" ]] && count=$(<"$INSTALL_COUNT_FILE")
count=$((count + 1))
printf '%s' "$count" > "$INSTALL_COUNT_FILE"
if [[ "$count" -eq 3 ]]; then
  exit 1
fi
exec /usr/bin/install "$@"
INSTALL
  chmod +x "$case_dir/bin/install"

  if output=$(PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT="$case_dir/root" \
    SYSTEMCTL_BIN="$case_dir/bin/systemctl" \
    SYSTEMCTL_LOG="$case_dir/systemctl.log" \
    CURL_BIN="$case_dir/bin/curl" \
    CURL_COUNT_FILE="$case_dir/curl.count" \
    INSTALL_COUNT_FILE="$case_dir/install.count" \
    HEALTH_ATTEMPTS=1 \
    HEALTH_DELAY_SECONDS=0 \
      "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist" 2>&1); then
    fail "deployment with failed rollback operation unexpectedly succeeded"
  fi

  grep -Fq 'rollback failed; inspect iptv.service immediately' <<<"$output" || fail "rollback operation failure was not reported"
  if grep -Fq 'previous release restored' <<<"$output"; then
    fail "failed rollback was reported as successful"
  fi
  assert_content new-binary "$case_dir/root/iptv"
  assert_content old-dashboard "$case_dir/root/web/dist/index.html"
  [[ ! -e "$case_dir/root/web/.dist.new" ]] || fail "failed dashboard was not cleaned up after partial rollback"
  [[ $(grep -Fc -- '--user restart iptv.service' "$case_dir/systemctl.log") -eq 1 ]] || fail "rollback continued to service restart after operation failure"
}

test_successful_publication
test_dashboard_publication_uses_atomic_exchange
test_non_200_health_restores_previous_release
test_failed_health_restores_previous_release
test_rollback_operation_failure_is_reported
printf 'native deploy tests passed\n'
