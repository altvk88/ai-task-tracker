#!/usr/bin/env bash
# Reverse sync: read the Kanban board(s) and push each card's lane back into the task's frontmatter
# status (board -> frontmatter). Reads ALL <vault>/kanban/*-board.md (all-projects + per-project),
# oldest-first by mtime so the most recently dragged board wins on conflict.
# Usage: tt-board-apply.sh <vault>
set -u
vault="${1:-}"
[ -n "$vault" ] || { echo "tt-board-apply: vault required" >&2; exit 2; }
shopt -s nullglob

lane_status() {
  case "$1" in
    "Ready")        echo ready;;
    "In Progress")  echo in-progress;;
    "Needs Input")  echo needs-input;;
    "Blocked")      echo blocked;;
    "Hold")         echo hold;;
    "Backlog")      echo backlog;;
    "Failed")       echo failed;;
    "Done")         echo done;;
    "Canceled")     echo cancelled;;
    "Cancelled")    echo cancelled;;
    *)              echo "";;
  esac
}

fm() { sed -n '/^---$/,/^---$/p' "$1" | sed -n 's/^'"$2"':[[:space:]]*//p' | head -1 | sed 's/[[:space:]]*$//'; }
setfield() { sed -i.bak "s|^$2:.*|$2: $3|" "$1" && rm -f "$1.bak"; }

today="$(date +%F)"
changed=0

apply_board() {  # apply_board <board-file>
  local board="$1" status="" line rel f cur
  while IFS= read -r line; do
    line="${line%$'\r'}"   # tolerate CRLF boards (Obsidian / PowerShell may write \r\n)
    case "$line" in
      "## "*) status="$(lane_status "${line#\#\# }")" ;;
      "- "*"[["*)
        [ -n "$status" ] || continue
        rel="${line#*[[}"; rel="${rel%%|*}"; rel="${rel%%]]*}"
        f="$vault/$rel.md"
        [ -f "$f" ] || continue
        cur="$(fm "$f" status)"; cur="${cur//in_progress/in-progress}"
        [ "$cur" = "$status" ] && continue
        # TT_DRY_RUN=1 → report planned changes only, write nothing (used by the watcher to gate)
        if [ -z "${TT_DRY_RUN:-}" ]; then
          setfield "$f" status "$status"
          if [ "$status" = "done" ] && [ -z "$(fm "$f" completed)" ]; then setfield "$f" completed "$today"; fi
          if [ "$status" = "ready" ] && [ -z "$(fm "$f" ready_at)" ]; then setfield "$f" ready_at "$today"; fi
          # leaving in-progress (dragged to done/ready/backlog/...) → release claim + lock, like /done
          if [ "$status" != "in-progress" ]; then
            [ -n "$(fm "$f" claim)" ] && { sed -i.bak 's|^claim:.*|claim:|' "$f" && rm -f "$f.bak"; }
            rm -rf "$vault/.locks/$(fm "$f" id).lock" 2>/dev/null
          fi
        fi
        echo "  $(fm "$f" id): $cur -> $status"
        changed=$((changed+1))
        ;;
    esac
  done < "$board"
}

# boards oldest-first (newest dragged board applied last = wins on conflict)
boards=( "$vault"/kanban/*-board.md )
[ "${#boards[@]}" -gt 0 ] || { echo "tt-board-apply: no boards in $vault/kanban" >&2; exit 1; }
while IFS= read -r b; do
  [ -n "$b" ] && apply_board "$b"
done < <(ls -1tr "${boards[@]}" 2>/dev/null)

if [ -n "${TT_DRY_RUN:-}" ]; then
  echo "tt-board-apply: $changed task(s) would update (dry-run)"
else
  echo "tt-board-apply: $changed task(s) updated"
fi
