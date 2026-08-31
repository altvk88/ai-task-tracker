<script>
  // Карточка таски. Сама в сеть не ходит — все данные уже посчитаны в store.js
  // (isOldClosed, liveBlockers), Card их только показывает.
  let { task } = $props();

  let priority = $derived(task.priority || 'none');
  let blockedTitle = $derived(
    task.liveBlockers?.length ? `Заблокировано: ${task.liveBlockers.join(', ')}` : ''
  );
  let busyTitle = $derived(task.claim?.agent ? `Занято агентом ${task.claim.agent}` : 'Занято');
</script>

<div class="card" data-priority={priority}>
  <div class="card-head">
    <span class="card-id">{task.id}</span>
    {#if task.liveBlockers?.length}
      <span class="lock" title={blockedTitle}>🔒</span>
    {/if}
  </div>
  <div class="card-title">{task.title}</div>
  {#if task.effort || task.claim}
    <div class="card-foot">
      {#if task.effort}<span class="badge effort">{task.effort}</span>{/if}
      {#if task.claim}<span class="badge busy" title={busyTitle}>занято</span>{/if}
    </div>
  {/if}
</div>
