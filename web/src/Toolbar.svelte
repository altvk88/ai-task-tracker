<script>
  // Фильтр по проекту (дропдаун с чекбоксами, TT-062), поиск по ID/заголовку
  // и переключатель старых закрытых. Все изменения идут через store.js —
  // сам тулбар состояния не хранит, кроме того, открыт ли сам дропдаун.
  import { filter, projects, visibleTasks, creating } from './store.js';
  import { describeProjectSelection } from './view-state.js';

  let projectMenuOpen = $state(false);

  function toggleProject(p) {
    filter.update((f) => ({
      ...f,
      projects: f.projects.includes(p) ? f.projects.filter((x) => x !== p) : [...f.projects, p],
    }));
  }

  // Клик вне дропдауна и Escape закрывают его; слушатели висят только пока
  // он открыт — не плодим их на весь жизненный цикл тулбара.
  function onDocumentClick(e) {
    if (!e.target.closest('.project-filter')) projectMenuOpen = false;
  }
  function onDocumentKeydown(e) {
    if (e.key === 'Escape') projectMenuOpen = false;
  }
  $effect(() => {
    if (!projectMenuOpen) return;
    document.addEventListener('click', onDocumentClick);
    document.addEventListener('keydown', onDocumentKeydown);
    return () => {
      document.removeEventListener('click', onDocumentClick);
      document.removeEventListener('keydown', onDocumentKeydown);
    };
  });
</script>

<div class="toolbar">
  <div class="project-filter">
    <button
      type="button"
      class="btn"
      aria-haspopup="true"
      aria-expanded={projectMenuOpen}
      onclick={() => (projectMenuOpen = !projectMenuOpen)}
    >
      {describeProjectSelection($filter.projects, $projects)} ▾
    </button>
    {#if projectMenuOpen}
      <div class="project-menu" role="menu">
        {#if $projects.length === 0}
          <p class="project-empty">Нет проектов</p>
        {/if}
        {#each $projects as p (p)}
          <label class="project-option">
            <input
              type="checkbox"
              checked={$filter.projects.includes(p)}
              onchange={() => toggleProject(p)}
            />
            {p}
          </label>
        {/each}
      </div>
    {/if}
  </div>

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

  <button type="button" class="btn btn-primary" onclick={() => creating.set(true)}>+ Новая таска</button>

  <span class="counter">Показано: {$visibleTasks.length}</span>
</div>
