#!/bin/sh
set -eu

engine="${CONTAINER_ENGINE:-podman}"
image="${1:-localhost/upgopher:adoption-test}"
name="upgopher-smoke-$$"
fixture_dir="$(mktemp -d)"
current_container=""

cleanup() {
  if [ -n "$current_container" ]; then
    "$engine" stop --time 0 "$current_container" >/dev/null 2>&1 || true
  fi
  rm -rf "$fixture_dir"
}
trap cleanup EXIT INT TERM

start_container() {
  current_container="$name-$1"
  shift
  "$engine" run -d --rm --network host --name "$current_container" "$image" "$@" >/dev/null
}

stop_container() {
  "$engine" stop --time 0 "$current_container" >/dev/null
  current_container=""
}

wait_for_url() {
  curl --retry 20 --retry-connrefused --retry-delay 1 --silent --show-error \
    --output /dev/null "$1"
}

image_user="$("$engine" image inspect --format '{{.Config.User}}' "$image")"
test "$image_user" = "65532:65532"

printf '%s\n' "Upgopher container smoke test" >"$fixture_dir/payload.txt"

start_container files
wait_for_url http://127.0.0.1:9090/
upload_code="$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --form "file=@$fixture_dir/payload.txt" http://127.0.0.1:9090/)"
test "$upload_code" = "303"
zip_code="$(curl --silent --output "$fixture_dir/files.zip" --write-out '%{http_code}' \
  http://127.0.0.1:9090/zip)"
test "$zip_code" = "200"
test -s "$fixture_dir/files.zip"
stop_container

start_container readonly -dir /data -readonly
wait_for_url http://127.0.0.1:9090/
readonly_code="$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --form "file=@$fixture_dir/payload.txt" http://127.0.0.1:9090/)"
test "$readonly_code" = "403"
stop_container

start_container auth -dir /data -user demo -pass smoke-test-only
wait_for_url http://127.0.0.1:9090/
unauthorized_code="$(curl --silent --output /dev/null --write-out '%{http_code}' \
  http://127.0.0.1:9090/)"
authorized_code="$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --user demo:smoke-test-only http://127.0.0.1:9090/)"
test "$unauthorized_code" = "401"
test "$authorized_code" = "200"
stop_container

start_container tls -dir /data -ssl -port 9443
curl --insecure --retry 20 --retry-connrefused --retry-delay 1 --silent \
  --show-error --output /dev/null https://127.0.0.1:9443/
stop_container

printf '%s\n' "container smoke test passed"
