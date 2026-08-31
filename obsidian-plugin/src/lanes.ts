// Превращение фронтматтера в таски и раскладка их по лейнам.
// Модуль принимает простые объекты, а не файлы Obsidian, поэтому тестируется без него.
import { type Schema, normalize, lane } from "./schema.ts";

export type Task = {
  id: string;
  title: string;
  status: string;
  project: string;
  priority: string;
  effort: string;
  path: string;
  blockedBy: string[];
  claimed: boolean;
  claimAgent: string;
  claimRaw: string;
};

export type Lane = { lane: string; status: string; tasks: Task[] };

export type Filter = { project?: string; query?: string };

const str = (v: unknown): string => (typeof v === "string" ? v : v == null ? "" : String(v));

/** Приводит фронтматтер к таске. Терпит обе исторические формы claim и blocked_by. */
export function toTask(path: string, fm: Record<string, unknown> | null | undefined): Task {
  const f = fm ?? {};
  const raw = f["blocked_by"];
  const blockedBy = Array.isArray(raw)
    ? raw.map(str).filter(Boolean)
    : str(raw) ? [str(raw)] : [];

  let claimed = false;
  let claimAgent = "";
  let claimRaw = "";
  const claim = f["claim"];
  if (typeof claim === "string" && claim.trim() !== "") {
    claimed = true;
    claimRaw = claim;
  } else if (claim && typeof claim === "object") {
    const agent = str((claim as Record<string, unknown>)["agent"]);
    if (agent !== "") {
      claimed = true;
      claimAgent = agent;
    }
  }

  return {
    id: str(f["id"]),
    title: str(f["title"]),
    status: str(f["status"]),
    project: str(f["project"]),
    priority: str(f["priority"]),
    effort: str(f["effort"]),
    path,
    blockedBy,
    claimed,
    claimAgent,
    claimRaw,
  };
}

/**
 * Раскладывает таски по лейнам в порядке схемы. Пустые лейны сохраняются: доска
 * с исчезающими колонками прыгает под руками. Неизвестные статусы получают свои
 * лейны в конце — так таска видна, а не теряется молча.
 */
export function buildLanes(schema: Schema, tasks: Task[], filter: Filter = {}): Lane[] {
  const q = (filter.query ?? "").trim().toLowerCase();
  const visible = tasks.filter((t) => {
    if (filter.project && t.project !== filter.project) return false;
    if (q === "") return true;
    return t.id.toLowerCase().includes(q) || t.title.toLowerCase().includes(q);
  });

  const lanes: Lane[] = schema.statuses.map((st) => ({ lane: st.lane, status: st.id, tasks: [] }));
  const byStatus = new Map(lanes.map((l) => [l.status, l]));

  for (const t of visible) {
    const { id } = normalize(schema, t.status);
    const known = byStatus.get(id);
    if (known) {
      known.tasks.push(t);
      continue;
    }
    let extra = lanes.find((l) => l.status === id);
    if (!extra) {
      extra = { lane: lane(schema, id), status: id, tasks: [] };
      lanes.push(extra);
    }
    extra.tasks.push(t);
  }
  return lanes;
}
