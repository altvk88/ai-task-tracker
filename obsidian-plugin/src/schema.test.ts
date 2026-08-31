import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { parseSchema, normalize, lane, clearsClaimOn, setsCompletedOn, setsReadyAtOn } from "./schema.ts";

const RAW = JSON.stringify({
  version: 1,
  statuses: [
    { id: "backlog", lane: "Backlog" },
    { id: "ready", lane: "Ready", agentPickable: true },
    { id: "in-progress", lane: "In Progress" },
    { id: "done", lane: "Done" },
    { id: "cancelled", lane: "Canceled" },
  ],
  aliases: { in_progress: "in-progress", canceled: "cancelled" },
  clearsClaim: ["ready", "done", "cancelled", "backlog"],
  setsCompleted: ["done"],
  setsReadyAt: ["ready"],
  promoteFrom: "backlog",
  promoteTo: "ready",
});

test("нормализация терпит регистр, пробелы и алиасы", () => {
  const s = parseSchema(RAW);
  assert.deepEqual(normalize(s, "ready"), { id: "ready", known: true });
  assert.deepEqual(normalize(s, "in_progress"), { id: "in-progress", known: true });
  assert.deepEqual(normalize(s, "  IN_PROGRESS  "), { id: "in-progress", known: true });
  assert.deepEqual(normalize(s, "canceled"), { id: "cancelled", known: true });
  assert.deepEqual(normalize(s, "выдумка"), { id: "выдумка", known: false });
  assert.deepEqual(normalize(s, ""), { id: "", known: false });
});

test("лейн неизвестного статуса — сам статус, чтобы таска не пропала", () => {
  const s = parseSchema(RAW);
  assert.equal(lane(s, "in-progress"), "In Progress");
  assert.equal(lane(s, "выдумка"), "выдумка");
});

test("правила переходов читаются из схемы", () => {
  const s = parseSchema(RAW);
  assert.equal(clearsClaimOn(s, "done"), true);
  assert.equal(clearsClaimOn(s, "in-progress"), false);
  assert.equal(setsCompletedOn(s, "done"), true);
  assert.equal(setsCompletedOn(s, "ready"), false);
  assert.equal(setsReadyAtOn(s, "ready"), true);
});

test("битая схема даёт внятную ошибку, а не молчаливый пустой список", () => {
  assert.throws(() => parseSchema("{не json"), /схем/i);
  assert.throws(() => parseSchema(JSON.stringify({ statuses: [] })), /статус/i);
});

test("реальная схема vault разбирается и даёт десять статусов", () => {
  const raw = readFileSync("D:/task tracker/.task-tracker/schema.json", "utf8");
  const s = parseSchema(raw);
  assert.equal(s.statuses.length, 10, "в схеме vault десять статусов");
  assert.equal(lane(s, "in-progress"), "In Progress");
  assert.equal(normalize(s, "in_progress").id, "in-progress");
});
