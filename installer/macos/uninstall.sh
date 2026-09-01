#!/usr/bin/env bash
# Удаление tt с macOS. Снимает launchd-агента и файлы программы; vault,
# файл настроек и плагин внутри vault — данные пользователя, их не трогает,
# как и деинсталлятор Windows (installer/tt.iss, CurUninstallStepChanged).
set -eu

INSTALL_DIR="$HOME/.local/bin"
PLIST="$HOME/Library/LaunchAgents/com.tt.serve.plist"

if [ -f "$PLIST" ]; then
  launchctl unload "$PLIST" >/dev/null 2>&1 || true
  rm -f "$PLIST"
  echo "Автозапуск снят."
fi

if [ -f "$INSTALL_DIR/tt" ]; then
  rm -f "$INSTALL_DIR/tt"
  echo "$INSTALL_DIR/tt удалён."
fi

cat <<'EOF'

Vault, файл настроек (~/Library/Application Support/tt/config.json) и плагин
внутри vault не тронуты — это данные пользователя, а не файлы программы.

Если install.sh добавлял строку в PATH (профиль ~/.zprofile или
~/.bash_profile, помеченную "# added by tt installer"), удали её вручную —
автоматически строка не убирается: рядом в файле профиля могли появиться
твои собственные правки, трогать файл без спроса небезопасно.
EOF
