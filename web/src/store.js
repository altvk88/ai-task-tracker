// Единственный источник данных для доски: ходит за снимком и за сменой
// статуса, держит подписку на `/api/events`, хранит фильтры и отдаёт
// компонентам готовые лейны. Компоненты в сеть не ходят.
import { writable, derived, get } from 'svelte/store';
import { applyChange, startLive } from './live.js';
import { normalizeStatus } from './status.js';
import { computePulse } from './pulse.js';
import { loadStoredProjects, storeProjects, resolveSelectedProjects, projectMatches } from './view-state.js';

// Раскладка по лейнам скопирована по смыслу с obsidian-plugin/src/lanes.ts —
// тот же порядок статусов, те же правила для пустых и неизвестных лейнов.
// Дублирование сознательное: плагин — TypeScript-модуль Obsidian, а не npm-
// пакет, который можно было бы просто заимпортировать в Vite-сборку.

const MS_PER_DAY = 24 * 60 * 60 * 1000;
// Порог «старая закрытая» (TT-063). Зашит константой, но не молча: подпись
// галочки в Toolbar.svelte подставляет то же число, так что менять его —
// значит менять и то, что видит человек.
export const OLD_CLOSED_DAYS = 14;
const CLOSED_STATUSES = new Set(['done', 'cancelled']);

// --- Токен на запись (TT-031) --------------------------------------------
//
// Сервер требует токен на запись только для запросов не с loopback-адреса —
// то есть именно для случая "открыл доску с телефона по ссылке". Ссылка
// содержит ?token=…; чтобы не таскать параметр по URL при каждом переходе
// внутри SPA, токен один раз сохраняется в sessionStorage (переживает
// перезагрузку той же вкладки, но не расшаривается на другие вкладки и не
// улетает на сервер как часть истории браузера дольше, чем нужно) и сразу
// вычищается из адресной строки.
const TOKEN_KEY = 'tt-write-token';

function captureTokenFromURL() {
  const url = new URL(window.location.href);
  const token = url.searchParams.get('token');
  if (!token) return;
  try {
    sessionStorage.setItem(TOKEN_KEY, token);
    url.searchParams.delete('token');
    window.history.replaceState(null, '', url);
  } catch {
    // Приватный режим и т.п. может запрещать sessionStorage — доска в этом
    // случае просто останется без сохранённого токена между переходами.
  }
}
captureTokenFromURL();

/**
 * Заголовок Authorization для fetch, если токен когда-либо был получен.
 * Экспортирован: панель таски и форма создания (TT-054) — такие же
 * писатели, как смена статуса, и им нужен тот же токен для записи с
 * телефона из локальной сети.
 */
export function authHeaders() {
  try {
    const token = sessionStorage.getItem(TOKEN_KEY);
    return token ? { Authorization: `Bearer ${token}` } : {};
  } catch {
    return {};
  }
}

/** Подпись лейна; для неизвестного статуса — сам статус, чтобы таска была видна. */
function laneTitle(schema, id) {
  return schema.statuses.find((st) => st.id === id)?.lane ?? id;
}

/**
 * Ключ сортировки «свежее — выше» (TT-063): по дате завершения для закрытых
 * статусов, иначе по дате создания. Без даты — в конец колонки, а не в
 * начало: пустая дата не значит «только что», обычно это как раз легаси-
 * запись без даты, то есть скорее старая, чем свежая.
 */
function freshnessKey(t) {
  const t1 = Date.parse(t.completed ?? '');
  if (!Number.isNaN(t1)) return t1;
  const t2 = Date.parse(t.created ?? '');
  return Number.isNaN(t2) ? -Infinity : t2;
}

/**
 * Раскладывает таски по лейнам в порядке схемы. Пустые лейны сохраняются —
 * иначе доска прыгает под руками при фильтрации. Неизвестные статусы уходят
 * в один общий лейн в конце, а не по лейну на каждый такой статус. Внутри
 * лейна — свежие сверху: без этого включение «старых закрытых» вываливает
 * тысячу записей в порядке файловой системы, и первым на глаза попадается
 * что попало, а не то, что человеку интереснее всего увидеть только что
 * включив фильтр.
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
  for (const l of lanes) l.tasks.sort((a, b) => freshnessKey(b) - freshnessKey(a));
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

// Набор проектов — единственное поле фильтра, которое переживает
// перезагрузку страницы (TT-059/TT-062, см. обоснование в view-state.js).
// Значение из localStorage подставляется оптимистично ещё до загрузки
// снимка; если снимок придёт без части проектов, ниже набор сам сузится.
export const filter = writable({ projects: loadStoredProjects(), query: '', showOldClosed: false });

// Каждое изменение набора — сразу в localStorage.
filter.subscribe(($filter) => storeProjects($filter.projects));

// Сохранённые (или уже выбранные) проекты могут исчезнуть из снимка —
// переименовали, удалили таски, открыли другой vault. Проверяем при каждой
// загрузке схемы, а не только один раз при старте: то же самое может
// произойти и посреди работы, когда снимок обновится сам.
snapshot.subscribe(($snapshot) => {
  if (!$snapshot.schema) return;
  const available = [...new Set($snapshot.tasks.map((t) => t.project).filter(Boolean))];
  filter.update((f) => ({ ...f, projects: resolveSelectedProjects(f.projects, available) }));
});

/** Список проектов, встречающихся в снимке, для выпадающего фильтра тулбара. */
export const projects = derived(snapshot, ($snapshot) =>
  [...new Set($snapshot.tasks.map((t) => t.project).filter(Boolean))].sort()
);

