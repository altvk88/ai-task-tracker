<script>
  // Страница «Пульс» — сводка по снимку вместо dashboard.md и Dataview.
  // Сам подсчёт — чистая функция computePulse в pulse.js, здесь только вывод.
  import { pulseData, selectedId } from './store.js';
</script>

{#if $pulseData}
  <div class="pulse">
    <section class="pulse-block">
      <h2>В работе <span class="pulse-count">{$pulseData.inProgress.length}</span></h2>
      {#if $pulseData.inProgress.length}
        <ul class="pulse-list">
          {#each $pulseData.inProgress as t (t.id)}
            <li>
              <button type="button" class="pulse-link" onclick={() => selectedId.set(t.id)}>{t.id}</button>
              <span class="pulse-muted">{t.project}</span>
              <span class="pulse-muted">{t.agent || '— без claim'}</span>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="pulse-empty">Ничего не в работе.</p>
      {/if}
    </section>

    <section class="pulse-block">
      <h2>Ready по проектам</h2>
      {#if $pulseData.readyByProject.length}
        <table class="pulse-table">
          <tbody>
            {#each $pulseData.readyByProject as row (row.project)}
              <tr>
                <td>{row.project}</td>
                <td class="pulse-num">{row.count}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {:else}
        <p class="pulse-empty">Готовых к работе тасок нет.</p>
      {/if}
    </section>

    <section class="pulse-block">
      <h2>Needs input <span class="pulse-count">{$pulseData.needsInput.length}</span></h2>
      {#if $pulseData.needsInput.length}
        <ul class="pulse-list">
          {#each $pulseData.needsInput as t (t.id)}
            <li>
              <button type="button" class="pulse-link" onclick={() => selectedId.set(t.id)}>{t.id}</button>
              <span class="pulse-muted">{t.project}</span>
              <span>{t.title}</span>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="pulse-empty">Разбирать руками нечего.</p>
      {/if}
    </section>

    <section class="pulse-block">
      <h2>Непарсящиеся <span class="pulse-count">{$pulseData.broken}</span></h2>
      <p class="pulse-empty">
        {#if $pulseData.broken}
          Список и причины — в `tt doctor`.
        {:else}
          Битых тасок нет.
        {/if}
      </p>
    </section>
  </div>
{/if}
