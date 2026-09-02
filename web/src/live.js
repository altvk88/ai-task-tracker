// Живое обновление доски: разбор SSE-события, точечный патч списка тасок и
// переподключение с нарастающей паузой.
//
// Ни Svelte, ни DOM здесь нет намеренно: всё внешнее (сам EventSource,
// таймер) приходит параметрами, поэтому модуль проверяется `node --test` без
// браузера — так же, как модули плагина Obsidian.

/** Пауза перед первой попыткой переподключения. */
export const RECONNECT_BASE_MS = 1000;

// Потолок паузы. Было 30 секунд по периоду пульса сервера (defaultHeartbeat в
// internal/server/sse.go) — но сервер на своей же машине поднимается за доли
// секунды, и полминуты молчания перед следующей попыткой означают полминуты
// устаревшей доски сверх того, что уже показывает признак обрыва (TT-061).
// 8 секунд — это всего 4-й шаг экспоненты (1-2-4-8), после которого доска уже
// явно предупредила о разрыве, а частить чаще незачем: не мучаем сервер,
// который либо ещё не поднялся, либо неисправен по другой причине.
export const RECONNECT_MAX_MS = 8000;

/** Пауза перед попыткой номер `attempt` (1 — первая): 1с, 2с, 4с… до потолка. */
export function reconnectDelay(attempt) {
  return Math.min(RECONNECT_BASE_MS * 2 ** Math.max(0, attempt - 1), RECONNECT_MAX_MS);
}

/**
 * Разбирает тело события `change`. Мусор и события без id молча отбрасываем:
 * рвать подписку из-за одного нечитаемого события хуже, чем пропустить его —
 * следующий resync или перезагрузка страницы всё равно всё выправят.
 */
export function parseChange(raw) {
  let data;
  try {
    data = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!data || typeof data.id !== 'string' || data.id === '') return null;
  return { id: data.id, kind: data.kind || 'updated', task: data.task ?? null };
}

/**
 * Применяет патч к списку тасок, возвращая новый список: обновляет одну
 * таску, добавляет незнакомую (её могли создать после снимка) и убирает
 * удалённую. Остальные объекты те же самые — Svelte перерисует только
 * затронутую карточку.
 */
export function applyChange(tasks, change) {
  if (!change) return tasks;
  if (change.kind === 'removed') {
    const next = tasks.filter((t) => t.id !== change.id);
    return next.length === tasks.length ? tasks : next;
  }
  if (!change.task) return tasks;
  const i = tasks.findIndex((t) => t.id === change.id);
  if (i < 0) return [...tasks, change.task];
  const next = tasks.slice();
  next[i] = change.task;
  return next;
}

/**
 * Держит подписку на поток событий, переподключаясь при обрыве.
 *
 * Переподключение ведём сами, а не полагаемся на встроенное в EventSource:
 * оно оживает только после разрыва уже установленного потока, а на ответ с
 * ошибкой (сервер перезапускается и отдаёт 502, обратный прокси — 503)
 * переводит соединение в CLOSED навсегда. Плюс встроенная пауза постоянная,
 * а нам нужна нарастающая.
 *
 * Возвращает функцию остановки.
 */
export function startLive({
  createSource,
  onChange,
  onResync,
  // Сообщает 'online'/'offline' наружу — источник признака связи в шапке
  // (TT-061). Вызывается только на смене состояния, не на каждой попытке:
  // иначе серия неудачных переподключений дёргала бы индикатор туда-сюда.
  onStatus = () => {},
  setTimer = setTimeout,
  clearTimer = clearTimeout,
  delay = reconnectDelay,
}) {
  let source = null;
  let timer = null;
  let attempt = 0;
  let stopped = false;
  let offline = false;

  const setOffline = (v) => {
    if (offline === v) return;
    offline = v;
    onStatus(v ? 'offline' : 'online');
  };

  const open = () => {
    timer = null;
    const src = createSource('/api/events');
    source = src;
    let dead = false;

    src.onopen = () => {
      // За время обрыва события потеряны, поэтому после переподключения
      // состояние собираем заново. При самом первом открытии снимок уже
      // грузит вызывающий — второй запрос не нужен.
      setOffline(false);
      if (attempt > 0) onResync();
      attempt = 0;
    };
    src.addEventListener('change', (ev) => onChange(parseChange(ev.data)));
    src.addEventListener('resync', () => onResync());
    src.onerror = () => {
      // onerror у EventSource может прийти не один раз; закрываем и
      // планируем ровно одну повторную попытку на соединение.
      if (dead) return;
      dead = true;
      setOffline(true);
      src.close();
      if (source === src) source = null;
      if (stopped) return;
      attempt += 1;
      timer = setTimer(open, delay(attempt));
    };
  };

  open();

  return () => {
    stopped = true;
    if (timer !== null) clearTimer(timer);
    timer = null;
    source?.close();
    source = null;
  };
}
