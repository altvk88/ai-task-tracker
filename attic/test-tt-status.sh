#!/usr/bin/env bash
set -u
. "$(dirname "$0")/lib/assert.sh"
LIB="$(cd "$(dirname "$0")/../claude-global/lib" && pwd)"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
vault="$tmp/v"; dir="$vault/tasks/proj"; mkdir -p "$dir"

mk() {  # mk <file> <id> <status> <priority> <created>
  cat > "$dir/$1" <<EOF
---
id: $2
title: $1
status: $3
project: proj
priority: $4
created: $5
blocked_by:
---
## Log
EOF
}
mk a.md P-001 ready high 2026-06-02
mk b.md P-002 ready high 2026-06-01
mk c.md P-003 in-progress medium 2026-06-01
mk d.md P-004 backlog low 2026-06-01
mk e.md P-005 done high 2026-05-01
mk f.md P-006 needs-input medium 2026-06-01

# counts
counts="$(bash "$LIB/tt-status.sh" counts "$vault" proj)"
assert_contains "$counts" "ready=2" "counts: ready=2"
assert_contains "$counts" "in-progress=1" "counts: in-progress=1"
assert_contains "$counts" "needs-input=1" "counts: needs-input=1"
assert_contains "$counts" "done=1" "counts: done=1"

# next: high+самая ранняя created => b.md (P-002, 2026-06-01)
nextf="$(bash "$LIB/tt-status.sh" next "$vault" proj)"
assert_eq "$dir/b.md" "$nextf" "next = высший приоритет, затем старейшая"

# inprogress: содержит P-003
inp="$(bash "$LIB/tt-status.sh" inprogress "$vault" proj)"
assert_contains "$inp" "P-003" "inprogress перечисляет P-003"

# banner: одна строка с именем проекта и Ready: 2
banner="$(bash "$LIB/tt-status.sh" banner "$vault" proj)"
assert_contains "$banner" "proj" "banner содержит имя проекта"
assert_contains "$banner" "Ready: 2" "banner содержит Ready: 2"

finish
