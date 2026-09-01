#!/usr/bin/env bash
# Normalize non-canonical status spellings in task frontmatter to the project's taxonomy.
#   in_progress -> in-progress,  needs_input -> needs-input
# Usage: tt-normalize.sh <vault>
#
# Implementation note: one awk pass finds offenders; sed runs only on those files
# (steady state: zero rewrites). The old version forked 4+ processes per task file,
# which is slow/fragile on git-bash/Windows inside hook timeouts.
set -u
vault="${1:-}"
[ -n "$vault" ] || { echo "tt-normalize: vault required" >&2; exit 2; }
shopt -s nullglob

files=( "$vault"/tasks/*/*.md )
n=0
if [ "${#files[@]}" -gt 0 ]; then
  # print "<status>\t<file>" for files whose frontmatter status needs normalizing
  while IFS=$'\t' read -r cur f; do
    [ -n "$f" ] || continue
    case "$cur" in
      in_progress) new="in-progress";;
      needs_input) new="needs-input";;
      canceled) new="cancelled";;
      on-hold|on_hold|onhold) new="hold";;
      *) continue;;
    esac
    sed -i.bak "s|^status:[[:space:]]*$cur[[:space:]]*\$|status: $new|" "$f" && rm -f "$f.bak"
    echo "  $(basename "$f"): $cur -> $new"
    n=$((n+1))
  done < <(awk '
    FNR == 1 { infm = 0; fmdone = 1; found = 0 }
    FNR == 1 && $0 ~ /^---\r?[ \t]*$/ { infm = 1; fmdone = 0; next }
    fmdone { next }
    /^---\r?[ \t]*$/ { fmdone = 1; next }
    infm && !found && /^status:/ {
      found = 1
      line = $0; sub(/\r$/, "", line)
      sub(/^status:[ \t]*/, "", line); sub(/[ \t]+$/, "", line)
      if (line == "in_progress" || line == "needs_input" || line == "canceled" \
          || line == "on-hold" || line == "on_hold" || line == "onhold") printf "%s\t%s\n", line, FILENAME
    }
  ' "${files[@]}")
fi
echo "tt-normalize: $n file(s) normalized"
