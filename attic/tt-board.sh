#!/usr/bin/env bash
# Regenerate Obsidian Kanban-plugin boards from task frontmatter (one-way: status -> board view).
# Writes:
#   <vault>/kanban/all-projects-board.md      (all projects)
#   <vault>/kanban/<project>-board.md         (one per project, tasks of that project only)
# Usage: tt-board.sh <vault>
#
# Implementation note: a single awk pass per board (instead of per-field sed/head pipelines).
# On git-bash/Windows process forks are expensive and can fail under load (cygheap errors);
# the old per-field version spawned thousands of processes and could die mid-write inside the
# 15s PostToolUse hook timeout, leaving a truncated board. Writes are atomic (tmp + mv) so an
# interrupted run can never leave a half-written board behind.
set -u
vault="${1:-}"
[ -n "$vault" ] || { echo "tt-board: vault required" >&2; exit 2; }
mkdir -p "$vault/kanban"
shopt -s nullglob

write_board() {  # write_board <outfile> <task-file...>
  out="$1"; shift
  [ "$#" -gt 0 ] || set -- /dev/null
  tmp="$out.tmp.$$"
  awk -v vlt="$vault" '
    function flush(   st, rel) {
      if (cur == "" || !("status" in fm)) { cur = ""; return }
      st = fm["status"]
      if (st == "in_progress") st = "in-progress"      # терпим оба написания
      if (st == "needs_input") st = "needs-input"
      if (st == "canceled")    st = "cancelled"         # терпим US-написание
      if (st == "on-hold" || st == "on_hold" || st == "onhold") st = "hold"
      rel = substr(cur, length(vlt) + 2)
      gsub(/\\/, "/", rel); sub(/\.md$/, "", rel)
      lane[st] = lane[st] "- [ ] [[" rel "|" fm["id"] " · " fm["project"] " · " fm["title"] "]]\n"
      cur = ""
    }
    FNR == 1 { flush(); cur = FILENAME; infm = 0; fmdone = 1; delete fm }
    FNR == 1 && $0 ~ /^---\r?[ \t]*$/ { infm = 1; fmdone = 0; next }
    fmdone { next }
    /^---\r?[ \t]*$/ { fmdone = 1; next }
    infm && /^(id|title|status|project):/ {
      line = $0; sub(/\r$/, "", line)
      p = index(line, ":")
      key = substr(line, 1, p - 1)
      val = substr(line, p + 1)
      gsub(/^[ \t]+|[ \t]+$/, "", val)
      if (!(key in fm)) fm[key] = val
    }
    END {
      flush()
      printf "---\n\nkanban-plugin: basic\n\n---\n\n"
      n = split("backlog ready in-progress needs-input blocked hold failed done cancelled", ord, " ")
      split("Backlog Ready In_Progress Needs_Input Blocked Hold Failed Done Canceled", hdr, " ")
      for (i = 1; i <= n; i++) {
        h = hdr[i]; gsub(/_/, " ", h)
        printf "## %s\n\n", h
        if (ord[i] in lane) printf "%s", lane[ord[i]]
        printf "\n"
      }
      print "%% kanban:settings"
      print "```"
      print "{\"kanban-plugin\":\"basic\",\"lane-width\":272,\"show-checkboxes\":false}"
      print "```"
      print "%%"
    }
  ' "$@" > "$tmp" && mv -f "$tmp" "$out" || { rm -f "$tmp"; echo "tt-board: failed writing $out" >&2; return 1; }
}

# --- all-projects board ---
all=( "$vault"/tasks/*/*.md )
write_board "$vault/kanban/all-projects-board.md" "${all[@]}"
echo "tt-board: wrote all-projects-board.md (${#all[@]} tasks)"

# --- per-project boards (skip _-prefixed sample dirs) ---
for d in "$vault"/tasks/*/; do
  proj="$(basename "$d")"
  case "$proj" in _*) continue;; esac
  pf=( "$d"*.md )
  write_board "$vault/kanban/$proj-board.md" "${pf[@]}"
  echo "tt-board: wrote $proj-board.md (${#pf[@]} tasks)"
done
