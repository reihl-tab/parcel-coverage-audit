#!/usr/bin/env bash
# Isolated JSON-only branch; never changes main or dispatches scan workflows.
set -euo pipefail
cp "$1"/tests.json /tmp/tests-badge.json
cp "$1"/coverage.json /tmp/coverage-badge.json
WORK=$(mktemp -d)
git fetch origin badge-data || true
if git show-ref --verify --quiet refs/remotes/origin/badge-data; then
  git worktree add "$WORK" origin/badge-data
else
  git worktree add --detach "$WORK" HEAD
  git -C "$WORK" switch --orphan badge-data
  git -C "$WORK" rm -rf --ignore-unmatch .
fi
cp /tmp/tests-badge.json "$WORK/tests.json"
cp /tmp/coverage-badge.json "$WORK/coverage.json"
git -C "$WORK" config user.name 'github-actions[bot]'
git -C "$WORK" config user.email "$(printf '%s' '41898282+github-actions[bot]' '@users.noreply.github.com')"
git -C "$WORK" add tests.json coverage.json
if ! git -C "$WORK" diff --cached --quiet; then
  git -C "$WORK" commit -m 'data: measured successful main CI badges'
  git -C "$WORK" push origin HEAD:badge-data
fi
