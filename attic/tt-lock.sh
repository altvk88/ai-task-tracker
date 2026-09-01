#!/usr/bin/env bash
# Атомарный lock задачи через создание директории (переносимо, без рантайма).
# Usage:
#   tt-lock.sh acquire   <vault> <task-id>   # mkdir lock; код 0 если захвачен, 1 если уже занят
#   tt-lock.sh release   <vault> <task-id>   # снять lock; код 0
#   tt-lock.sh is-locked <vault> <task-id>   # код 0 если залочен, иначе 1
set -u
cmd="${1:-}"; vault="${2:-}"; id="${3:-}"
lockdir="$vault/.locks/$id.lock"
case "$cmd" in
  acquire)
    mkdir -p "$vault/.locks" 2>/dev/null
    if mkdir "$lockdir" 2>/dev/null; then exit 0; else exit 1; fi   # mkdir атомарен
    ;;
  release)  rmdir "$lockdir" 2>/dev/null || rm -rf "$lockdir" 2>/dev/null; exit 0 ;;
  is-locked) [ -d "$lockdir" ] && exit 0 || exit 1 ;;
  *) echo "tt-lock: unknown command '$cmd'" >&2; exit 2 ;;
esac
