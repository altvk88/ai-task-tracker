// Общий контракт правил флоу. Тот же файл .task-tracker/schema.json читает Go-бинарник,
// поэтому лейны и переходы совпадают на обеих досках по построению, а не по внимательности.
// Модуль не знает про файловую систему: текст ему передаёт вызывающий код.

export type Status = {
  id: string;
  lane: string;
  agentPickable?: boolean;
};

export type Schema = {
  version: number;
  statuses: Status[];
  aliases: Record<string, string>;
  clearsClaim: string[];
  setsCompleted: string[];
  setsReadyAt: string[];
  promoteFrom: string;
  promoteTo: string;
};

export function parseSchema(raw: string): Schema {
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch (e) {
    throw new Error(`схема не разбирается: ${(e as Error).message}`);
  }
  const s = data as Partial<Schema>;
  if (!Array.isArray(s.statuses) || s.statuses.length === 0) {
    throw new Error("в схеме нет ни одного статуса");
  }
  return {
    version: s.version ?? 1,
    statuses: s.statuses,
    aliases: s.aliases ?? {},
    clearsClaim: s.clearsClaim ?? [],
    setsCompleted: s.setsCompleted ?? [],
    setsReadyAt: s.setsReadyAt ?? [],
    promoteFrom: s.promoteFrom ?? "backlog",
    promoteTo: s.promoteTo ?? "ready",
  };
}

/** Приводит написание статуса к каноническому и сообщает, известен ли он схеме. */
export function normalize(s: Schema, status: string): { id: string; known: boolean } {
  const v = (status ?? "").trim().toLowerCase();
  if (v === "") return { id: "", known: false };
  const canon = s.aliases[v] ?? v;
  const found = s.statuses.some((st) => st.id === canon);
  return found ? { id: canon, known: true } : { id: (status ?? "").trim(), known: false };
}

/** Подпись лейна; для неизвестного статуса — сам статус, чтобы таска была видна. */
export function lane(s: Schema, id: string): string {
  return s.statuses.find((st) => st.id === id)?.lane ?? id;
}

export const clearsClaimOn = (s: Schema, id: string): boolean => s.clearsClaim.includes(id);
export const setsCompletedOn = (s: Schema, id: string): boolean => s.setsCompleted.includes(id);
export const setsReadyAtOn = (s: Schema, id: string): boolean => s.setsReadyAt.includes(id);
