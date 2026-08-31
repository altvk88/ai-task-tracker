<script>
  import Card from './Card.svelte';

  let { laneData } = $props();
</script>

<div class="lane">
  <div class="lane-head">
    <span class="lane-title">{laneData.lane}</span>
    <span class="lane-count">{laneData.tasks.length}</span>
  </div>
  <div class="lane-body">
    {#each laneData.tasks as task (task.path)}
      <Card {task} />
    {/each}
  </div>
</div>
