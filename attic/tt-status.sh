#!/usr/bin/env bash
# Инспекция файлов задач проекта в vault.
# Usage:
#   tt-status.sh counts     <vault> <project>   # "backlog=.. ready=.. in-progress=.. needs-input=.. blocked=.. hold=.. failed=.. done=.. cancelled=.."
#   tt-status.sh next       <vault> <project>   # путь к следующей ready (priority desc, created asc); пусто если нет
#   tt-status.sh inprogress <vault> <project>   # строки "<id>" для in-progress задач
#   tt-status.sh banner     <vault> <project>   # одна строка-сводка
set -u
cmd="${1:-}"; vault="${2:-}"; project="${3:-}"
dir="$vault/tasks/$project"

fm() {  # fm <file> <key> -> первый скаляр из frontmatter (без ведущих пробелов в ключе)
  sed -n '/^---$/,/^---$/p' "$1" | sed -n 's/^'"$2"':[[:space:]]*//p' | head -1 | sed 's/[[:space:]]*$//'
}
prio_rank() { case "$1" in critical) echo 0;; high) echo 1;; medium) echo 2;; low) echo 3;; *) echo 4;; esac; }

case "$cmd" in
  counts)
    b=0; r=0; ip=0; ni=0; bk=0; hd=0; fl=0; dn=0; cn=0
    for f in "$dir"/*.md; do [ -e "$f" ] || continue
      case "$(fm "$f" status)" in
        backlog) b=$((b+1));; ready) r=$((r+1));; in-progress) ip=$((ip+1));;
        needs-input) ni=$((ni+1));; blocked) bk=$((bk+1));; hold) hd=$((hd+1));;
        failed) fl=$((fl+1));; done) dn=$((dn+1));; cancelled) cn=$((cn+1));;
      esac
    done
    printf 'backlog=%s ready=%s in-progress=%s needs-input=%s blocked=%s hold=%s failed=%s done=%s cancelled=%s\n' "$b" "$r" "$ip" "$ni" "$bk" "$hd" "$fl" "$dn" "$cn"
    ;;
  next)
    best=""; best_key=""
    for f in "$dir"/*.md; do [ -e "$f" ] || continue
      [ "$(fm "$f" status)" = "ready" ] || continue
      key="$(prio_rank "$(fm "$f" priority)")|$(fm "$f" created)"
      if [ -z "$best_key" ] || [ "$key" \< "$best_key" ]; then best_key="$key"; best="$f"; fi
    done
    [ -n "$best" ] && printf '%s\n' "$best"
    ;;
  inprogress)
    for f in "$dir"/*.md; do [ -e "$f" ] || continue
      [ "$(fm "$f" status)" = "in-progress" ] || continue
      printf '%s\n' "$(fm "$f" id)"
    done
    ;;
  banner)
    c="$(bash "$0" counts "$vault" "$project" 2>/dev/null)"
    g() { printf '%s\n' "$c" | tr ' ' '\n' | sed -n 's/^'"$1"'=//p'; }
    printf '📋 %s — Ready: %s · In-progress: %s · Needs-input: %s · Blocked: %s · Failed: %s\n' \
      "$project" "$(g ready)" "$(g in-progress)" "$(g needs-input)" "$(g blocked)" "$(g failed)"
    ;;
  *) echo "tt-status: unknown command '$cmd'" >&2; exit 2 ;;
esac
