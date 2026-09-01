import { test } from 'node:test';
import assert from 'node:assert/strict';
import { diffFields, parseDependsOn, describeSaveError } from './edit.js';

test('diffFields видит только изменившиеся поля, в фиксированном порядке', () => {
  const original = { title: 'Было', priority: 'medium', effort: '2h', due: '', spec: '' };
  const draft = { title: 'Было', priority: 'high', effort: '2h', due: '2026-09-05', spec: '' };
  assert.deepEqual(diffFields(original, draft), [
    ['priority', 'high'],
    ['due', '2026-09-05'],
  ]);
});

test('diffFields не путает отсутствующее поле с пустой строкой', () => {
  const original = {}; // таска без spec/due вообще не прислала их в detail
  const draft = { spec: '' };
  assert.deepEqual(diffFields(original, draft), []);
});

test('diffFields видит очистку поля', () => {
  const original = { spec: 'старый спек' };
  const draft = { spec: '' };
  assert.deepEqual(diffFields(original, draft), [['spec', '']]);
});

test('parseDependsOn режет по запятой и убирает пустые/пробелы', () => {
  assert.deepEqual(parseDependsOn(' TT-001,  TT-002 ,,TT-003'), ['TT-001', 'TT-002', 'TT-003']);
  assert.deepEqual(parseDependsOn(''), []);
  assert.deepEqual(parseDependsOn('   '), []);
});

test('describeSaveError помечает конфликтом только 409', () => {
  assert.deepEqual(describeSaveError(409, { error: 'таску изменили' }), {
    message: 'таску изменили',
    conflict: true,
  });
  assert.deepEqual(describeSaveError(500, null), { message: 'сервер ответил 500', conflict: false });
  assert.deepEqual(describeSaveError(400, { error: 'плохое значение' }), {
    message: 'плохое значение',
    conflict: false,
  });
});
