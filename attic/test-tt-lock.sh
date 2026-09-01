#!/usr/bin/env bash
set -u
. "$(dirname "$0")/lib/assert.sh"
LIB="$(cd "$(dirname "$0")/../claude-global/lib" && pwd)"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
vault="$tmp/v"; mkdir -p "$vault"

bash "$LIB/tt-lock.sh" acquire "$vault" P-001; assert_success $? "первый acquire успешен"
bash "$LIB/tt-lock.sh" is-locked "$vault" P-001; assert_success $? "is-locked видит лок"
bash "$LIB/tt-lock.sh" acquire "$vault" P-001; assert_failure $? "повторный acquire падает (взаимоисключение)"
bash "$LIB/tt-lock.sh" release "$vault" P-001; assert_success $? "release успешен"
bash "$LIB/tt-lock.sh" is-locked "$vault" P-001; assert_failure $? "после release лока нет"
bash "$LIB/tt-lock.sh" acquire "$vault" P-001; assert_success $? "acquire снова успешен после release"

finish