// Таски по id — по ПОЛНОМУ снимку, а не по отфильтрованному подмножеству:
// блокер может лежать в другом проекте, который сейчас скрыт фильтром, но
// это не делает его снятым. Экспортирован — панель таски (TT-035) ищет по
// нему и саму выбранную таску, и её блокеров.
export const tasksById = derived(snapshot, ($snapshot) => {
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

/** Таски, прошедшие проект и поиск, но ДО отсечения старых закрытых — общий
 *  знаменатель для видимых тасок и для счётчика скрытых (TT-063): иначе
 *  счётчик считал бы по другому набору, чем реально фильтруется. */
const projectAndSearchFiltered = derived([enrichedTasks, filter], ([$tasks, $filter]) => {
  const q = $filter.query.trim().toLowerCase();
  return $tasks.filter((t) => {
    if (!projectMatches($filter.projects, t.project)) return false;
    if (q && !t.id.toLowerCase().includes(q) && !t.title.toLowerCase().includes(q)) return false;
    return true;
  });
});

export const visibleTasks = derived([projectAndSearchFiltered, filter], ([$tasks, $filter]) =>
  $filter.showOldClosed ? $tasks : $tasks.filter((t) => !t.isOldClosed)
);

/** Сколько старых закрытых сейчас скрыто галочкой — обратная связь для
 *  Toolbar.svelte (TT-063): без неё «ничего не изменилось» неотличимо от
 *  поломки, особенно когда у выбранного проекта старых закрытых нет вовсе. */
export const hiddenOldClosedCount = derived(projectAndSearchFiltered, ($tasks) =>
  $tasks.reduce((n, t) => n + (t.isOldClosed ? 1 : 0), 0)
);

/** Показывать ли проект на карточке (TT-062): не нужно, пока на доске один
 *  проект — ни явно выбранный в одиночку, ни единственный существующий при
 *  «Все проекты». Как только проектов на доске больше одного, без подписи
 *  колонка превращается в кашу из неизвестно чьих карточек. */
export const showProjectOnCard = derived([filter, projects], ([$filter, $projects]) =>
  $filter.projects.length > 1 || ($filter.projects.length === 0 && $projects.length > 1)
);

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

// --- Живое обновление -----------------------------------------------------

/**
 * Есть ли сейчас связь с сервером (TT-061). 'online' до первого отказа —
 * оптимистично, чтобы не мигать индикатором на старте, пока первое
 * соединение ещё открывается штатно за доли секунды.
 */
export const connectionState = writable('online');

// Ноутбук закрыли и открыли — TCP-соединение к этому моменту обычно уже
// мертво, но браузер видит это не сразу (а иногда и вовсе не присылает
// onerror, если ОС просто заморозила вкладку без явного разрыва сокета).
// Поэтому при возврате видимости или восстановлении сети закрываем старое
// соединение и открываем новое сами, не дожидаясь, заметит ли это
// EventSource. Дублирующие события (visibilitychange и online почти
// одновременно) гасим минимальным интервалом между перезапусками.
const WAKE_DEBOUNCE_MS = 2000;

/** Подписывается на поток изменений и на признаки «устройство ожило». */
export function startLiveUpdates() {
  const options = {
    createSource: (url) => new EventSource(url),
    onChange: (change) => snapshot.update((s) => ({ ...s, tasks: applyChange(s.tasks, change) })),
    onResync: () => void loadSnapshot(),
    onStatus: (status) => connectionState.set(status),
  };

  let stop = startLive(options);
  let lastRestart = 0;

  const onWake = () => {
    const now = Date.now();
    if (now - lastRestart < WAKE_DEBOUNCE_MS) return;
    lastRestart = now;
    stop();
    stop = startLive(options);
    void loadSnapshot();
  };
  const onVisible = () => {
    if (document.visibilityState === 'visible') onWake();
  };
  document.addEventListener('visibilitychange', onVisible);
  window.addEventListener('online', onWake);

  return () => {
    stop();
    document.removeEventListener('visibilitychange', onVisible);
    window.removeEventListener('online', onWake);
  };
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

/** ID выбранной карточки — по ней TaskPanel открывает боковую панель. */
export const selectedId = writable('');

/** Открыта ли форма создания таски (кнопка на тулбаре, TT-054). */
export const creating = writable(false);

// --- Переключатель «доска / пульс» (TT-035) ------------------------------

export const view = writable('board');

/** Сводка для страницы «Пульс», посчитанная из уже загруженного снимка. */
export const pulseData = derived(snapshot, ($snapshot) =>
  $snapshot.schema ? computePulse($snapshot.tasks, $snapshot.schema) : null
);

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
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
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

/**
 * Точечно обновляет одну таску в снимке — общий хвост moveTask и, с TT-054,
 * панели/формы создания: сохранили поле, тело или создали таску — сразу
 * показываем актуальный ответ сервера, не дожидаясь SSE (оно всё равно
 * придёт следом и просто повторно применит то же самое).
 */
export function patchTask(id, task) {
  snapshot.update((s) => ({ ...s, tasks: applyChange(s.tasks, { id, kind: 'updated', task }) }));
}
