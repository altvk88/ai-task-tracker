# Active Context

## Current focus

Этап 1 — ядро: `model`, `vault`, `cli` и команды `tt list` / `tt set` / `tt doctor`.
Этап ничего не удаляет из vault: доски, вотчер и bash-скрипты остаются работать.

План: `D:/task tracker/docs/superpowers/plans/2026-08-31-tt-core-phase-1.md`
Спек: `D:/task tracker/docs/superpowers/specs/2026-08-31-local-task-tracker-ui-design.md`

## Recent changes

- **2026-08-31** — TT-001: установлен Go 1.27.0, создан модуль
  `github.com/alkulagin-creator/tt`, каталоги `cmd/tt` и `internal/{model,vault,cli}`,
  подключён `gopkg.in/yaml.v3`, ветка `main`.
- **2026-08-31** — `.gitattributes` с правилом `internal/vault/testdata/** -text`:
  у пользователя `core.autocrlf=true`, и без этого git перепишет переводы строк в фикстурах,
  а тесты на CRLF и BOM станут фикцией.
- **2026-08-31** — заведены `memory-bank/` и `rules/` (требование глобального CLAUDE.md;
  в плане этапа этого шага не было).

## Next steps

После TT-001 открываются четыре независимые задачи — их можно вести параллельно:

- **TT-002** чтение фронтматтера (`Split`, `Parse`, `model.Task`)
- **TT-003** схема статусов и лейнов
- **TT-005** кавычки в значениях фронтматтера
- **TT-007** замок через mkdir

Дальше по графу: TT-004 (scan) ← TT-002; TT-006 (писатель) ← TT-005;
TT-008 (guard'ы) ← TT-002+TT-003; TT-009 (CLI+list) ← TT-003+TT-004;
TT-010 (set) ← TT-006+007+008+009; TT-011 (doctor) ← TT-006+009; TT-012 ← TT-011;
TT-013 (приёмка на живом vault) ← TT-010+TT-012.
