#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
test_dir=$(mktemp -d)
container_id=""
cleanup() {
    if [[ -n "$container_id" ]]; then
        docker rm -f "$container_id" >/dev/null
    fi
    rm -rf "$test_dir"
}
trap cleanup EXIT

case "$(docker info --format '{{.Architecture}}')" in
    aarch64|arm64) test_arch=arm64 ;;
    x86_64|amd64) test_arch=amd64 ;;
    *) echo 'Unsupported Docker architecture' >&2; exit 1 ;;
esac

# The pinned 2019 baseline has two unrelated printf-vet errors in UnMount.
go test -mod=vendor -vet=off ./...
GOOS=linux GOARCH="$test_arch" CGO_ENABLED=0 \
    go test -mod=vendor -vet=off -c -o "$test_dir/recovery.test" ./manager
docker build -t lium-loopback-recovery-tests:local -f tests/recovery.Dockerfile .
container_id=$(docker create --privileged --network none \
    --label lium.test=loopback-mount-recovery \
    -e LOOPBACK_MOUNT_TESTS=1 \
    lium-loopback-recovery-tests:local /recovery.test -test.v)
docker cp "$test_dir/recovery.test" "$container_id:/recovery.test"
docker start -a "$container_id"
test "$(docker inspect --format '{{.State.Status}}' "$container_id")" = exited
exit "$(docker inspect --format '{{.State.ExitCode}}' "$container_id")"
