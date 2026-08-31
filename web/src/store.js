// Единственный источник данных для доски: ходит за снимком и за сменой
// статуса, держит подписку на `/api/events`, хранит фильтры и отдаёт
// компонентам готовые лейны. Компоненты в сеть не ходят.
import { writable, derived, get } from 'svelte/store';
import { applyChange, startLive } from './live.js';

// Раскладка по лейнам скопирована по смыслу с obsidian-plugin/src/lanes.ts —
// тот же порядок статусов, те же правила для пустых и неизвестных лейнов.
// Дублирование сознательное: плагин — TypeScript-модуль Obsidian, а не npm-
// пакет, который можно было бы просто заимпортировать в Vite-сборку.

const MS_PER_DAY = 24 * 60 * 60 * 1000;
const OLD_CLOSED_DAYS = 14;
const CLOSED_STATUSES = new Set(['done', 'cancelled']);

/** Приводит написание статуса к каноническому по схеме сервера. */
function normalizeStatus(schema, status) {
  const v = (status ?? '').trim().toLowerCase();
  if (v === '') return '';
  const canon = schema.aliases?.[v] ?? v;
  return schema.statuses.some((st) => st.id === canon) ? canon : (status ?? '').trim();
}

/** Подпись лейна; для неизвестного статуса — сам статус, чтобы таска была видна. */
function laneTitle(schema, id) {
  return schema.statuses.find((st) => st.id === id)?.lane ?? id;
}

/**
 * Раскладывает таски по лейнам в порядке схемы. Пустые лейны сохраняются —
 * иначе доска прыгает под руками при фильтрации. Неизвестные статусы уходят
 * в один общий лейн в конце, а не по лейну на каждый такой статус.
 */
function buildLanes(schema, tasks) {
  const lanes = schema.statuses.map((st) => ({ lane: st.lane, status: st.id, tasks: [] }));
  const byStatus = new Map(lanes.map((l) => [l.status, l]));
  let unknown = null;

  for (const t of tasks) {
    const id = normalizeStatus(schema, t.status);
    const known = byStatus.get(id);
    if (known) {
      known.tasks.push(t);
      continue;
    }
    if (!unknown) {
      unknown = { lane: laneTitle(schema, id), status: id, tasks: [] };
      lanes.push(unknown);
    }
    unknown.tasks.push(t);
  }
  return lanes;
}

/** true, если блокер ещё держит таску: не done/cancelled или отсутствует в снимке. */
function isLiveBlocker(id, byId) {
  const blocker = byId.get(id);
  return !blocker || !CLOSED_STATUSES.has(blocker.status);
}

function daysSince(dateStr) {
  const t = Date.parse(dateStr);
  if (Number.isNaN(t)) return null;
  return (Date.now() - t) / MS_PER_DAY;
}

export const snapshot = writable({ tasks: [], schema: null, summary: null, loading: true, error: '' });

export const filter = writable({ project: '', query: '', showOldClosed: false });

/** Список проектов, встречающихся в снимке, для выпадающего фильтра тулбара. */
export const projects = derived(snapshot, ($snapshot) =>
  [...new Set($snapshot.tasks.map((t) => t.project).filter(Boolean))].sort()
);

// Таски по id — по ПОЛНОМУ снимку, а не по отфильтрованному подмножеству:
// блокер может лежать в другом проекте, который сейчас скрыт фильтром, но
// это не делает его снятым.
const tasksById = derived(snapshot, ($snapshot) => {
  const map = new Map();
  for (const t of $snapshot.tasks) if (t.id) map.set(t.id, t);
  return map;
});

/** Таски без ошибок парсинга, с признаком старой закрытой и живыми блокерами. */
const enrichedTasks = derived([snapshot, tasksById], ([$snapshot, $byId]) =>
  $snapshot.tasks
    .filter((t) => !t.parseError)
    .map((t) => {
      const age = t.completed ? daysSince(t.completed) : null;
      // Дата завершения не проставлена — считаем таску старой: это либо
      // легаси-запись до автоматической простановки completed, либо забытый
      // вручную закрытый пункт. Прятать по умолчанию безопаснее, чем сорить
      // доску записями неизвестного возраста.
      const isOldClosed = CLOSED_STATUSES.has(t.status) && (age === null || age > OLD_CLOSED_DAYS);
      const liveBlockers = (t.blockedBy ?? []).filter((id) => isLiveBlocker(id, $byId));
      return { ...t, isOldClosed, liveBlockers };
    })
);

