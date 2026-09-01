#!/usr/bin/env bash
# Установка tt для macOS. Запускать из распакованного архива
# tt-darwin-<arch>-<version>.tar.gz (installer/macos/build.sh собирает его
# из Windows/Linux кросс-компиляцией — сам .pkg/.dmg собрать без macOS
# нельзя, поэтому поставка — архив с бинарником и этот скрипт, а не
# графический мастер, см. README.md).
#
# Делает то же, что установщик под Windows (installer/tt.iss): кладёт
# бинарник в PATH, спрашивает vault и порт и пишет их через "tt config set",
# предлагает "tt scaffold" при пустом vault, заводит автозапуск (launchd,
# аналог ярлыка в автозагрузке на Windows) и опционально ставит плагин
# Obsidian. Права администратора не нужны — всё per-user, как и на Windows
# (PrivilegesRequired=lowest).
set -eu

HERE="$(cd "$(dirname "$0")" && pwd)"

case "$(uname -m)" in
  arm64) BIN_SRC="$HERE/tt-darwin-arm64" ;;
  x86_64) BIN_SRC="$HERE/tt-darwin-amd64" ;;
  *) echo "Неизвестная архитектура: $(uname -m)" >&2; exit 1 ;;
esac
[ -f "$BIN_SRC" ] || {
  echo "Не найден $BIN_SRC — запускай install.sh из распакованного архива целиком," \
    "а не отдельно от бинарников." >&2
  exit 1
}

INSTALL_DIR="$HOME/.local/bin"
mkdir -p "$INSTALL_DIR"
cp "$BIN_SRC" "$INSTALL_DIR/tt"
chmod +x "$INSTALL_DIR/tt"
TT="$INSTALL_DIR/tt"
echo "tt установлен в $TT"

# ---- PATH: как HKCU\Environment на Windows, только через профиль оболочки.
# ~/.local/bin не входит в PATH «из коробки» на чистой macOS. Правим то, что
# реально подхватится: .zprofile для zsh (умолчание с Catalina) и
# .bash_profile, если сессия сейчас в bash. Метка нужна, чтобы не дублировать
# строку при повторном запуске установщика. ----
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    PROFILE="$HOME/.zprofile"
    [ -n "${BASH_VERSION:-}" ] && PROFILE="$HOME/.bash_profile"
    MARK="# added by tt installer"
    if ! grep -qs "$MARK" "$PROFILE" 2>/dev/null; then
      { printf '\nexport PATH="%s:$PATH" %s\n' "$INSTALL_DIR" "$MARK"; } >> "$PROFILE"
      echo "Добавлено в PATH через $PROFILE — новые окна терминала подхватят сами," \
        "в текущем набери: export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
    ;;
esac

# ---- Gatekeeper: аналог предупреждения про SmartScreen в README для Windows.
# Актуально, только если бинарник скачан браузером (это ставит атрибут
# карантина); при локальной сборке из исходников (go build) атрибута нет и
# предупреждения не будет. ----
cat <<EOF

Если "$TT" скачан из интернета (а не собран локально из исходников),
первый запуск может завершиться сообщением macOS: "«tt» нельзя открыть,
так как его автор не может быть подтверждён". Это Gatekeeper, а не признак
вируса — бинарник не подписан и не нотаризован (сертификат Apple Developer
Program стоит денег и не куплен для этого проекта). Как обойти:
  1. Системные настройки → Конфиденциальность и безопасность → внизу
     страницы появится кнопка "Всё равно открыть" после первой попытки, или
  2. в терминале снять карантин:  xattr -d com.apple.quarantine "$TT"

EOF

# ---- vault и порт: как страницы мастера на Windows, только вопросами в
# терминале. Пишутся через "tt config set", а не правкой JSON напрямую —
# как и требует задача. ----
read -rp "Путь к vault таск-трекера: " VAULT
VAULT="${VAULT/#\~/$HOME}"
if [ ! -d "$VAULT/tasks" ]; then
  read -rp "В \"$VAULT\" нет каталога tasks — это не похоже на vault. Создать структуру? [y/N] " ans
  case "$ans" in
    y|Y) "$TT" scaffold --vault "$VAULT" ;;
    *) echo "Установка прервана: vault не задан." >&2; exit 1 ;;
  esac
fi

read -rp "Порт веб-доски tt serve [4173]: " PORT
PORT="${PORT:-4173}"

"$TT" config set --vault "$VAULT" --port "$PORT"
echo "Настройки сохранены (tt config show, чтобы проверить)."

# ---- автозапуск: launchd вместо windows-ярлыка в автозагрузке. LaunchAgent
# (не LaunchDaemon) — работает в сессии пользователя без sudo, как и
# требуется. Заводится только с согласия. ----
read -rp "Запускать \"tt serve\" автоматически при входе в систему? [y/N] " AUTOSTART
if [ "$AUTOSTART" = "y" ] || [ "$AUTOSTART" = "Y" ]; then
  LOG_DIR="$HOME/Library/Logs/tt"
  PLIST_DIR="$HOME/Library/LaunchAgents"
  mkdir -p "$LOG_DIR" "$PLIST_DIR"
  PLIST="$PLIST_DIR/com.tt.serve.plist"
  sed -e "s#__TT_BIN__#$TT#" -e "s#__LOG_DIR__#$LOG_DIR#" \
    "$HERE/com.tt.serve.plist.template" > "$PLIST"
  launchctl unload "$PLIST" >/dev/null 2>&1 || true
  launchctl load -w "$PLIST"
  echo "Автозапуск настроен: $PLIST (логи — $LOG_DIR)"
fi

# ---- плагин Obsidian: опция, по умолчанию выключена — не у всех вообще
# стоит Obsidian, как и на Windows. schema.json не перезаписывается, если
# уже существует — его могли подстроить под свой набор лейнов. ----
read -rp "Установить плагин Obsidian tt-board в этот vault? [y/N] " PLUGIN
if [ "$PLUGIN" = "y" ] || [ "$PLUGIN" = "Y" ]; then
  PLUGIN_DIR="$VAULT/.obsidian/plugins/tt-board"
  DATA_DIR="$VAULT/.task-tracker"
  mkdir -p "$PLUGIN_DIR" "$DATA_DIR"
  cp "$HERE/obsidian-plugin/main.js" "$HERE/obsidian-plugin/manifest.json" \
    "$HERE/obsidian-plugin/styles.css" "$PLUGIN_DIR/"
  [ -f "$DATA_DIR/schema.json" ] || cp "$HERE/obsidian-plugin/schema.json" "$DATA_DIR/schema.json"
  echo "Плагин установлен в $PLUGIN_DIR"
fi

# ---- инструкция по подключению ИИ-агентов (Claude Code, Codex) — TT-058.
# Файл кладётся рядом с бинарником, чтобы вернуться к нему после установки;
# логика подключения самих агентов сюда не дублируется — она в самом файле
# и в agents/claude, agents/codex репозитория. ----
AGENT_DOC="$INSTALL_DIR/AGENT-INTEGRATION.md"
if [ -f "$HERE/AGENT-INTEGRATION.md" ]; then
  cp "$HERE/AGENT-INTEGRATION.md" "$AGENT_DOC"
fi

echo
echo "Готово. Проверить: tt config show"

if [ -f "$AGENT_DOC" ]; then
  cat <<EOF

Хочешь, чтобы задачи в этом vault выполнял ИИ-агент? Инструкция по
подключению Claude Code и Codex — в файле:
  $AGENT_DOC
EOF
fi
