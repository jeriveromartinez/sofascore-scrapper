#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
deploy_script="$script_dir/deploy.sh"
atomic_exchange="$script_dir/atomic_exchange.py"
expected_sha=1111111111111111111111111111111111111111
advanced_sha=2222222222222222222222222222222222222222
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
if [[ "$*" == "--user restart iptv.service" && -n "${SERVICE_STATE_LOG:-}" ]]; then
  printf '%s|%s\n' \
    "$(<"$DEPLOY_ROOT/iptv")" \
    "$(<"$DEPLOY_ROOT/web/dist/index.html")" >> "$SERVICE_STATE_LOG"
fi
if [[ "$*" == "--user restart iptv.service" && -n "${DASHBOARD_STATE_LOG:-}" ]]; then
  [[ -f "$DEPLOY_ROOT/web/dist/index.html" && -f "$DEPLOY_ROOT/web/.dist.new/index.html" ]] || exit 1
  printf '%s|%s\n' \
    "$(<"$DEPLOY_ROOT/web/dist/index.html")" \
    "$(<"$DEPLOY_ROOT/web/.dist.new/index.html")" >> "$DASHBOARD_STATE_LOG"
fi
if [[ "${INTERRUPT_PHASE:-}" == after-dashboard-exchange \
  && "$*" == "--user restart iptv.service" \
  && ! -e "$SIGNAL_SENT_FILE" ]]; then
  touch "$SIGNAL_SENT_FILE"
  kill -s "$INTERRUPT_SIGNAL" "$PPID"
  if [[ "$INTERRUPT_SIGNAL" == INT ]]; then
    trap - INT
    kill -INT "$$"
  fi
fi
if [[ "${INTERRUPT_PHASE:-}" == after-restart \
  && "$*" == "--user is-active --quiet iptv.service" \
  && ! -e "$SIGNAL_SENT_FILE" ]]; then
  touch "$SIGNAL_SENT_FILE"
  kill -s "$INTERRUPT_SIGNAL" "$PPID"
  if [[ "$INTERRUPT_SIGNAL" == INT ]]; then
    trap - INT
    kill -INT "$$"
  fi
fi
exit 0
SYSTEMCTL
  chmod +x "$bin/systemctl"

  cat > "$bin/git" <<'GIT'
#!/usr/bin/env bash
[[ "$*" == "ls-remote --exit-code origin refs/heads/main" ]] || exit 2
count=0
[[ -f "$GIT_COUNT_FILE" ]] && count=$(<"$GIT_COUNT_FILE")
count=$((count + 1))
printf '%s' "$count" > "$GIT_COUNT_FILE"
sha=$REMOTE_MAIN_SHA
if [[ "$count" -gt 1 && -n "${REMOTE_MAIN_SHA_AFTER_FIRST:-}" ]]; then
  sha=$REMOTE_MAIN_SHA_AFTER_FIRST
fi
printf '%s\trefs/heads/main\n' "$sha"
GIT
  chmod +x "$bin/git"
}

run_interrupted_deployment() {
  local phase=$1
  local signal=$2
  local expected_status=$3
  local name="interrupt-${phase}-${signal}"
  local case_dir="$tmp/$name"
  local output
  local output_file="$case_dir/output.log"
  local status

  make_fixture "$name"
  cat > "$case_dir/bin/curl" <<'CURL'
#!/usr/bin/env bash
printf '200'
CURL
  chmod +x "$case_dir/bin/curl"

  cat > "$case_dir/bin/mv" <<'MV'
#!/usr/bin/env bash
/usr/bin/mv "$@"
if [[ "${INTERRUPT_PHASE:-}" == after-binary \
  && "$*" == *"/.iptv.new "* \
  && ! -e "$SIGNAL_SENT_FILE" ]]; then
  touch "$SIGNAL_SENT_FILE"
  kill -s "$INTERRUPT_SIGNAL" "$PPID"
  if [[ "$INTERRUPT_SIGNAL" == INT ]]; then
    trap - INT
    kill -INT "$$"
  fi
fi
MV
  chmod +x "$case_dir/bin/mv"

  set +e
  PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT="$case_dir/root" \
    SYSTEMCTL_BIN="$case_dir/bin/systemctl" \
    SYSTEMCTL_LOG="$case_dir/systemctl.log" \
    SERVICE_STATE_LOG="$case_dir/service-state.log" \
    CURL_BIN="$case_dir/bin/curl" \
    GIT_BIN="$case_dir/bin/git" \
    GIT_COUNT_FILE="$case_dir/git.count" \
    REMOTE_MAIN_SHA="$expected_sha" \
    INTERRUPT_PHASE="$phase" \
    INTERRUPT_SIGNAL="$signal" \
    SIGNAL_SENT_FILE="$case_dir/signal.sent" \
    HEALTH_ATTEMPTS=1 \
    HEALTH_DELAY_SECONDS=0 \
      "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist" "$expected_sha" >"$output_file" 2>&1
  status=$?
  set -e
  output=$(<"$output_file")

  [[ "$status" -eq "$expected_status" ]] || fail "$signal after $phase exited $status instead of $expected_status: $output"
  grep -Fq "deployment interrupted by $signal; restoring previous release" <<<"$output" || fail "$signal after $phase did not report rollback"
  assert_content old-binary "$case_dir/root/iptv"
  assert_content old-dashboard "$case_dir/root/web/dist/index.html"
  [[ $(tail -n 1 "$case_dir/service-state.log") == 'old-binary|old-dashboard' ]] || fail "$signal after $phase did not restore service state"
  [[ ! -e "$case_dir/root/web/.dist.new" ]] || fail "$signal after $phase left exchanged state after rollback"
}