export const visibleTasks = derived([enrichedTasks, filter], ([$tasks, $filter]) => {
  const q = $filter.query.trim().toLowerCase();
  return $tasks.filter((t) => {
    if (!$filter.showOldClosed && t.isOldClosed) return false;
    if ($filter.project && t.project !== $filter.project) return false;
    if (q && !t.id.toLowerCase().includes(q) && !t.title.toLowerCase().includes(q)) return false;
    return true;
  });
});

export const lanes = derived([snapshot, visibleTasks], ([$snapshot, $visible]) =>
  $snapshot.schema ? buildLanes($snapshot.schema, $visible) : []
);

/**
 * Загружает снимок с сервера: при монтировании App и повторно на resync.
 * «Загрузка…» показывается только пока доски ещё нет — иначе каждый resync
 * гасил бы её на время запроса.
 */
export async function loadSnapshot() {
  snapshot.update((s) => ({ ...s, loading: !s.schema, error: '' }));
  try {
    const res = await fetch('/api/snapshot');
    if (!res.ok) throw new Error(`сервер ответил ${res.status}`);
    const data = await res.json();
    snapshot.set({ tasks: data.tasks, schema: data.schema, summary: data.summary, loading: false, error: '' });
  } catch (err) {
    // Первая загрузка провалилась — показываем ошибку вместо доски. Повторная
    // — доска на экране остаётся (пусть и устаревшая), ошибка уходит в
    // уведомление: пустой экран вместо данных здесь хуже, чем старые данные.
    if (get(snapshot).schema) showNotice(`не удалось обновить снимок: ${err.message}`);
    else snapshot.update((s) => ({ ...s, loading: false, error: String(err) }));
  }
}

// --- Живое обновление ---------------------------------------------------

/** Подписывается на поток изменений. Возвращает функцию остановки. */
export function startLiveUpdates() {
  return startLive({
    createSource: (url) => new EventSource(url),
    onChange: (change) => snapshot.update((s) => ({ ...s, tasks: applyChange(s.tasks, change) })),
    onResync: () => void loadSnapshot(),
  });
}

// --- Уведомления --------------------------------------------------------

/** Текст последнего отказа; пусто — показывать нечего. */
export const notice = writable('');

// Отказ должен успеть прочитаться, поэтому уведомление живёт заметно дольше
// обычного тоста и в любом случае закрывается кнопкой.
const NOTICE_MS = 12000;
let noticeTimer = null;

export function showNotice(text) {
  if (noticeTimer !== null) clearTimeout(noticeTimer);
  notice.set(text);
  noticeTimer = setTimeout(dismissNotice, NOTICE_MS);
}

export function dismissNotice() {
  if (noticeTimer !== null) clearTimeout(noticeTimer);
  noticeTimer = null;
  notice.set('');
}

// --- Смена статуса ------------------------------------------------------

/** ID выбранной карточки. Панель таски — TT-035; пока это только подсветка. */
export const selectedId = writable('');

/**
 * Переносит таску в другой статус. Карточка переезжает сразу, запрос уходит
 * следом; отказ (409 от правил флоу, 500 от записи) возвращает её на место и
 * показывает причину.
 */
export async function moveTask(id, to) {
  const state = get(snapshot);
  const task = state.tasks.find((t) => t.id === id);
  if (!task || !state.schema || !to) return;
  // Бросок в свой же лейн — не изменение: ни запроса, ни мигания карточки.
  if (normalizeStatus(state.schema, task.status) === normalizeStatus(state.schema, to)) return;

  patchTask(id, { ...task, status: to });
  try {
    const res = await fetch(`/api/task/${encodeURIComponent(id)}/status`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ to }),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok) throw new Error(data?.error || `сервер ответил ${res.status}`);
    patchTask(id, data);
  } catch (err) {
    patchTask(id, task);
    showNotice(`${id}: ${err.message}`);
  }
}

function patchTask(id, task) {
  snapshot.update((s) => ({ ...s, tasks: applyChange(s.tasks, { id, kind: 'updated', task }) }));
}
