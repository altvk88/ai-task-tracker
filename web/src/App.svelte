<script>
  import { onMount } from 'svelte';
  import { snapshot, loadSnapshot } from './store.js';
  import Toolbar from './Toolbar.svelte';
  import Board from './Board.svelte';

  onMount(loadSnapshot);
</script>

<header class="app-header">
  <h1>tt</h1>
  <Toolbar />
</header>

{#if $snapshot.loading}
  <p class="status">Загрузка…</p>
{:else if $snapshot.error}
  <p class="status error">Ошибка загрузки: {$snapshot.error}</p>
{:else}
  <Board />
{/if}
