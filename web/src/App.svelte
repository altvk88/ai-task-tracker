<script>
  import { onMount } from 'svelte';
  import { snapshot, loadSnapshot, startLiveUpdates, notice, dismissNotice } from './store.js';
  import Toolbar from './Toolbar.svelte';
  import Board from './Board.svelte';

  // Снимок и подписка на изменения поднимаются вместе; возвращённая функция
  // закрывает поток при размонтировании.
  onMount(() => {
    void loadSnapshot();
    return startLiveUpdates();
  });
</script>

<header class="app-header">
  <h1>tt</h1>
  <Toolbar />
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
{:else}
  <Board />
{/if}
