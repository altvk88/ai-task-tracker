---
description: Создать задачу task-tracker
argument-hint: "\"<заголовок>\" PROJECT=<slug> [PRIORITY=high|medium|low] [EFFORT=2h] [DEPENDS_ON=ID,ID]"
---

Создай задачу task-tracker. Заголовок: $1. Поля: PROJECT=$PROJECT,
PRIORITY=$PRIORITY, EFFORT=$EFFORT, DEPENDS_ON=$DEPENDS_ON (всё, что задано).
`PROJECT` обязателен.

Выполни:

```
tt new --project "$PROJECT" --title "$1" \
  [--priority "$PRIORITY"] [--effort "$EFFORT"] [--depends-on "$DEPENDS_ON"]
```

`tt` сам берёт vault из файла настроек (`tt config show` покажет, откуда),
генерирует ID и slug по `projects/$PROJECT.md`, создаёт файл задачи из
шаблона и ставит `status: ready` (или `backlog`, если указан `DEPENDS_ON`).

Выведи пользователю ID и путь созданного файла из вывода команды.
