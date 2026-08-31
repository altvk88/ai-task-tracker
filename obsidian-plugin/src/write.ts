// Запись статуса ЧЕРЕЗ API Obsidian, а не мимо него. Это ключевое отличие от прежней
// схемы с файловыми досками: раньше скрипты писали в файл, который Obsidian держал
// открытым, и он перезаписывал его своей копией, ломая фронтматтер. processFrontMatter
// проходит через тот же слой, что и редактор, поэтому конфликт исключён по конструкции.
import { type App, type TFile, normalizePath } from "obsidian";
import { type Schema, clearsClaimOn, setsCompletedOn, setsReadyAtOn } from "./schema.ts";

// Дата локальная, как в Go (`time.Now().Format("2006-01-02")`): даты в одни и те же
// таски пишут и `tt`, и плагин, а UTC вечером по Москве дал бы вчерашнее число.
const today = (): string => {
  const d = new Date();
  const pad = (n: number): string => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};

const lockPath = (id: string): string => normalizePath(`.locks/${id}.lock`);

/** Совещательная проверка замка: атомарности от Obsidian не требуется и не предполагается. */
export async function isLocked(app: App, id: string): Promise<boolean> {
  if (id === "") return false;
  return app.vault.adapter.exists(lockPath(id));
}

/**
 * Меняет статус таски и приводит в порядок парные поля по правилам схемы:
 * completed/ready_at проставляются, claim снимается.
 *
 * Замок — долгоживущий признак «таска в работе», парный статусу, а не операции:
 * ставится при взятии в работу, снимается при уходе из неё, при прочих переходах
 * не трогается.
 */
export async function applyStatus(
  app: App,
  schema: Schema,
  file: TFile,
  id: string,
  from: string,
  to: string,
): Promise<void> {
  await app.fileManager.processFrontMatter(file, (fm: Record<string, unknown>) => {
    fm["status"] = to;
    // Уже заполненные даты не перебиваем: первая достовернее последней.
    if (setsCompletedOn(schema, to) && !fm["completed"]) fm["completed"] = today();
    if (setsReadyAtOn(schema, to) && !fm["ready_at"]) fm["ready_at"] = today();
    // Именно delete, а не присваивание null: null Obsidian может сериализовать
    // строкой `claim: null`, и Go-модель прочитала бы её как непустой скаляр,
    // то есть сочла бы таску занятой.
    if (clearsClaimOn(schema, to)) delete fm["claim"];
  });

  if (id === "") return;

  const p = lockPath(id);
  if (from === "in-progress" && to !== "in-progress") {
    if (await app.vault.adapter.exists(p)) {
      await app.vault.adapter.rmdir(p, false);
    }
  } else if (to === "in-progress" && from !== "in-progress") {
    if (!(await app.vault.adapter.exists(p))) {
      await app.vault.adapter.mkdir(p);
    }
  }
}
