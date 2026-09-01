<script>
  // Боковая панель таски. Базовые поля берутся из уже загруженного снимка
  // (tasksById) — открывается мгновенно; тело и остальные поля фронтматтера
  // грузятся отдельным запросом лениво, при каждом открытии заново: держать
  // 1299 тел в памяти клиента незачем (см. комментарий у /api/task в api.go).
  //
  // Правка (TT-054): черновик (`draft`) — отдельное состояние, не связанное
  // с `task`/`detail` напрямую. Это и даёт «SSE не стирает несохранённое»:
  // `detail` перечитывается только при открытии другой таски (см. $effect
  // ниже), а не при каждом обновлении снимка, поэтому набранный текст
  // никто не перезатирает, пока не нажато «Сохранить».
  import { selectedId, tasksById, authHeaders, patchTask } from './store.js';
  import { renderMarkdown } from './markdown.js';
  import { diffFields, describeSaveError } from './edit.js';
  import { applyFormat, FORMAT_ACTIONS } from './format.js';

  const FORMAT_LABELS = {
    bold: ['Ж', 'Жирный'],
    italic: ['К', 'Курсив'],
    heading: ['H', 'Заголовок'],
    list: ['•', 'Список'],
    checkbox: ['☑', 'Чек-бокс'],
    link: ['🔗', 'Ссылка'],
    table: ['▦', 'Таблица'],
    code: ['</>', 'Код'],
  };

  const CLOSED_STATUSES = new Set(['done', 'cancelled']);
  const PRIORITIES = ['high', 'medium', 'low'];

  let task = $derived($tasksById.get($selectedId));

  let detail = $state(null);
  let detailFor = $state('');
  let loading = $state(false);
  let error = $state('');

  // --- Правка -------------------------------------------------------------

  let editing = $state(false);
  let draft = $state(null);
  let baseVersion = $state('');
  let saving = $state(false);
  let saveError = $state('');
  let conflict = $state(false);

  // Вкладка тела в режиме правки: «Правка» — textarea с кнопками
  // форматирования, «Предпросмотр» — рендер того же draft.body через
  // renderMarkdown. Переключение не трогает draft, поэтому набранное не
  // теряется (см. AC «переключение вкладок не теряет набранное»).
  let bodyTab = $state('edit');
  let bodyTextarea = $state(null);

  /**
   * Кнопка форматирования (TT-055). Подставляет вычисленный applyFormat()
   * фрагмент через document.execCommand('insertText', ...), а не
   * присваиванием draft.body = ...: присваивание значения textarea в обход
   * родных команд редактирования стирает историю браузера, и Ctrl+Z после
   * нажатия кнопки перестаёт работать. execCommand('insertText') — это та
   * же операция, что и обычная печать/вставка, поэтому она пишется в
   * нативный undo-стек textarea и корректно отменяется. bind:value уже
   * слушает событие 'input', которое execCommand сама генерирует, поэтому
   * draft.body синхронизируется без ручного присваивания.
   *
   * Фокус остаётся в поле: кнопки используют onmousedown с preventDefault,
   * поэтому клик по кнопке никогда не переводит фокус на неё — textarea не
   * теряет ни фокус, ни выделение между чтением selectionStart/End и вызовом
   * execCommand.
   */
  function insertFormat(action) {
    const el = bodyTextarea;
    if (!el) return;
    const start = el.selectionStart;
    const end = el.selectionEnd;
    const result = applyFormat(draft.body, start, end, action);
    el.focus();
    el.setSelectionRange(start, end);
    const applied = document.execCommand && document.execCommand('insertText', false, result.snippet);
    if (!applied) {
      // Запасной путь для окружений без execCommand: работает корректно,
      // но теряет историю отмены для этой вставки — редкий случай, не
      // основной путь в актуальных браузерах.
      draft.body = result.text;
    }
    el.setSelectionRange(result.selectionStart, result.selectionEnd);
  }

  $effect(() => {
    const id = $selectedId;
    if (!id) return;
    if (detailFor === id) return;
    loading = true;
    error = '';
    detail = null;
    editing = false;
    draft = null;
    saveError = '';
    conflict = false;
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
    if (editing && !confirm('Есть несохранённые изменения. Закрыть без сохранения?')) return;
    selectedId.set('');
  }

  function openBlocker(id) {
    if (editing && !confirm('Есть несохранённые изменения. Перейти без сохранения?')) return;
    selectedId.set(id);
  }

  function blockerLive(id) {
    const b = $tasksById.get(id);
    return !b || !CLOSED_STATUSES.has(b.status);
  }

  function keydown(e) {
    if (e.key === 'Escape' && $selectedId) close();
  }

  function startEdit() {
    draft = {
      title: task.title,
      priority: task.priority,
      effort: task.effort,
      due: detail.due,
      spec: detail.spec,
      body: detail.body,
    };
    baseVersion = detail.version;
    saveError = '';
    conflict = false;
    bodyTab = 'edit';
    editing = true;
  }

  function cancelEdit() {
    editing = false;
    draft = null;
    saveError = '';
    conflict = false;
  }

  /** Отмена правки после конфликта: черновик тоже отбрасывается, а панель
   *  перечитывает таску заново — оставлять на экране версию, которую сервер
   *  уже отверг, хуже, чем лишний запрос. */
  function discardAfterConflict() {
    cancelEdit();
    detailFor = '';
  }

  /** Подтягивает свежую версию с сервера после 409, не трогая черновик —
   *  человек не теряет набранный текст и может нажать «Сохранить» ещё раз. */
  async function refreshAfterConflict() {
    try {
      const res = await fetch(`/api/task/${encodeURIComponent(task.id)}`);
      if (!res.ok) throw new Error(`сервер ответил ${res.status}`);
      detail = await res.json();
      baseVersion = detail.version;
      conflict = false;
      saveError = '';
    } catch (err) {
      saveError = `Не удалось обновить: ${err.message}`;
    }
  }

  async function postJSON(url, body) {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => null);
    if (!res.ok) {
      const { message, conflict: isConflict } = describeSaveError(res.status, data);
      const err = new Error(message);
      err.conflict = isConflict;
      throw err;
    }
    return data;
  }

  async function save() {
    if (!task || !detail || !draft) return;
    saving = true;
    saveError = '';
    conflict = false;
    const original = { title: task.title, priority: task.priority, effort: task.effort, due: detail.due, spec: detail.spec };
    const changedFields = diffFields(original, draft);
    const bodyChanged = draft.body !== detail.body;
    let version = baseVersion;
    try {
      for (const [key, value] of changedFields) {
        const data = await postJSON(`/api/task/${encodeURIComponent(task.id)}/field`, { key, value, baseVersion: version });
        version = data.version;
      }
      if (bodyChanged) {
        const data = await postJSON(`/api/task/${encodeURIComponent(task.id)}/body`, { body: draft.body, baseVersion: version });
        version = data.version;
      }
      detail = { ...detail, due: draft.due, spec: draft.spec, body: draft.body, version };
      patchTask(task.id, { ...task, title: draft.title, priority: draft.priority, effort: draft.effort, version });
      editing = false;
      draft = null;
    } catch (err) {
      // Часть изменений могла уже уйти на диск (цикл шёл по одному полю за
      // раз) — baseVersion подтягиваем до места отказа, чтобы повторная
      // попытка не переписывала уже сохранённое старым baseVersion.
      baseVersion = version;
      saveError = err.message;
      conflict = !!err.conflict;
    } finally {
      saving = false;
    }
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

      {#if editing}
        <input class="field-input panel-title-input" bind:value={draft.title} aria-label="Заголовок" />
      {:else}
        <h2 class="panel-title">{task.title}</h2>
      {/if}

      <dl class="panel-fields">
        <dt>Статус</dt>
        <dd>{task.status}</dd>
        <dt>Приоритет</dt>
        <dd>
          {#if editing}
            <select class="field-input" bind:value={draft.priority}>
              {#each PRIORITIES as p (p)}<option value={p}>{p}</option>{/each}
            </select>
          {:else}
            {task.priority || '—'}
          {/if}
        </dd>
        <dt>Эффорт</dt>
        <dd>
          {#if editing}
            <input class="field-input" bind:value={draft.effort} placeholder="2h, 1d…" />
          {:else}
            {task.effort || '—'}
          {/if}
        </dd>
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
        {#if !editing}
          <div class="panel-actions">
            <button type="button" class="btn" onclick={startEdit}>Править</button>
          </div>
        {/if}

        <dl class="panel-fields">
          <dt>Создана</dt>
          <dd>{detail.created || '—'}</dd>
          <dt>Срок</dt>
          <dd>
            {#if editing}
              <input class="field-input" type="date" bind:value={draft.due} />
            {:else}
              {detail.due || '—'}
            {/if}
          </dd>
          <dt>Готова с</dt>
          <dd>{detail.readyAt || '—'}</dd>
          <dt>Завершена</dt>
          <dd>{detail.completed || '—'}</dd>
          <dt>Попыток</dt>
          <dd>{detail.attempts || '—'}</dd>
        </dl>

        <div class="panel-section">
          <h3>Spec</h3>
          {#if editing}
            <textarea class="field-input textarea" bind:value={draft.spec} rows="4"></textarea>
          {:else if detail.spec}
            <pre class="panel-raw">{detail.spec}</pre>
          {:else}
            <p class="panel-status">(пусто)</p>
          {/if}
        </div>

        {#if detail.result}
          <div class="panel-section">
            <h3>Result</h3>
            <pre class="panel-raw">{detail.result}</pre>
          </div>
        {/if}

        <div class="panel-section">
          <h3>Тело</h3>
          {#if editing}
            <div class="body-tabs" role="tablist">
              <button type="button" class="body-tab" class:active={bodyTab === 'edit'} onclick={() => (bodyTab = 'edit')}>Правка</button>
              <button type="button" class="body-tab" class:active={bodyTab === 'preview'} onclick={() => (bodyTab = 'preview')}>Предпросмотр</button>
            </div>
            {#if bodyTab === 'edit'}
              <div class="format-toolbar">
                {#each FORMAT_ACTIONS as action (action)}
                  <button
                    type="button"
                    class="format-btn"
                    title={FORMAT_LABELS[action][1]}
                    aria-label={FORMAT_LABELS[action][1]}
                    onmousedown={(e) => e.preventDefault()}
                    onclick={() => insertFormat(action)}
                  >
                    {FORMAT_LABELS[action][0]}
                  </button>
                {/each}
              </div>
              <textarea
                class="field-input textarea body-textarea"
                bind:value={draft.body}
                bind:this={bodyTextarea}
                rows="10"
              ></textarea>
            {:else}
              <div class="markdown-body body-preview">{@html renderMarkdown(draft.body)}</div>
            {/if}
          {:else if detail.body}
            <div class="markdown-body">{@html renderMarkdown(detail.body)}</div>
          {:else}
            <p class="panel-status">(пусто)</p>
          {/if}
        </div>

        {#if editing}
          {#if saveError}
            <div class="conflict-banner" role="alert">
              <p>{saveError}</p>
              {#if conflict}
                <div class="panel-actions">
                  <button type="button" class="btn" onclick={refreshAfterConflict}>Обновить и повторить</button>
                  <button type="button" class="btn" onclick={discardAfterConflict}>Отменить мою правку</button>
                </div>
              {/if}
            </div>
          {/if}
          <div class="panel-actions">
            <button type="button" class="btn btn-primary" onclick={save} disabled={saving}>
              {saving ? 'Сохранение…' : 'Сохранить'}
            </button>
            <button type="button" class="btn" onclick={cancelEdit} disabled={saving}>Отмена</button>
          </div>
        {/if}
      {/if}
    </aside>
  </div>
{/if}
