<script>
  // Заглушка: заголовок и число тасок из /api/snapshot. Полноценная доска —
  // отдельная задача со своей приёмкой, здесь только проверяем, что бандл
  // собирается, вшивается и достаёт данные с бэкенда.
  let total = $state(null);
  let error = $state('');

  $effect(() => {
    fetch('/api/snapshot')
      .then((r) => r.json())
      .then((data) => {
        total = data.summary.total;
      })
      .catch((err) => {
        error = String(err);
      });
  });
</script>

<h1>tt</h1>
{#if error}
  <p>Ошибка загрузки: {error}</p>
{:else if total === null}
  <p>Загрузка…</p>
{:else}
  <p>Тасок в vault: {total}</p>
{/if}
