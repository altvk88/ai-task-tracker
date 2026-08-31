#!/usr/bin/env bash
# Watch ONLY the Kanban board files; when a board changes (you dragged a card), push the new
# lane -> task frontmatter status (via tt-board-apply.sh), then regenerate all boards to stay
# consistent. Watches boards only (never task files) → cannot revert agent-driven status changes.
# Single-instance via a PID file. Usage: tt-board-watch.sh <vault> [interval_seconds]
set -u
shopt -s nullglob
vault="${1:-}"
[ -n "$vault" ] || { echo "tt-board-watch: vault required" >&2; exit 2; }
interval="${2:-2}"
here="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$vault/.locks"
pidfile="$vault/.locks/board-watch.pid"

if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile" 2>/dev/null)" 2>/dev/null; then
  echo "tt-board-watch: already running (pid $(cat "$pidfile"))"; exit 0
fi
echo $$ > "$pidfile"
trap 'rm -f "$pidfile"' EXIT

sig() { for b in "$vault"/kanban/*-board.md; do stat -c '%Y %s' "$b"; done | tr '\n' ' '; }

last="$(sig)"
echo "tt-board-watch: watching $vault/kanban/*-board.md every ${interval}s (pid $$)"
while :; do
  sleep "$interval"
  cur="$(sig)"
  if [ "$cur" != "$last" ]; then
    bash "$here/tt-board-apply.sh" "$vault" 2>&1 | sed 's/^/[watch] /'
    bash "$here/tt-board.sh" "$vault" >/dev/null 2>&1   # пересобрать все доски для консистентности
    last="$(sig)"                                        # ре-базлайн ПОСЛЕ пересборки → без петли
  fi
done
