<script>
  // Форма создания таски с доски (TT-054): открывается кнопкой в Toolbar,
  // отправляет ровно то, что принимает POST /api/task (см. create.go), и
  // после успеха открывает панель новой таски.
  import { creating, projects, selectedId, patchTask, showNotice, authHeaders } from './store.js';
  import { parseDependsOn } from './edit.js';

  let project = $state('');
  let title = $state('');
  let priority = $state('medium');
  let effort = $state('');
  let dependsOn = $state('');
  let submitting = $state(false);
  let error = $state('');

  function reset() {
    project = '';
    title = '';
    priority = 'medium';
    effort = '';
    dependsOn = '';
    error = '';
  }

  function close() {
    creating.set(false);
    reset();
  }

  async function submit(e) {
    e.preventDefault();
    if (!project.trim() || !title.trim()) return;
    submitting = true;
    error = '';
    try {
      const res = await fetch('/api/task', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...authHeaders() },
        body: JSON.stringify({
          project: project.trim(),
          title: title.trim(),
          priority,
          effort,
          dependsOn: parseDependsOn(dependsOn),
        }),
      });
      const data = await res.json().catch(() => null);
      if (!res.ok) throw new Error(data?.error || `сервер ответил ${res.status}`);
      patchTask(data.id, data);
      if (data.warnings?.length) showNotice(`${data.id}: ${data.warnings.join('; ')}`);
      selectedId.set(data.id);
      close();
    } catch (err) {
      error = err.message;
    } finally {
      submitting = false;
    }
  }
</script>

{#if $creating}
  <div class="panel-backdrop" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
    <aside class="panel" aria-label="Новая таска">
      <div class="panel-head">
        <h2 class="panel-title">Новая таска</h2>
        <button type="button" class="panel-close" onclick={close} aria-label="Закрыть">×</button>
      </div>

      <form onsubmit={submit} class="create-form">
        <label class="form-row">
          <span>Проект</span>
          <input class="field-input" list="create-task-projects" bind:value={project} required />
          <datalist id="create-task-projects">
            {#each $projects as p (p)}<option value={p}></option>{/each}
          </datalist>
        </label>

        <label class="form-row">
          <span>Заголовок</span>
          <input class="field-input" bind:value={title} required />
        </label>

        <label class="form-row">
          <span>Приоритет</span>
          <select class="field-input" bind:value={priority}>
            <option value="high">high</option>
            <option value="medium">medium</option>
            <option value="low">low</option>
          </select>
        </label>

        <label class="form-row">
          <span>Оценка</span>
          <input class="field-input" bind:value={effort} placeholder="2h, 1d…" />
        </label>

        <label class="form-row">
          <span>Зависимости</span>
          <input class="field-input" bind:value={dependsOn} placeholder="TT-001, TT-002" />
        </label>

        {#if error}<p class="panel-status error">{error}</p>{/if}

        <div class="panel-actions">
          <button type="submit" class="btn btn-primary" disabled={submitting}>
            {submitting ? 'Создание…' : 'Создать'}
          </button>
          <button type="button" class="btn" onclick={close} disabled={submitting}>Отмена</button>
        </div>
      </form>
    </aside>
  </div>
{/if}
