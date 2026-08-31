import { ItemView, type WorkspaceLeaf, TFile, Notice } from "obsidian";
import { type Schema, parseSchema, normalize } from "./schema.ts";
import { toTask, buildLanes, type Task, type Lane } from "./lanes.ts";
import { checkTransition } from "./guards.ts";
import { isLocked, applyStatus } from "./write.ts";

export const VIEW_TYPE_TT_BOARD = "tt-board";

const SCHEMA_PATH = ".task-tracker/schema.json";
const TASKS_PREFIX = "tasks/";

export class BoardView extends ItemView {
  private schema: Schema | null = null;
  private filterProject = "";
  private query = "";
  private redrawQueued = false;

  constructor(leaf: WorkspaceLeaf) {
    super(leaf);
  }

  getViewType(): string {
    return VIEW_TYPE_TT_BOARD;
  }

  getDisplayText(): string {
    return "TT Board";
  }

  getIcon(): string {
    return "layout-grid";
  }

  async onOpen(): Promise<void> {
    await this.loadSchema();
    // Живое обновление: Obsidian сам держит metadataCache актуальным, поэтому правка
    // таски агентом из терминала доезжает до доски без всякого вотчера.
    this.registerEvent(this.app.metadataCache.on("changed", () => this.queueRedraw()));
    this.registerEvent(this.app.vault.on("delete", () => this.queueRedraw()));
    this.registerEvent(this.app.vault.on("rename", () => this.queueRedraw()));
    this.render();
  }

  private async loadSchema(): Promise<void> {
    try {
      const raw = await this.app.vault.adapter.read(SCHEMA_PATH);
      this.schema = parseSchema(raw);
    } catch (e) {
      this.schema = null;
      new Notice(`TT Board: не читается ${SCHEMA_PATH} — ${(e as Error).message}`);
    }
  }

  /** Правки прилетают пачками; перерисовываем один раз за кадр. */
  private queueRedraw(): void {
    if (this.redrawQueued) return;
    this.redrawQueued = true;
    window.requestAnimationFrame(() => {
      this.redrawQueued = false;
      this.render();
    });
  }

  private collect(): Task[] {
    const tasks: Task[] = [];
    for (const file of this.app.vault.getMarkdownFiles()) {
      if (!file.path.startsWith(TASKS_PREFIX)) continue;
      if (file.path.split("/")[1]?.startsWith("_")) continue;
      const fm = this.app.metadataCache.getFileCache(file)?.frontmatter;
      if (!fm) continue;
      tasks.push(toTask(file.path, fm as Record<string, unknown>));
    }
    return tasks;
  }

  private render(): void {
    const root = this.contentEl;
    root.empty();
    root.addClass("tt-board");
    if (!this.schema) {
      root.createEl("p", { text: `TT Board: нет ${SCHEMA_PATH}` });
      return;
    }

    const tasks = this.collect();
    const byId = new Map(tasks.filter((t) => t.id).map((t) => [t.id, t]));
    this.renderToolbar(root, tasks);

    const lanes = buildLanes(this.schema, tasks, {
      project: this.filterProject || undefined,
      query: this.query || undefined,
    });
    const board = root.createDiv({ cls: "tt-lanes" });
    for (const l of lanes) this.renderLane(board, l, byId);
  }

  private renderToolbar(root: HTMLElement, tasks: Task[]): void {
    const bar = root.createDiv({ cls: "tt-toolbar" });

    const projects = [...new Set(tasks.map((t) => t.project).filter(Boolean))].sort();
    const select = bar.createEl("select");
    select.createEl("option", { value: "", text: "все проекты" });
    for (const p of projects) select.createEl("option", { value: p, text: p });
    select.value = this.filterProject;
    select.onchange = () => {
      this.filterProject = select.value;
      this.render();
    };

    const search = bar.createEl("input", { type: "search", placeholder: "поиск по ID и заголовку" });
    search.value = this.query;
    search.oninput = () => {
      this.query = search.value;
      this.render();
    };

    bar.createSpan({ cls: "tt-count", text: `${tasks.length} тасок` });
  }

  private renderLane(board: HTMLElement, l: Lane, byId: Map<string, Task>): void {
    const el = board.createDiv({ cls: "tt-lane" });
    el.dataset.status = l.status;
    el.createDiv({ cls: "tt-lane-title", text: `${l.lane} · ${l.tasks.length}` });
    const list = el.createDiv({ cls: "tt-lane-list" });
    for (const t of l.tasks) this.renderCard(list, t, byId);
  }

