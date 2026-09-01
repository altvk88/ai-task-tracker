#!/usr/bin/env bash
# Quote frontmatter scalar fields whose UNQUOTED value would break YAML — i.e. it contains
# ": " (colon-space) or ends with ":". Obsidian can't parse such frontmatter and renders the
# whole block as raw red text (looks like an error). This self-heals task files.
#
# Skips values already quoted (" or ') or structured ([ list ] / { map }). Escapes \ and " for
# the double-quoted form. Frontmatter-only (first --- ... --- block); body is never touched.
#
# Usage: tt-quote-fm.sh <vault> [file ...]    # no files => all <vault>/tasks/*/*.md
set -u
vault="${1:-}"; [ -n "$vault" ] || { echo "tt-quote-fm: vault required" >&2; exit 2; }
shift
files=( "$@" )
[ "${#files[@]}" -gt 0 ] || files=( "$vault"/tasks/*/*.md )
command -v perl >/dev/null 2>&1 || { echo "tt-quote-fm: perl not found; skipping" >&2; exit 0; }

n=0
for f in "${files[@]}"; do
  [ -e "$f" ] || continue
  before="$(cat "$f")"
  perl -i -ne '
    BEGIN { $fm = 0; $done = 0 }
    if ($_ =~ /^---\s*$/ && !$fm && !$done) { $fm = 1; print; next }
    if ($_ =~ /^---\s*$/ &&  $fm)          { $fm = 0; $done = 1; print; next }
    if ($fm && /^([A-Za-z_][A-Za-z0-9_]*): (.+?)[ \t\r]*$/) {
      my ($k, $v) = ($1, $2);
      # \x22=" \x27=apostrophe \x5b=[ \x7b={  → leave already-quoted / structured values alone
      if ($v ne "" && $v !~ /^[\x22\x27\x5b\x7b]/ && ($v =~ /: / || $v =~ /:$/)) {
        $v =~ s/\\/\\\\/g; $v =~ s/\x22/\\\x22/g;
        print "$k: \x22$v\x22\n"; next;
      }
    }
    print;
  ' "$f"
  [ "$(cat "$f")" != "$before" ] && { echo "  quoted: $(basename "$f")"; n=$((n+1)); }
done
echo "tt-quote-fm: $n file(s) quoted"
