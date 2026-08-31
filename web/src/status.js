// Нормализация написания статуса — общая для доски (store.js) и «Пульса»
// (pulse.js): обе стороны должны одинаково понимать легаси-написания вроде
// "In Progress" и лишние пробелы, иначе подсчёты и лейны разойдутся.

/** Приводит написание статуса к каноническому по схеме сервера. */
export function normalizeStatus(schema, status) {
  const v = (status ?? '').trim().toLowerCase();
  if (v === '') return '';
  const canon = schema?.aliases?.[v] ?? v;
  return schema?.statuses?.some((st) => st.id === canon) ? canon : (status ?? '').trim();
}
