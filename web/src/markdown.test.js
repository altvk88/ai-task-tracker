import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderMarkdown } from './markdown.js';

test('заголовки, списки, чек-боксы, жирный/курсив рендерятся тегами', () => {
  const html = renderMarkdown(
    '## Заголовок\n\n- пункт\n- [ ] не сделано\n- [x] сделано\n\n**жирный** и *курсив*'
  );
  assert.match(html, /<h2>Заголовок<\/h2>/);
  assert.match(html, /<li>пункт<\/li>/);
  assert.match(html, /disabled="" type="checkbox"/);
  assert.match(html, /checked="" disabled="" type="checkbox"/);
  assert.match(html, /<strong>жирный<\/strong>/);
  assert.match(html, /<em>курсив<\/em>/);
});

test('чек-боксы рендерятся отключёнными (только для чтения)', () => {
  const html = renderMarkdown('- [x] готово');
  assert.match(html, /<input checked="" disabled="" type="checkbox">/);
});

test('таблица рендерится тегом table (прокрутка — через CSS в app.css)', () => {
  const html = renderMarkdown('| a | b |\n| --- | --- |\n| 1 | 2 |\n');
  assert.match(html, /<table>[\s\S]*<th>a<\/th>[\s\S]*<\/table>/);
});

test('блоки кода и инлайн-код рендерятся с экранированием содержимого', () => {
  const html = renderMarkdown('`inline`\n\n```js\nconst x = 1;\n```');
  assert.match(html, /<code>inline<\/code>/);
  assert.match(html, /<pre><code[^>]*>const x = 1;/);
});

test('ссылки и цитаты рендерятся', () => {
  const html = renderMarkdown('> цитата\n\n[текст](https://example.com)');
  assert.match(html, /<blockquote>/);
  assert.match(html, /<a href="https:\/\/example\.com">текст<\/a>/);
});

test('сырой блочный HTML экранируется, а не исполняется', () => {
  const html = renderMarkdown('<script>alert(1)</script>');
  assert.doesNotMatch(html, /<script>/);
  assert.match(html, /&lt;script&gt;/);
});

test('сырой инлайновый HTML (в т.ч. обработчики событий) экранируется', () => {
  const html = renderMarkdown('текст <img src=x onerror="alert(1)"> ещё текст');
  assert.doesNotMatch(html, /<img/);
  assert.match(html, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});

test('опасные схемы ссылок (javascript:) не создают исполняемый href', () => {
  const html = renderMarkdown('[кликни](javascript:alert(1))');
  assert.doesNotMatch(html, /href="javascript:/);
});

test('вики-ссылки остаются обычным текстом', () => {
  const html = renderMarkdown('см. [[другая-таска]] для контекста');
  assert.match(html, /\[\[другая-таска\]\]/);
});

test('пустое тело не падает', () => {
  assert.equal(renderMarkdown(''), '');
  assert.equal(renderMarkdown(null), '');
});

test('картинка с опасной схемой не превращается в <img>', () => {
  const html = renderMarkdown('![подпись](javascript:alert(1))');
  assert.ok(!html.includes('<img'), html);
  assert.ok(html.includes('подпись'), html);
});

test('обычная картинка остаётся картинкой', () => {
  const html = renderMarkdown('![схема](https://example.com/a.png)');
  assert.ok(html.includes('<img'), html);
});
