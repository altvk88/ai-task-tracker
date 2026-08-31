#!/usr/bin/env bash
# PostToolUse hook: when an Edit/Write touches a TASK FILE inside the tracker vault,
# regenerate the Kanban board (normalize statuses first). Silent no-op otherwise.
# Registered for matcher "Edit|Write|MultiEdit". Reads the hook payload (JSON) from stdin.
set -u

payload="$(cat 2>/dev/null)"
[ -n "$payload" ] || exit 0

# best-effort extract of the edited file path (no jq dependency)
fp="$(printf '%s' "$payload" | sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
[ -n "$fp" ] || exit 0

reg="$HOME/.claude/task-tracker.json"
[ -f "$reg" ] || exit 0
vault="$(sed -n 's/.*"vault"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$reg" | head -1)"
[ -n "$vault" ] || exit 0

# normalize separators + case for a Windows-safe prefix match
norm() { printf '%s' "$1" | tr '\\' '/' | tr -s '/' | tr 'A-Z' 'a-z'; }
lfp="$(norm "$fp")"
lv="$(norm "$vault")"

case "$lfp" in
  "$lv"/tasks/*/*.md)
    # actual-case path (forward slashes) for tools that open the edited file directly
    fpu="$(printf '%s' "$fp" | tr '\\' '/' | tr -s '/')"
    bash "$HOME/.claude/lib/tt-normalize.sh" "$vault" >/dev/null 2>&1
    bash "$HOME/.claude/lib/tt-quote-fm.sh" "$vault" "$fpu" >/dev/null 2>&1   # self-heal YAML-breaking titles
    bash "$HOME/.claude/lib/tt-board.sh" "$vault" >/dev/null 2>&1
    ;;
esac
exit 0
