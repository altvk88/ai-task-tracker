// Подсчёт для страницы «Пульс» — чистая функция от списка тасок снимка, без
// Svelte и без сети: те же соображения, что и у live.js — проверяется
// `node --test` без браузера.

import { normalizeStatus } from './status.js';

/**
 * Считает четыре раздела «Пульса» из уже загруженного снимка:
 * - inProgress — таски в работе (кто держит claim);
 * - readyByProject — сколько ready по проектам, по убыванию;
 * - needsInput — таски, зависшие в needs-input, целиком;
 * - broken — сколько тасок не распарсилось (см. `tt doctor`).
 */
export function computePulse(tasks, schema) {
  const inProgress = [];
  const needsInput = [];
  const readyCounts = new Map();
  let broken = 0;

  for (const t of tasks ?? []) {
    if (t.parseError) {
      broken += 1;
      continue;
    }
    const status = normalizeStatus(schema, t.status);
    if (status === 'in-progress') {
      inProgress.push({ id: t.id, project: t.project, agent: t.claim?.agent || '' });
    } else if (status === 'needs-input') {
      needsInput.push({ id: t.id, project: t.project, title: t.title });
    } else if (status === 'ready') {
      const key = t.project || '(без проекта)';
      readyCounts.set(key, (readyCounts.get(key) || 0) + 1);
    }
  }

  const readyByProject = [...readyCounts.entries()]
    .map(([project, count]) => ({ project, count }))
    .sort((a, b) => b.count - a.count || a.project.localeCompare(b.project));

  inProgress.sort((a, b) => a.id.localeCompare(b.id));
  needsInput.sort((a, b) => a.id.localeCompare(b.id));

  return { inProgress, readyByProject, needsInput, broken };
}
