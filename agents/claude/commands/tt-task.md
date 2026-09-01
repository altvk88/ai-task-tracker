---
description: Создать задачу task-tracker
argument-hint: "<заголовок> PROJECT=<slug> [PRIORITY=high|medium|low] [EFFORT=2h] [DEPENDS_ON=ID,ID]"
---

Разбери $ARGUMENTS: первое слово (или фраза в кавычках) — заголовок задачи,
дальше пары `КЛЮЧ=значение` (`PROJECT`, `PRIORITY`, `EFFORT`, `DEPENDS_ON`).
`PROJECT` обязателен.

Выполни:

```
tt new --project "<PROJECT>" --title "<заголовок>" \
  [--priority <PRIORITY>] [--effort <EFFORT>] [--depends-on <DEPENDS_ON>]
```

`tt` сам берёт vault из файла настроек (`tt config show` покажет, откуда),
генерирует ID и slug по `projects/<PROJECT>.md`, создаёт файл задачи из
шаблона и ставит `status: ready` (или `backlog`, если указан `DEPENDS_ON`).

Выведи пользователю ID и путь созданного файла из вывода команды.
