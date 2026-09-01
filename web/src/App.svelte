<script>
  import { onMount } from 'svelte';
  import { snapshot, loadSnapshot, startLiveUpdates, notice, dismissNotice, view } from './store.js';
  import Toolbar from './Toolbar.svelte';
  import Board from './Board.svelte';
  import Pulse from './Pulse.svelte';
  import TaskPanel from './TaskPanel.svelte';
  import CreateTask from './CreateTask.svelte';

  // Снимок и подписка на изменения поднимаются вместе; возвращённая функция
  // закрывает поток при размонтировании.
  onMount(() => {
    void loadSnapshot();
    return startLiveUpdates();
  });
</script>

<header class="app-header">
  <h1>tt</h1>
  <nav class="view-tabs">
    <button type="button" class:active={$view === 'board'} onclick={() => view.set('board')}>Доска</button>
    <button type="button" class:active={$view === 'pulse'} onclick={() => view.set('pulse')}>Пульс</button>
  </nav>
  {#if $view === 'board'}<Toolbar />{/if}
</header>

{#if $notice}
  <div class="notice" role="alert">
    <span>{$notice}</span>
    <button type="button" onclick={dismissNotice} aria-label="Закрыть">×</button>
  </div>
{/if}

{#if $snapshot.loading}
  <p class="status">Загрузка…</p>
{:else if $snapshot.error}
  <p class="status error">Ошибка загрузки: {$snapshot.error}</p>
{:else if $view === 'pulse'}
  <Pulse />
{:else}
  <Board />
{/if}

<TaskPanel />
<CreateTask />
