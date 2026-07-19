#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
deploy_script="$script_dir/deploy.sh"
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
exit 0
SYSTEMCTL
  chmod +x "$bin/systemctl"
}

test_successful_publication() {
  make_fixture success
  local case_dir="$tmp/success"

  cat > "$case_dir/bin/curl" <<'CURL'
#!/usr/bin/env bash
exit 0
CURL
  chmod +x "$case_dir/bin/curl"

  DEPLOY_ROOT="$case_dir/root" \
  SYSTEMCTL_BIN="$case_dir/bin/systemctl" \
  SYSTEMCTL_LOG="$case_dir/systemctl.log" \
  CURL_BIN="$case_dir/bin/curl" \
  HEALTH_ATTEMPTS=1 \
  HEALTH_DELAY_SECONDS=0 \
    "$deploy_script" "$case_dir/artifacts/iptv" "$case_dir/artifacts/web-dist"

  assert_content new-binary "$case_dir/root/iptv"
  assert_content new-dashboard "$case_dir/root/web/dist/index.html"
  assert_content old-binary "$case_dir/root/.deploy/previous/iptv"
  assert_content old-dashboard "$case_dir/root/.deploy/previous/web-dist/index.html"
  grep -Fq -- '--user restart iptv.service' "$case_dir/systemctl.log" || fail "service was not restarted"
  [[ -x "$case_dir/root/iptv" ]] || fail "installed binary is not executable"
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
[[ "$count" -gt 1 ]]
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
  [[ $(grep -Fc -- '--user restart iptv.service' "$case_dir/systemctl.log") -eq 2 ]] || fail "rollback did not restart service"
}

test_successful_publication
test_failed_health_restores_previous_release
printf 'native deploy tests passed\n'
