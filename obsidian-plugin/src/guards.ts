// Зеркало internal/model/guards.go: стерегутся ровно два случая — взятие в работу
// таски с живыми блокерами и захват чужой. Полной матрицы переходов нет сознательно:
// перетаскивание карточки между лейнами законно, доска не должна спорить с человеком.
import { type Schema, normalize } from "./schema.ts";
import { type Task } from "./lanes.ts";

const RESOLVED = new Set(["done", "cancelled"]);

/** ID блокеров, которые всё ещё держат таску. Несуществующий блокер считается держащим. */
export function unresolvedBlockers(task: Task, byId: Map<string, Task>): string[] {
  return task.blockedBy.filter((id) => {
    const blocker = byId.get(id);
    return !blocker || !RESOLVED.has(blocker.status);
  });
}

/**
 * Возвращает текст ошибки или null, если переход разрешён.
 * `locked` — есть ли файл замка `.locks/<ID>.lock`; проверка совещательная,
 * атомарности от Obsidian не требуется и не предполагается.
 */
export function checkTransition(
  schema: Schema,
  task: Task,
  to: string,
  byId: Map<string, Task>,
  locked: boolean,
): string | null {
  const { id: canon, known } = normalize(schema, to);
  if (!known) return `неизвестный статус «${to}»`;
  if (canon !== "in-progress") return null;

  const blockers = unresolvedBlockers(task, byId);
  if (blockers.length > 0) return `таска ${task.id} заблокирована: ${blockers.join(", ")}`;

  if (task.claimed) {
    if (task.claimAgent === "") return `таска ${task.id} занята (нераспознанный claim: ${task.claimRaw})`;
    return `таска ${task.id} занята агентом ${task.claimAgent}`;
  }
  if (locked) return `таска ${task.id} занята: стоит замок .locks/${task.id}.lock`;
  return null;
}
