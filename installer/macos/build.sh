#!/usr/bin/env bash
# Собирает поставку tt для macOS: бинарники darwin/arm64 и darwin/amd64 плюс
# staging-каталог со скриптами установки и плагином Obsidian.
#
# Кросс-компиляция бинарника работает на любой ОС (Go сам решает GOOS/GOARCH,
# CGO не используется нигде в проекте) — этот скрипт можно запускать и с
# Windows/Linux. Чего кросс-компиляция НЕ даёт — это .pkg/.dmg: pkgbuild,
# productbuild и hdiutil существуют только на самой macOS. Поэтому здесь
# собирается не пакет, а каталог со скриптом установки и голыми бинарниками —
# см. installer/macos/install.sh и README.md, раздел про установку на macOS.
#
# Порядок сборки веб-бандла и плагина — тот же, что и для Windows-установщика
# (см. README.md): оба вшиваются/копируются как готовые артефакты, поэтому
# собираются раньше самого tt.
set -eu

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
DIST="$HERE/dist"
VERSION="1.0.0"

cd "$ROOT/web" && npm install && npm run build
cd "$ROOT/obsidian-plugin" && npm install && npm run build
cd "$ROOT"

rm -f "$DIST/tt-darwin-arm64" "$DIST/tt-darwin-amd64"
mkdir -p "$DIST/obsidian-plugin"

for arch in arm64 amd64; do
  echo "==> сборка darwin/$arch"
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go build -o "$DIST/tt-darwin-$arch" ./cmd/tt
done

cp "$HERE/install.sh" "$HERE/uninstall.sh" "$HERE/com.tt.serve.plist.template" "$DIST/"
cp "$ROOT/obsidian-plugin/main.js" "$ROOT/obsidian-plugin/manifest.json" \
   "$ROOT/obsidian-plugin/styles.css" "$DIST/obsidian-plugin/"
cp "$ROOT/internal/model/schema_default.json" "$DIST/obsidian-plugin/schema.json"
cp "$ROOT/AGENT-INTEGRATION.md" "$DIST/"
chmod +x "$DIST/install.sh" "$DIST/uninstall.sh"

for arch in arm64 amd64; do
  tar -C "$DIST" -czf "$HERE/tt-darwin-$arch-$VERSION.tar.gz" \
    "tt-darwin-$arch" install.sh uninstall.sh com.tt.serve.plist.template \
    obsidian-plugin AGENT-INTEGRATION.md
done

echo "Готово: $DIST/ и архивы installer/macos/tt-darwin-{arm64,amd64}-$VERSION.tar.gz"
echo "Дальше — на самой macOS: распаковать нужный архив и запустить ./install.sh"
