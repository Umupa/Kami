#!/usr/bin/env bash
# Runs one delegated task with the dsh headless agent (see AGENTS.md, "协作开发").
#
#   scripts/dst/run.sh <task-file> [session-id]
#
# The task brief is read from <task-file>; pass the session id printed by an
# earlier run to continue that conversation (e.g. to send review feedback).
# The full event stream is kept in .dst/runs/ for review.
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
	echo "usage: scripts/dst/run.sh <task-file> [session-id]" >&2
	exit 64
fi

root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
task=$1
session=${2:-}
log_dir="$root/.dst/runs"
mkdir -p "$log_dir"
log="$log_dir/$(date +%Y%m%d-%H%M%S).jsonl"

preamble="You are a contributor to this repository. Read AGENTS.md first and follow it.
Rules for this task:
- Change only the files the task names; ask in your final answer instead of widening scope.
- Do not run git commit, push, reset, checkout, stash or clean; leave changes in the working tree.
- Do not launch the app, send synthetic input, capture the screen or write the system clipboard.
- Do not read or write outside this repository except the Go module cache and build cache.
- Before finishing, run gofmt -l ., go vet ./..., go test ./... and fix what fails.
- End with: files changed, what you verified (with command results), open questions.

Task:
"

args=(--profile headless --patch "$root/scripts/dst/worker.patch.yml" --json)
[[ -n $session ]] && args+=(--session-id "$session")

status=0
(cd "$root" && { printf '%s' "$preamble"; cat "$task"; } | dsh "${args[@]}" >"$log") || status=$?

echo "log: ${log#"$root"/}"
jq -r 'select(.type == "session") | "session: \(.sessionId)"' "$log"
jq -r 'select(.type == "error") | "error: \(.message)"' "$log"
jq -r 'select(.type == "final") | .text' "$log"
exit $status
