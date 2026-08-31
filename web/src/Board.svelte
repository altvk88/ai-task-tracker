<script>
  import Lane from './Lane.svelte';
  import { lanes } from './store.js';
</script>

<div class="board">
  {#each $lanes as l (l.status)}
    <Lane laneData={l} />
  {/each}
</div>
