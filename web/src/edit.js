// Чистая логика редактирования и создания таски из панели — без fetch и без
// DOM, чтобы проверяться `node --test` так же, как live.js и pulse.js.
// Сами сетевые запросы остаются в TaskPanel.svelte/CreateTask.svelte: там,
// где их держит остальной код панели.

/** Поля фронтматтера, которые правит панель, в порядке отправки на сервер. */
export const EDITABLE_FIELDS = ['title', 'priority', 'effort', 'due', 'spec'];

/**
 * Сравнивает значения полей до правки с черновиком формы и возвращает список
 * изменившихся пар [key, value] в порядке EDITABLE_FIELDS. Отсутствующее в
 * original значение считается пустой строкой — так что таска без due/spec и
 * стёртое в форме поле дают одинаковый результат (реальное изменение, а не
 * undefined !== '').
 */
export function diffFields(original, draft) {
  const out = [];
  for (const key of EDITABLE_FIELDS) {
    const before = original[key] ?? '';
    const after = draft[key] ?? '';
    if (before !== after) out.push([key, after]);
  }
  return out;
}

/** Разбирает список зависимостей из текстового поля формы создания таски. */
export function parseDependsOn(text) {
  return text
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}

/**
 * Переводит отказ записи (код ответа + разобранное тело ошибки) в сообщение
 * для человека и признак конфликта версии. 409 — единственный код, где нужен
 * особый UI (см. TaskPanel.svelte): предложение обновить и повторить, а не
 * просто текст ошибки, который проще потерять в общем потоке.
 */
export function describeSaveError(status, data) {
  return { message: data?.error || `сервер ответил ${status}`, conflict: status === 409 };
}