  private renderCard(list: HTMLElement, t: Task, byId: Map<string, Task>): void {
    const card = list.createDiv({ cls: "tt-card" });
    card.dataset.path = t.path;
    if (t.priority) card.addClass(`tt-prio-${t.priority}`);

    const head = card.createDiv({ cls: "tt-card-head" });
    head.createSpan({ cls: "tt-id", text: t.id || t.path.split("/").pop() || "?" });
    if (t.claimed) {
      head.createSpan({ cls: "tt-claim", text: "◉", title: t.claimAgent || t.claimRaw || "занята" });
    }
    const blockers = t.blockedBy.filter((id) => {
      const b = byId.get(id);
      return !b || (b.status !== "done" && b.status !== "cancelled");
    });
    if (blockers.length > 0) {
      head.createSpan({ cls: "tt-lock", text: "🔒", title: `ждёт: ${blockers.join(", ")}` });
    }
    if (t.effort) head.createSpan({ cls: "tt-effort", text: t.effort });

    card.createDiv({ cls: "tt-title", text: t.title || "(без заголовка)" });

    // Клик открывает файл таски рядом — нативная сила Obsidian, которой не будет у веб-доски.
    card.onclick = () => {
      if (card.dataset.dragged === "1") {
        delete card.dataset.dragged;
        return;
      }
      const file = this.app.vault.getAbstractFileByPath(t.path);
      if (file instanceof TFile) void this.app.workspace.getLeaf("split").openFile(file);
    };

    this.makeDraggable(card, t.path);
  }

  /** Перетаскивание на pointer-событиях: работает и мышью, и пальцем. */
  private makeDraggable(card: HTMLElement, path: string): void {
    card.onpointerdown = (down: PointerEvent) => {
      if (down.button !== 0) return;
      // Флаг мог остаться от прошлого жеста, завершившегося без click (pointercancel):
      // иначе следующий честный клик по карточке был бы съеден.
      delete card.dataset.dragged;
      const startX = down.clientX;
      const startY = down.clientY;
      let ghost: HTMLElement | null = null;
      let target: HTMLElement | null = null;

      const move = (ev: PointerEvent): void => {
        if (!ghost) {
          // Порог в 5 пикселей отделяет перетаскивание от клика.
          if (Math.hypot(ev.clientX - startX, ev.clientY - startY) < 5) return;
          card.addClass("tt-dragging");
          ghost = card.cloneNode(true) as HTMLElement;
          ghost.addClass("tt-drag-ghost");
          document.body.appendChild(ghost);
          card.setPointerCapture(down.pointerId);
        }
        ghost.style.left = `${ev.clientX - 100}px`;
        ghost.style.top = `${ev.clientY - 16}px`;

        const under = document.elementFromPoint(ev.clientX, ev.clientY);
        const lane = (under?.closest(".tt-lane") ?? null) as HTMLElement | null;
        if (lane !== target) {
          target?.removeClass("tt-drop-target");
          target = lane;
          target?.addClass("tt-drop-target");
        }
      };

      const up = (): void => {
        card.onpointermove = null;
        card.onpointerup = null;
        card.onpointercancel = null;
        card.removeClass("tt-dragging");
        const dragged = ghost !== null;
        ghost?.remove();
        ghost = null;
        target?.removeClass("tt-drop-target");
        const to = target?.dataset.status;
        target = null;
        if (dragged) {
          // Браузер шлёт click после отпускания — гасим его, иначе откроется файл.
          card.dataset.dragged = "1";
          if (to) void this.moveTask(path, to);
        }
      };

      card.onpointermove = move;
      card.onpointerup = up;
      card.onpointercancel = up;
    };
  }

  /** Переносит таску в другой статус со всеми проверками. Вызывается из drag-and-drop. */
  private async moveTask(path: string, to: string): Promise<void> {
    if (!this.schema) return;
    const file = this.app.vault.getAbstractFileByPath(path);
    if (!(file instanceof TFile)) return;

    const tasks = this.collect();
    const byId = new Map(tasks.filter((t) => t.id).map((t) => [t.id, t]));
    const fm = this.app.metadataCache.getFileCache(file)?.frontmatter;
    const task = toTask(path, fm as Record<string, unknown>);

    const from = normalize(this.schema, task.status).id;
    const target = normalize(this.schema, to).id;
    if (from === target) return;

    const locked = await isLocked(this.app, task.id);
    const err = checkTransition(this.schema, task, target, byId, locked);
    if (err) {
      new Notice(err);
      this.render();
      return;
    }
    try {
      await applyStatus(this.app, this.schema, file, task.id, from, target);
    } catch (e) {
      new Notice(`не удалось записать: ${(e as Error).message}`);
      this.render();
    }
  }
}