test_interruptions_restore_previous_release() {
  local phase
  local signal
  local status
  for phase in after-binary after-dashboard-exchange after-restart; do
    for signal in INT TERM; do
      status=130
      [[ "$signal" == TERM ]] && status=143
      run_interrupted_deployment "$phase" "$signal" "$status"
    done
  done
}

test_stale_before_mutation_does_not_change_production() {
  make_fixture stale-before
  local case_dir="$tmp/stale-before"
  local output

  cat > "$case_dir/bin/curl" <<'CURL'
#!/usr/bin/env bash
printf '200'
CURL
  chmod +x "$case_dir/bin/curl"

  if output=$(DEPLOY_ROOT="$case_dir/root" \
    SYSTEMCTL_BIN="$case_dir/bin/systemctl" \
    SYSTEMCTL_LOG="$case_dir/systemctl.log" \
    CURL_BIN="$case_dir/bin/curl" \
    GIT_BIN="$case_dir/bin/git" \
    GIT_COUNT_FILE="$case_dir/git.count" \
    REMOTE_MAIN_SHA="$advanced_sha" \
    HEALTH_ATTEMPTS=1 \
    HEALTH_DELAY_SECONDS=0 \
      "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist" "$expected_sha" 2>&1); then
    fail "stale deployment unexpectedly succeeded"
  fi

  grep -Fq "selected SHA $expected_sha is not current main $advanced_sha" <<<"$output" || fail "stale deployment was not rejected inside publisher"
  assert_content old-binary "$case_dir/root/iptv"
  assert_content old-dashboard "$case_dir/root/web/dist/index.html"
  [[ ! -e "$case_dir/systemctl.log" ]] || fail "stale deployment restarted the service"
  [[ ! -e "$case_dir/root/web/.dist.new" ]] || fail "stale deployment left staged dashboard state"
}

test_main_advance_during_publication_restores_previous_release() {
  make_fixture main-advance
  local case_dir="$tmp/main-advance"
  local output

  cat > "$case_dir/bin/curl" <<'CURL'
#!/usr/bin/env bash
printf '200'
CURL
  chmod +x "$case_dir/bin/curl"

  if output=$(DEPLOY_ROOT="$case_dir/root" \
    SYSTEMCTL_BIN="$case_dir/bin/systemctl" \
    SYSTEMCTL_LOG="$case_dir/systemctl.log" \
    CURL_BIN="$case_dir/bin/curl" \
    GIT_BIN="$case_dir/bin/git" \
    GIT_COUNT_FILE="$case_dir/git.count" \
    REMOTE_MAIN_SHA="$expected_sha" \
    REMOTE_MAIN_SHA_AFTER_FIRST="$advanced_sha" \
    HEALTH_ATTEMPTS=1 \
    HEALTH_DELAY_SECONDS=0 \
      "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist" "$expected_sha" 2>&1); then
    fail "deployment whose main advanced unexpectedly succeeded"
  fi

  grep -Fq "selected SHA $expected_sha is not current main $advanced_sha" <<<"$output" || fail "main advance was not detected inside publisher"
  grep -Fq 'previous release restored' <<<"$output" || fail "main advance did not report successful rollback"
  assert_content old-binary "$case_dir/root/iptv"
  assert_content old-dashboard "$case_dir/root/web/dist/index.html"
  [[ $(grep -Fc -- '--user restart iptv.service' "$case_dir/systemctl.log") -eq 2 ]] || fail "main advance did not restore service state"
  [[ ! -e "$case_dir/root/web/.dist.new" ]] || fail "main advance left exchanged state after rollback"
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
  GIT_BIN="$case_dir/bin/git" \
  GIT_COUNT_FILE="$case_dir/git.count" \
  REMOTE_MAIN_SHA="$expected_sha" \
  HEALTH_ATTEMPTS=1 \
  HEALTH_DELAY_SECONDS=0 \
    "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist" "$expected_sha"

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
    GIT_BIN="$case_dir/bin/git" \
    GIT_COUNT_FILE="$case_dir/git.count" \
    REMOTE_MAIN_SHA="$expected_sha" \
    HEALTH_ATTEMPTS=1 \
    HEALTH_DELAY_SECONDS=0 \
      "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist" "$expected_sha"; then
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
    GIT_BIN="$case_dir/bin/git" \
    GIT_COUNT_FILE="$case_dir/git.count" \
    REMOTE_MAIN_SHA="$expected_sha" \
    HEALTH_ATTEMPTS=1 \
    HEALTH_DELAY_SECONDS=0 \
      "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist" "$expected_sha"; then
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
    GIT_BIN="$case_dir/bin/git" \
    GIT_COUNT_FILE="$case_dir/git.count" \
    REMOTE_MAIN_SHA="$expected_sha" \
    HEALTH_ATTEMPTS=1 \
    HEALTH_DELAY_SECONDS=0 \
      "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist" "$expected_sha" 2>&1); then
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

tests=(
  test_successful_publication
  test_dashboard_publication_uses_atomic_exchange
  test_non_200_health_restores_previous_release
  test_failed_health_restores_previous_release
  test_rollback_operation_failure_is_reported
  test_interruptions_restore_previous_release
  test_stale_before_mutation_does_not_change_production
  test_main_advance_during_publication_restores_previous_release
)
if (($#)); then
  tests=("$@")
fi
for test_name in "${tests[@]}"; do
  "$test_name"
done
printf 'native deploy tests passed\n'
