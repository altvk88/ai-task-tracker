#!/usr/bin/env bash
set -u
. "$(dirname "$0")/lib/assert.sh"
LIB="$(cd "$(dirname "$0")/../claude-global/lib" && pwd)"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
vault="$tmp/v"; dir="$vault/tasks/proj"; mkdir -p "$dir" "$vault/kanban"
fm(){ sed -n '/^---$/,/^---$/p' "$1" | sed -n 's/^'"$2"':[[:space:]]*//p' | head -1 | sed 's/[[:space:]]*$//'; }

mk(){ cat > "$dir/$1" <<EOF
---
id: $2
title: $1
status: $3
project: proj
priority: high
created: 2026-06-01
completed:
ready_at:
blocked_by:
---
## Log
EOF
}
mk a.md P-001 backlog
mk b.md P-002 ready

# доска: P-001 перетащили в Ready, P-002 — в Done
cat > "$vault/kanban/all-projects-board.md" <<'BOARD'
---

kanban-plugin: basic

---

## Ready

- [ ] [[tasks/proj/a|P-001 · proj · a]]

## In Progress


## Needs Input


## Blocked


## Backlog


## Failed


## Done

- [ ] [[tasks/proj/b|P-002 · proj · b]]

%% kanban:settings
```
{"kanban-plugin":"basic"}
```
%%
BOARD

bash "$LIB/tt-board-apply.sh" "$vault" >/dev/null
assert_eq "ready" "$(fm "$dir/a.md" status)" "P-001 backlog->ready применился"
assert_eq "done"  "$(fm "$dir/b.md" status)" "P-002 ready->done применился"
assert_eq "2026-06-01" "$(fm "$dir/a.md" created)" "created не тронут"
[ -n "$(fm "$dir/b.md" completed)" ]; assert_success $? "completed проставлен при ->done"
[ -n "$(fm "$dir/a.md" ready_at)" ]; assert_success $? "ready_at проставлен при ->ready"
finish
