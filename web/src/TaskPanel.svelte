<script>
  // Боковая панель таски. Базовые поля берутся из уже загруженного снимка
  // (tasksById) — открывается мгновенно; тело и остальные поля фронтматтера
  // грузятся отдельным запросом лениво, при каждом открытии заново: держать
  // 1299 тел в памяти клиента незачем (см. комментарий у /api/task в api.go).
  import { selectedId, tasksById } from './store.js';
  import { renderMarkdown } from './markdown.js';

  const CLOSED_STATUSES = new Set(['done', 'cancelled']);

  let task = $derived($tasksById.get($selectedId));

  let detail = $state(null);
  let detailFor = $state('');
  let loading = $state(false);
  let error = $state('');

  $effect(() => {
    const id = $selectedId;
    if (!id) return;
    if (detailFor === id) return;
    loading = true;
    error = '';
    detail = null;
    fetch(`/api/task/${encodeURIComponent(id)}`)
      .then((res) => {
        if (!res.ok) throw new Error(`сервер ответил ${res.status}`);
        return res.json();
      })
      .then((data) => {
        detail = data;
        detailFor = id;
      })
      .catch((err) => {
        error = err.message;
      })
      .finally(() => {
        loading = false;
      });
  });

  function close() {
    selectedId.set('');
  }

  function openBlocker(id) {
    selectedId.set(id);
  }

  function blockerLive(id) {
    const b = $tasksById.get(id);
    return !b || !CLOSED_STATUSES.has(b.status);
  }

  function keydown(e) {
    if (e.key === 'Escape' && $selectedId) close();
  }
</script>

<svelte:window onkeydown={keydown} />

{#if task}
  <div class="panel-backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
    <aside class="panel" aria-label="Таска {task.id}">
      <div class="panel-head">
        <div>
          <span class="panel-id">{task.id}</span>
          <span class="panel-project">{task.project}</span>
        </div>
        <button type="button" class="panel-close" onclick={close} aria-label="Закрыть">×</button>
      </div>

      <h2 class="panel-title">{task.title}</h2>

      <dl class="panel-fields">
        <dt>Статус</dt>
        <dd>{task.status}</dd>
        <dt>Приоритет</dt>
        <dd>{task.priority || '—'}</dd>
        <dt>Эффорт</dt>
        <dd>{task.effort || '—'}</dd>
        <dt>Занято</dt>
        <dd>{task.claim?.agent || '—'}</dd>
        <dt>Путь</dt>
        <dd class="panel-path">{task.path}</dd>
      </dl>

      {#if task.blockedBy?.length}
        <div class="panel-section">
          <h3>Блокеры</h3>
          <ul class="blockers">
            {#each task.blockedBy as id (id)}
              <li>
                <button
                  type="button"
                  class="blocker"
                  class:live={blockerLive(id)}
                  onclick={() => openBlocker(id)}
                >
                  {id}
                </button>
              </li>
            {/each}
          </ul>
        </div>
      {/if}

      {#if loading}
        <p class="panel-status">Загрузка тела…</p>
      {:else if error}
        <p class="panel-status error">Не удалось загрузить: {error}</p>
      {:else if detail}
        <dl class="panel-fields">
          <dt>Создана</dt>
          <dd>{detail.created || '—'}</dd>
          <dt>Срок</dt>
          <dd>{detail.due || '—'}</dd>
          <dt>Готова с</dt>
          <dd>{detail.readyAt || '—'}</dd>
          <dt>Завершена</dt>
          <dd>{detail.completed || '—'}</dd>
          <dt>Попыток</dt>
          <dd>{detail.attempts || '—'}</dd>
        </dl>

        {#if detail.spec}
          <div class="panel-section">
            <h3>Spec</h3>
            <pre class="panel-raw">{detail.spec}</pre>
          </div>
        {/if}

        {#if detail.result}
          <div class="panel-section">
            <h3>Result</h3>
            <pre class="panel-raw">{detail.result}</pre>
          </div>
        {/if}

        <div class="panel-section">
          <h3>Тело</h3>
          {#if detail.body}
            <div class="markdown-body">{@html renderMarkdown(detail.body)}</div>
          {:else}
            <p class="panel-status">(пусто)</p>
          {/if}
        </div>
      {/if}
    </aside>
  </div>
{/if}
