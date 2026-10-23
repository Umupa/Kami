#!/usr/bin/env bash
# Runs the checks every change must pass: formatting, vet on both released
# architectures, MyGo's UI vet and the tests. Pass --integration to add the
# read-only macOS integration tests.
set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
unformatted=$(gofmt -l .)
if [[ -n $unformatted ]]; then
	echo "没有 gofmt 的文件：" >&2
	echo "$unformatted" >&2
	exit 1
fi
go vet ./...
GOARCH=amd64 go vet ./...
go tool mygo vet .
go test ./...
if [[ "${1:-}" == "--integration" ]]; then
	go test -tags integration ./internal/macos
fi
