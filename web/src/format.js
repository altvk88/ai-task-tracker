// Чистая логика кнопок форматирования над полем тела (TT-055): по выделению
// в textarea и коду кнопки вычисляет, чем заменить выделение и куда поставить
// курсор после — без DOM, поэтому проверяется `node --test` как edit.js.
//
// Каждая функция возвращает { snippet, selStart, selEnd }: snippet — строка,
// которой заменяется выделенный диапазон; selStart/selEnd — позиции нового
// выделения ОТНОСИТЕЛЬНО начала snippet (а не всего текста). applyFormat()
// ниже переводит их в абсолютные позиции и решает, что в TaskPanel.svelte
// не поместится: сама подстановка snippet в textarea — через
// document.execCommand('insertText', ...), а не присваивание value, иначе
// браузер стирает историю Ctrl+Z (см. отчёт по TT-055).

function wrap(selected, left, right, placeholder) {
  const content = selected || placeholder;
  return { snippet: left + content + right, selStart: left.length, selEnd: left.length + content.length };
}

/** Добавляет префикс к каждой непустой строке выделения; без выделения — просто вставляет префикс. */
function prefixLines(selected, prefix) {
  if (!selected) return { snippet: prefix, selStart: prefix.length, selEnd: prefix.length };
  const snippet = selected
    .split('\n')
    .map((line) => prefix + line)
    .join('\n');
  return { snippet, selStart: 0, selEnd: snippet.length };
}

const ACTIONS = {
  bold: (selected) => wrap(selected, '**', '**', 'жирный текст'),
  italic: (selected) => wrap(selected, '_', '_', 'курсив'),
  heading: (selected) => wrap(selected, '## ', '', 'Заголовок'),
  list: (selected) => prefixLines(selected, '- '),
  checkbox: (selected) => prefixLines(selected, '- [ ] '),
  link: (selected) => {
    if (!selected) {
      const text = 'текст';
      return { snippet: `[${text}](url)`, selStart: 1, selEnd: 1 + text.length };
    }
    const snippet = `[${selected}](url)`;
    const selStart = snippet.indexOf('](') + 2;
    return { snippet, selStart, selEnd: selStart + 3 };
  },
  table: () => {
    const snippet = '| Заголовок 1 | Заголовок 2 |\n| --- | --- |\n| Ячейка | Ячейка |';
    const selStart = snippet.indexOf('Заголовок 1');
    return { snippet, selStart, selEnd: selStart + 'Заголовок 1'.length };
  },
  code: (selected) => {
    if (selected.includes('\n')) {
      return { snippet: '```\n' + selected + '\n```', selStart: 4, selEnd: 4 + selected.length };
    }
    return wrap(selected, '`', '`', 'код');
  },
};

export const FORMAT_ACTIONS = Object.keys(ACTIONS);

/**
 * text/start/end — содержимое textarea и её текущее выделение; action — одна
 * из FORMAT_ACTIONS. Возвращает { text, selectionStart, selectionEnd, snippet }:
 * text/selectionStart/selectionEnd — итог для тестов и как запасной путь без
 * execCommand, snippet — то, что реально подставляется в textarea в
 * TaskPanel.svelte, чтобы не терять историю отмены.
 */
export function applyFormat(text, start, end, action) {
  const build = ACTIONS[action];
  if (!build) throw new Error(`неизвестное действие форматирования: ${action}`);
  const selected = text.slice(start, end);
  const { snippet, selStart, selEnd } = build(selected);
  return {
    text: text.slice(0, start) + snippet + text.slice(end),
    selectionStart: start + selStart,
    selectionEnd: start + selEnd,
    snippet,
  };
}
