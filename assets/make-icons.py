"""Пересобирает иконки из мастер-файла assets/tt.png.

Запуск (нужен Pillow):  python assets/make-icons.py

Мастер — квадратный PNG с прозрачным фоном. Из него получаются:

  assets/tt.ico            полный набор 16..256 — для установщика и tt.exe
  web/public/favicon.ico   только 16..48 — вкладка браузера
  web/public/apple-touch-icon.png

Почему у веба свой .ico: браузер скачивает файл целиком, и полный набор
с 256-м размером весит 78 КБ на каждое открытие доски против 9 КБ у урезанного.

После пересборки .ico иконку в tt.exe нужно перевшить, иначе бинарник
останется со старой:

  go install github.com/akavel/rsrc@latest
  rsrc -ico assets/tt.ico -arch amd64 -o cmd/tt/rsrc_windows.syso
"""

import pathlib

from PIL import Image

root = pathlib.Path(__file__).resolve().parent.parent
master = Image.open(root / "assets" / "tt.png")

master.save(root / "assets" / "tt.ico", sizes=[(s, s) for s in (16, 24, 32, 48, 64, 128, 256)])
master.save(root / "web" / "public" / "favicon.ico", sizes=[(s, s) for s in (16, 24, 32, 48)])
master.resize((180, 180), Image.LANCZOS).save(root / "web" / "public" / "apple-touch-icon.png")

print("готово: assets/tt.ico, web/public/favicon.ico, web/public/apple-touch-icon.png")
