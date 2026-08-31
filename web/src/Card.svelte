<script>
  // Карточка таски. За данными в сеть не ходит — всё уже посчитано в store.js
  // (isOldClosed, liveBlockers); в store же уходит и перенос при перетаскивании.
  import { moveTask, selectedId } from './store.js';

  let { task } = $props();

  let priority = $derived(task.priority || 'none');
  let blockedTitle = $derived(
    task.liveBlockers?.length ? `Заблокировано: ${task.liveBlockers.join(', ')}` : ''
  );
  let busyTitle = $derived(task.claim?.agent ? `Занято агентом ${task.claim.agent}` : 'Занято');

  // Порог, отделяющий перетаскивание от клика: без него дрожание руки на
  // тачскрине превращает каждое касание в жест.
  const DRAG_THRESHOLD = 5;

  let el;
  let dragged = false;

  // Перетаскивание на pointer-событиях, а не на HTML5 DnD: доску открывают с
  // телефона, а там drag-and-drop браузера не работает вовсе.
  function pointerdown(down) {
    if (down.button !== 0) return;
    // Флаг мог остаться от прошлого жеста, завершившегося pointercancel:
    // после него браузер click не шлёт, и следующий честный клик по карточке
    // был бы съеден. Поэтому сброс — на входе в жест, а не после него.
    dragged = false;

    const startX = down.clientX;
    const startY = down.clientY;
    let ghost = null;
    let target = null;

    const move = (ev) => {
      if (!ghost) {
        if (Math.hypot(ev.clientX - startX, ev.clientY - startY) < DRAG_THRESHOLD) return;
        el.classList.add('dragging');
        ghost = el.cloneNode(true);
        ghost.classList.add('drag-ghost');
        document.body.appendChild(ghost);
        el.setPointerCapture(down.pointerId);
      }
      ghost.style.left = `${ev.clientX - 100}px`;
      ghost.style.top = `${ev.clientY - 16}px`;

      const lane = document.elementFromPoint(ev.clientX, ev.clientY)?.closest('.lane') ?? null;
      if (lane !== target) {
        target?.classList.remove('drop-target');
        target = lane;
        target?.classList.add('drop-target');
      }
    };

    const up = () => {
      el.removeEventListener('pointermove', move);
      el.removeEventListener('pointerup', up);
      el.removeEventListener('pointercancel', up);
      el.classList.remove('dragging');
      const to = target?.dataset.status;
      target?.classList.remove('drop-target');
      target = null;
      if (!ghost) return;
      ghost.remove();
      ghost = null;
      // Браузер шлёт click после отпускания — гасим его, иначе бросок мимо
      // лейнов открыл бы карточку.
      dragged = true;
      if (to) void moveTask(task.id, to);
    };

    el.addEventListener('pointermove', move);
    el.addEventListener('pointerup', up);
    el.addEventListener('pointercancel', up);
  }

  function select() {
    if (dragged) {
      dragged = false;
      return;
    }
    selectedId.set(task.id);
  }
</script>

<div
  class="card"
  class:selected={$selectedId === task.id}
  data-priority={priority}
  bind:this={el}
  role="button"
  tabindex="0"
  onpointerdown={pointerdown}
  onclick={select}
  onkeydown={(e) => e.key === 'Enter' && select()}
>
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
