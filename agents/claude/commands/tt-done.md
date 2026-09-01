---
description: Завершить задачу task-tracker вручную
argument-hint: "<ID> [RESULT=\"текст\"]"
---

Заверши задачу `$1` в task-tracker. `RESULT` из $ARGUMENTS — краткий итог,
если передан.

`tt done [--result "<RESULT>"] "$1"` — ставит `status: done` и `completed:`
на сегодня, снимает блок `claim:` и замок, и сам переводит в `ready` каждую
задачу, у которой все `blocked_by` стали `done`.

Допиши строку в `## Log` файла задачи с итогом и, если в выводе `tt done`
были промоутнутые ID, перечисли их пользователю.
