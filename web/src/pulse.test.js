import { test } from 'node:test';
import assert from 'node:assert/strict';
import { computePulse } from './pulse.js';

// Схема с легаси-алиасами — как в internal/model/schema.go: "In Progress" и
// "in_progress" должны считаться как in-progress, а не как неизвестный статус.
const SCHEMA = {
  statuses: [
    { id: 'backlog', lane: 'Backlog' },
    { id: 'ready', lane: 'Ready' },
    { id: 'in-progress', lane: 'In Progress' },
    { id: 'needs-input', lane: 'Needs Input' },
    { id: 'done', lane: 'Done' },
  ],
  aliases: { 'in progress': 'in-progress', in_progress: 'in-progress' },
};

test('подсчёт по статусам не зависит от исторических написаний', () => {
  const tasks = [
    { id: 'A-1', status: 'In Progress', project: 'a', claim: { agent: 'bob' } },
    { id: 'A-2', status: 'in_progress', project: 'a', claim: { agent: 'ann' } },
    { id: 'A-3', status: 'Ready', project: 'a' },
  ];
  const pulse = computePulse(tasks, SCHEMA);
  assert.deepEqual(
    pulse.inProgress.map((t) => t.id),
    ['A-1', 'A-2']
  );
  assert.equal(pulse.readyByProject.length, 1);
});

test('проекты сортируются по убыванию числа ready', () => {
  const tasks = [
    { id: 'A-1', status: 'ready', project: 'a' },
    { id: 'B-1', status: 'ready', project: 'b' },
    { id: 'B-2', status: 'ready', project: 'b' },
    { id: 'C-1', status: 'ready', project: 'c' },
    { id: 'C-2', status: 'ready', project: 'c' },
  ];
  const pulse = computePulse(tasks, SCHEMA);
  assert.deepEqual(pulse.readyByProject, [
    { project: 'b', count: 2 },
    { project: 'c', count: 2 },
    { project: 'a', count: 1 },
  ]);
});

test('пустой снимок не роняет подсчёт', () => {
  assert.deepEqual(computePulse([], SCHEMA), {
    inProgress: [],
    readyByProject: [],
    needsInput: [],
    broken: 0,
  });
  assert.deepEqual(computePulse(undefined, SCHEMA).readyByProject, []);
});

test('needs-input собирается целиком, битые таски считаются отдельно', () => {
  const tasks = [
    { id: 'A-1', status: 'needs-input', project: 'a', title: 'вопрос' },
    { id: 'A-2', status: 'needs-input', project: 'a', title: 'ещё вопрос' },
    { id: 'A-3', status: 'ready', project: 'a', parseError: 'плохой YAML' },
  ];
  const pulse = computePulse(tasks, SCHEMA);
  assert.equal(pulse.needsInput.length, 2);
  assert.equal(pulse.broken, 1);
  assert.equal(pulse.readyByProject.length, 0, 'битая таска не должна попасть в ready');
});
