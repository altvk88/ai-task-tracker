import { test } from "node:test";
import assert from "node:assert/strict";
import { parseSchema } from "./schema.ts";
import { toTask, buildLanes } from "./lanes.ts";

const SCHEMA = parseSchema(JSON.stringify({
  statuses: [
    { id: "backlog", lane: "Backlog" },
    { id: "ready", lane: "Ready" },
    { id: "in-progress", lane: "In Progress" },
    { id: "done", lane: "Done" },
  ],
  aliases: { in_progress: "in-progress" },
  clearsClaim: [], setsCompleted: [], setsReadyAt: [],
  promoteFrom: "backlog", promoteTo: "ready",
}));

test("фронтматтер превращается в таску, даты остаются строками", () => {
  const t = toTask("tasks/alpha/one.md", {
    id: "ALP-1", title: "первая", status: "ready", project: "alpha",
    priority: "high", effort: "2h", created: "2026-08-01", blocked_by: ["ALP-9"],
  });
  assert.equal(t.id, "ALP-1");
  assert.equal(t.status, "ready");
  assert.deepEqual(t.blockedBy, ["ALP-9"]);
  assert.equal(t.path, "tasks/alpha/one.md");
  assert.equal(t.claimed, false);
});

test("claim читается и блоком, и скаляром — в vault есть обе формы", () => {
  const block = toTask("a.md", { id: "A-1", claim: { agent: "claude", host: "h" } });
  assert.equal(block.claimed, true);
  assert.equal(block.claimAgent, "claude");

  const scalar = toTask("b.md", { id: "A-2", claim: "claude 2026-08-04" });
  assert.equal(scalar.claimed, true);
  assert.equal(scalar.claimAgent, "", "у скалярного claim владелец неизвестен");
  assert.equal(scalar.claimRaw, "claude 2026-08-04");

  const empty = toTask("c.md", { id: "A-3", claim: null });
  assert.equal(empty.claimed, false);

  const blank = toTask("d.md", { id: "A-4", claim: "   " });
  assert.equal(blank.claimed, false, "пробельный скаляр — не занятость");
});

test("blocked_by терпит скаляр и отсутствие", () => {
  assert.deepEqual(toTask("a.md", { id: "A", blocked_by: "B-1" }).blockedBy, ["B-1"]);
  assert.deepEqual(toTask("a.md", { id: "A" }).blockedBy, []);
  assert.deepEqual(toTask("a.md", { id: "A", blocked_by: null }).blockedBy, []);
  assert.deepEqual(toTask("a.md", { id: "A", blocked_by: [] }).blockedBy, []);
});

test("лейны идут в порядке схемы, пустые сохраняются", () => {
  const tasks = [
    toTask("a.md", { id: "A-1", status: "ready", title: "раз" }),
    toTask("b.md", { id: "A-2", status: "in_progress", title: "два" }),
    toTask("c.md", { id: "A-3", status: "ready", title: "три" }),
  ];
  const lanes = buildLanes(SCHEMA, tasks);
  assert.deepEqual(lanes.map((l) => l.lane), ["Backlog", "Ready", "In Progress", "Done"]);
  assert.equal(lanes[0].tasks.length, 0, "пустой лейн обязан остаться на доске");
  assert.deepEqual(lanes[1].tasks.map((t) => t.id), ["A-1", "A-3"]);
  assert.deepEqual(lanes[2].tasks.map((t) => t.id), ["A-2"], "in_progress попал в In Progress");
});

test("таска с неизвестным статусом не теряется, а получает свой лейн в конце", () => {
  const lanes = buildLanes(SCHEMA, [toTask("a.md", { id: "A-1", status: "выдумка" })]);
  const extra = lanes.find((l) => l.lane === "выдумка");
  assert.ok(extra, "неизвестный статус обязан дать отдельный лейн");
  assert.equal(extra.tasks.length, 1);
  assert.equal(lanes[lanes.length - 1].lane, "выдумка", "и он идёт последним");
});

test("две таски с одним неизвестным статусом попадают в один лейн", () => {
  const lanes = buildLanes(SCHEMA, [
    toTask("a.md", { id: "A-1", status: "выдумка" }),
    toTask("b.md", { id: "A-2", status: "выдумка" }),
  ]);
  assert.equal(lanes.filter((l) => l.lane === "выдумка").length, 1, "лейн не должен дублироваться");
  assert.equal(lanes[lanes.length - 1].tasks.length, 2);
});

test("фильтр по проекту и поиск по ID и заголовку", () => {
  const tasks = [
    toTask("a.md", { id: "ALP-1", status: "ready", title: "починить логин", project: "alpha" }),
    toTask("b.md", { id: "BET-1", status: "ready", title: "починить выход", project: "beta" }),
  ];
  assert.equal(buildLanes(SCHEMA, tasks, { project: "beta" })[1].tasks.length, 1);
  assert.equal(buildLanes(SCHEMA, tasks, { query: "логин" })[1].tasks.length, 1);
  assert.equal(buildLanes(SCHEMA, tasks, { query: "alp-1" })[1].tasks.length, 1, "поиск по ID без регистра");
  assert.equal(buildLanes(SCHEMA, tasks, { query: "починить" })[1].tasks.length, 2);
});
