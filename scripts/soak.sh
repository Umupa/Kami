#!/usr/bin/env bash
# Repeat real OCR, generated 4K captures and headless panel sessions in isolated processes.
set -euo pipefail
cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
for package in macos capture panel; do
 go test -tags 'integration soak' -count=1 -timeout=60m -run '^TestSoak' -v "./internal/$package"
done
