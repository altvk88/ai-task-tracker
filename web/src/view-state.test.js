import { test } from 'node:test';
import assert from 'node:assert/strict';
import { resolveInitialProject, loadStoredProject, storeProject } from './view-state.js';

test('resolveInitialProject подставляет сохранённый проект, если он есть в снимке', () => {
  assert.equal(resolveInitialProject('KAM', ['KAM', 'TT']), 'KAM');
});

test('resolveInitialProject откатывается к «Все проекты», если сохранённого проекта больше нет', () => {
  assert.equal(resolveInitialProject('GONE', ['KAM', 'TT']), '');
});

test('resolveInitialProject с пустым сохранённым значением — «Все проекты»', () => {
  assert.equal(resolveInitialProject('', ['KAM']), '');
});

test('loadStoredProject/storeProject без localStorage (аналог приватного режима) не бросают исключений', () => {
  assert.equal(loadStoredProject(), '', 'localStorage недоступен в node --test — тихо пусто');
  assert.doesNotThrow(() => storeProject('KAM'));
});
