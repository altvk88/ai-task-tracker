import { test } from 'node:test';
import assert from 'node:assert/strict';
import { applyFormat, FORMAT_ACTIONS } from './format.js';

test('bold по выделению оборачивает текст и выделяет его же без звёздочек', () => {
  const r = applyFormat('привет мир', 7, 10, 'bold');
  assert.equal(r.text, 'привет **мир**');
  assert.equal(r.snippet, '**мир**');
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'мир');
});

test('bold без выделения вставляет заготовку и выделяет её целиком', () => {
  const r = applyFormat('текст ', 6, 6, 'bold');
  assert.equal(r.text, 'текст **жирный текст**');
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'жирный текст');
});

test('italic оборачивает подчёркиваниями', () => {
  const r = applyFormat('слово', 0, 5, 'italic');
  assert.equal(r.text, '_слово_');
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'слово');
});

test('heading добавляет префикс и не включает его в выделение', () => {
  const r = applyFormat('Заметка', 0, 7, 'heading');
  assert.equal(r.text, '## Заметка');
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'Заметка');
});

test('heading без выделения даёт плейсхолдер', () => {
  const r = applyFormat('', 0, 0, 'heading');
  assert.equal(r.text, '## Заголовок');
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'Заголовок');
});

test('list превращает каждую строку выделения в пункт', () => {
  const r = applyFormat('раз\nдва', 0, 7, 'list');
  assert.equal(r.text, '- раз\n- два');
  assert.equal(r.selectionStart, 0);
  assert.equal(r.selectionEnd, r.text.length);
});

test('list без выделения просто вставляет префикс и ставит курсор после него', () => {
  const r = applyFormat('', 0, 0, 'list');
  assert.equal(r.text, '- ');
  assert.equal(r.selectionStart, 2);
  assert.equal(r.selectionEnd, 2);
});

test('checkbox превращает строки в чек-лист', () => {
  const r = applyFormat('первое\nвторое', 0, 13, 'checkbox');
  assert.equal(r.text, '- [ ] первое\n- [ ] второе');
});

test('link по выделению подставляет текст ссылки и выделяет url', () => {
  const r = applyFormat('сайт', 0, 4, 'link');
  assert.equal(r.text, '[сайт](url)');
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'url');
});

test('link без выделения выделяет текст ссылки первым', () => {
  const r = applyFormat('', 0, 0, 'link');
  assert.equal(r.text, '[текст](url)');
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'текст');
});

test('table вставляет шаблон и выделяет первый заголовок независимо от выделения', () => {
  const r = applyFormat('игнор', 0, 5, 'table');
  assert.match(r.text, /^\| Заголовок 1 \| Заголовок 2 \|\n\| --- \| --- \|\n\| Ячейка \| Ячейка \|$/);
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'Заголовок 1');
});

test('code без переноса строки — инлайновый бэктик', () => {
  const r = applyFormat('x = 1', 0, 5, 'code');
  assert.equal(r.text, '`x = 1`');
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'x = 1');
});

test('code с переносом строки — блок из тройных бэктиков', () => {
  const r = applyFormat('a\nb', 0, 3, 'code');
  assert.equal(r.text, '```\na\nb\n```');
  assert.equal(r.text.slice(r.selectionStart, r.selectionEnd), 'a\nb');
});

test('код неизвестного действия падает с понятной ошибкой', () => {
  assert.throws(() => applyFormat('x', 0, 1, 'nope'), /неизвестное действие/);
});

test('FORMAT_ACTIONS перечисляет все восемь кнопок', () => {
  assert.deepEqual(FORMAT_ACTIONS, ['bold', 'italic', 'heading', 'list', 'checkbox', 'link', 'table', 'code']);
});
