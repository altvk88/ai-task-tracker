import { test } from "node:test";
import assert from "node:assert/strict";
import { parseSchema } from "./schema.ts";
import { toTask, type Task } from "./lanes.ts";
import { unresolvedBlockers, checkTransition } from "./guards.ts";

const SCHEMA = parseSchema(JSON.stringify({
  statuses: [
    { id: "backlog", lane: "Backlog" },
    { id: "ready", lane: "Ready" },
    { id: "in-progress", lane: "In Progress" },
    { id: "done", lane: "Done" },
    { id: "cancelled", lane: "Canceled" },
  ],
  aliases: { in_progress: "in-progress" },
  clearsClaim: [], setsCompleted: [], setsReadyAt: [],
  promoteFrom: "backlog", promoteTo: "ready",
}));

const index = (...tasks: Task[]) => new Map(tasks.map((t) => [t.id, t]));

const ALL = index(
  toTask("a.md", { id: "A-1", status: "done" }),
  toTask("b.md", { id: "A-2", status: "in-progress" }),
  toTask("c.md", { id: "A-3", status: "cancelled" }),
);

test("блокер держит, пока не done и не cancelled", () => {
  assert.deepEqual(unresolvedBlockers(toTask("x.md", { id: "X" }), ALL), []);
  assert.deepEqual(unresolvedBlockers(toTask("x.md", { id: "X", blocked_by: ["A-1"] }), ALL), []);
  assert.deepEqual(unresolvedBlockers(toTask("x.md", { id: "X", blocked_by: ["A-3"] }), ALL), []);
  assert.deepEqual(unresolvedBlockers(toTask("x.md", { id: "X", blocked_by: ["A-2"] }), ALL), ["A-2"]);
  assert.deepEqual(unresolvedBlockers(toTask("x.md", { id: "X", blocked_by: ["A-1", "A-2"] }), ALL), ["A-2"]);
  assert.deepEqual(
    unresolvedBlockers(toTask("x.md", { id: "X", blocked_by: ["A-99"] }), ALL),
    ["A-99"],
    "несуществующий блокер держит: молча разблокировать из-за опечатки нельзя",
  );
});

test("нельзя взять в работу таску с живым блокером", () => {
  const t = toTask("x.md", { id: "X", status: "ready", blocked_by: ["A-2"] });
  const err = checkTransition(SCHEMA, t, "in-progress", ALL, false);
  assert.ok(err && err.includes("A-2"), `ошибка обязана называть блокер, получено: ${err}`);
});

test("с закрытым блокером взять можно", () => {
  const t = toTask("x.md", { id: "X", status: "ready", blocked_by: ["A-1"] });
  assert.equal(checkTransition(SCHEMA, t, "in-progress", ALL, false), null);
});

test("историческое написание целевого статуса принимается", () => {
  const t = toTask("x.md", { id: "X", status: "ready" });
  assert.equal(checkTransition(SCHEMA, t, "in_progress", ALL, false), null);
});

test("чужой claim не отбирается", () => {
  const t = toTask("x.md", { id: "X", status: "ready", claim: { agent: "other" } });
  const err = checkTransition(SCHEMA, t, "in-progress", ALL, false);
  assert.ok(err && err.includes("other"), `ошибка обязана назвать владельца, получено: ${err}`);
});

test("скалярный claim — занята неизвестно кем, текст показывается", () => {
  const t = toTask("x.md", { id: "X", status: "ready", claim: "avk @ 2026-06-29" });
  const err = checkTransition(SCHEMA, t, "in-progress", ALL, false);
  assert.ok(err && err.includes("avk @ 2026-06-29"), `получено: ${err}`);
});

test("занятый замок мешает взять в работу", () => {
  const t = toTask("x.md", { id: "X", status: "ready" });
  const err = checkTransition(SCHEMA, t, "in-progress", ALL, true);
  assert.ok(err && /замок|занята/i.test(err), `получено: ${err}`);
});

test("уход из работы разрешён всегда — иначе не разблокировать залипшую таску", () => {
  const t = toTask("x.md", { id: "X", status: "in-progress", claim: { agent: "other" } });
  assert.equal(checkTransition(SCHEMA, t, "ready", ALL, true), null);
});

test("блокер не мешает уйти в backlog", () => {
  const t = toTask("x.md", { id: "X", status: "ready", blocked_by: ["A-2"] });
  assert.equal(checkTransition(SCHEMA, t, "backlog", ALL, false), null);
});

test("неизвестный целевой статус отклоняется", () => {
  const t = toTask("x.md", { id: "X", status: "ready" });
  assert.ok(checkTransition(SCHEMA, t, "выдумка", ALL, false));
});
