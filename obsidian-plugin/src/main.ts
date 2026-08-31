import { Plugin, Notice } from "obsidian";

export default class TtBoardPlugin extends Plugin {
  async onload(): Promise<void> {
    this.addCommand({
      id: "tt-board-ping",
      name: "TT Board: проверка загрузки",
      callback: () => new Notice("TT Board загружен"),
    });
    console.log("tt-board: загружен");
  }
}
