<script>
  // Фильтр по проекту, поиск по ID/заголовку и переключатель старых закрытых.
  // Все изменения идут через store.js — сам тулбар состояния не хранит.
  import { filter, projects, visibleTasks } from './store.js';
</script>

<div class="toolbar">
  <select
    value={$filter.project}
    onchange={(e) => filter.update((f) => ({ ...f, project: e.target.value }))}
  >
    <option value="">Все проекты</option>
    {#each $projects as p (p)}
      <option value={p}>{p}</option>
    {/each}
  </select>

  <input
    class="search"
    type="search"
    placeholder="Поиск по ID или заголовку…"
    value={$filter.query}
    oninput={(e) => filter.update((f) => ({ ...f, query: e.target.value }))}
  />

  <label class="toggle">
    <input
      type="checkbox"
      checked={$filter.showOldClosed}
      onchange={(e) => filter.update((f) => ({ ...f, showOldClosed: e.target.checked }))}
    />
    показывать старые закрытые
  </label>

  <span class="counter">Показано: {$visibleTasks.length}</span>
</div>
