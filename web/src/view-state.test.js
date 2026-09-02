import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  resolveSelectedProjects,
  loadStoredProjects,
  storeProjects,
  projectMatches,
  describeProjectSelection,
} from './view-state.js';

test('resolveSelectedProjects оставляет только проекты, которые есть в снимке', () => {
  assert.deepEqual(resolveSelectedProjects(['KAM', 'GONE', 'TT'], ['KAM', 'TT']), ['KAM', 'TT']);
});

test('resolveSelectedProjects с пустым сохранённым набором — пустой набор', () => {
  assert.deepEqual(resolveSelectedProjects([], ['KAM']), []);
});

test('resolveSelectedProjects — если ни один сохранённый проект не выжил, набор пуст (не ломает доску)', () => {
  assert.deepEqual(resolveSelectedProjects(['GONE'], ['KAM', 'TT']), []);
});

test('loadStoredProjects/storeProjects без localStorage (аналог приватного режима) не бросают исключений', () => {
  assert.deepEqual(loadStoredProjects(), [], 'localStorage недоступен в node --test — тихо пусто');
  assert.doesNotThrow(() => storeProjects(['KAM']));
});

test('projectMatches: пустой набор пропускает любой проект', () => {
  assert.equal(projectMatches([], 'KAM'), true);
  assert.equal(projectMatches([], ''), true);
});

test('projectMatches: непустой набор — только перечисленные проекты', () => {
  assert.equal(projectMatches(['KAM', 'TT'], 'KAM'), true);
  assert.equal(projectMatches(['KAM', 'TT'], 'REF'), false);
});

test('describeProjectSelection: пустой набор и полный набор — «Все проекты»', () => {
  assert.equal(describeProjectSelection([], ['KAM', 'TT']), 'Все проекты');
  assert.equal(describeProjectSelection(['KAM', 'TT'], ['KAM', 'TT']), 'Все проекты');
});

test('describeProjectSelection: один проект — его имя', () => {
  assert.equal(describeProjectSelection(['KAM'], ['KAM', 'TT', 'REF']), 'KAM');
});

test('describeProjectSelection: несколько — количество с верным русским склонением', () => {
  assert.equal(describeProjectSelection(['KAM', 'TT'], ['KAM', 'TT', 'REF', 'HRS']), '2 проекта');
  assert.equal(describeProjectSelection(['A', 'B', 'C', 'D', 'E'], ['A', 'B', 'C', 'D', 'E', 'F']), '5 проектов');
  assert.equal(
    describeProjectSelection(Array.from({ length: 21 }, (_, i) => `P${i}`), Array.from({ length: 30 }, (_, i) => `P${i}`)),
    '21 проект',
  );
});
