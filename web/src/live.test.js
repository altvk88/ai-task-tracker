import { test } from 'node:test';
import assert from 'node:assert/strict';
import { applyChange, parseChange, reconnectDelay, startLive, RECONNECT_MAX_MS } from './live.js';

const TASKS = [
  { id: 'A-1', status: 'ready', title: 'первая' },
  { id: 'A-2', status: 'backlog', title: 'вторая' },
];

test('патч change обновляет одну таску и не трогает остальные', () => {
  const next = applyChange(TASKS, { id: 'A-1', kind: 'updated', task: { id: 'A-1', status: 'in-progress', title: 'первая' } });
  assert.equal(next.length, 2);
  assert.equal(next[0].status, 'in-progress');
  assert.equal(next[1], TASKS[1], 'соседняя таска — тот же объект, перерисовывать её незачем');
  assert.equal(TASKS[0].status, 'ready', 'исходный список не изменён');
});

test('патч на неизвестную таску добавляет её', () => {
  const next = applyChange(TASKS, { id: 'A-9', kind: 'added', task: { id: 'A-9', status: 'ready', title: 'новая' } });
  assert.deepEqual(next.map((t) => t.id), ['A-1', 'A-2', 'A-9']);
});

test('kind: removed убирает таску', () => {
  assert.deepEqual(applyChange(TASKS, { id: 'A-1', kind: 'removed' }).map((t) => t.id), ['A-2']);
  assert.equal(applyChange(TASKS, { id: 'нет-такой', kind: 'removed' }), TASKS, 'нечего убирать — список тот же');
});

test('нечитаемое событие отбрасывается, а не рушит подписку', () => {
  assert.equal(parseChange('}{'), null);
  assert.equal(parseChange('{"kind":"updated"}'), null, 'без id патч применить некуда');
  assert.equal(applyChange(TASKS, null), TASKS);
  assert.deepEqual(parseChange('{"id":"A-1","task":{"id":"A-1"}}'), { id: 'A-1', kind: 'updated', task: { id: 'A-1' } });
});

test('пауза переподключения растёт и упирается в потолок', () => {
  assert.deepEqual([1, 2, 3, 4].map(reconnectDelay), [1000, 2000, 4000, 8000]);
  assert.equal(reconnectDelay(99), RECONNECT_MAX_MS);
});

/** Подставной EventSource: те же три способа подписки, что использует startLive. */
class FakeSource {
  constructor() {
    this.listeners = new Map();
    this.closed = false;
    FakeSource.opened.push(this);
  }
  addEventListener(type, fn) {
    this.listeners.set(type, fn);
  }
  emit(type, data) {
    this.listeners.get(type)?.({ data });
  }
  close() {
    this.closed = true;
  }
}
FakeSource.opened = [];

function harness() {
  FakeSource.opened = [];
  const calls = { resync: 0, changes: [], timers: [] };
  const stop = startLive({
    createSource: () => new FakeSource(),
    onChange: (c) => calls.changes.push(c),
    onResync: () => {
      calls.resync += 1;
    },
    setTimer: (fn, ms) => {
      calls.timers.push({ fn, ms });
      return calls.timers.length;
    },
    clearTimer: () => {},
  });
  return { calls, stop, sources: FakeSource.opened };
}

test('событие change доходит до обработчика разобранным', () => {
  const { calls, stop, sources } = harness();
  sources[0].emit('change', '{"id":"A-1","kind":"updated","task":{"id":"A-1","status":"done"}}');
  assert.deepEqual(calls.changes, [{ id: 'A-1', kind: 'updated', task: { id: 'A-1', status: 'done' } }]);
  stop();
  assert.equal(sources[0].closed, true, 'остановка закрывает соединение');
});

test('resync приводит к перезапросу снимка', () => {
  const { calls, stop, sources } = harness();
  sources[0].emit('resync', '{}');
  assert.equal(calls.resync, 1);
  stop();
});

test('обрыв переподключается с нарастающей паузой и один раз на соединение', () => {
  const { calls, stop, sources } = harness();

  sources[0].onerror();
  sources[0].onerror();
  assert.equal(sources[0].closed, true);
  assert.deepEqual(calls.timers.map((t) => t.ms), [1000], 'повторный onerror не плодит попыток');

  calls.timers[0].fn();
  assert.equal(sources.length, 2, 'по таймеру открыто новое соединение');

  sources[1].onerror();
  assert.deepEqual(calls.timers.map((t) => t.ms), [1000, 2000]);

  calls.timers[1].fn();
  sources[2].onopen();
  assert.equal(calls.resync, 1, 'после переподключения состояние пересобирается снимком');

  sources[2].onerror();
  assert.deepEqual(calls.timers.map((t) => t.ms), [1000, 2000, 1000], 'удачное открытие сбрасывает счётчик');
  stop();
});

test('первое открытие снимок не перезапрашивает — его уже грузит вызывающий', () => {
  const { calls, stop, sources } = harness();
  sources[0].onopen();
  assert.equal(calls.resync, 0);
  stop();
});

test('после остановки переподключений не планируется', () => {
  const { calls, stop, sources } = harness();
  stop();
  sources[0].onerror();
  assert.deepEqual(calls.timers, []);
});
