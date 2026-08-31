import { Plugin, type WorkspaceLeaf } from "obsidian";
import { BoardView, VIEW_TYPE_TT_BOARD } from "./board.ts";

export default class TtBoardPlugin extends Plugin {
  async onload(): Promise<void> {
    this.registerView(VIEW_TYPE_TT_BOARD, (leaf: WorkspaceLeaf) => new BoardView(leaf));

    this.addRibbonIcon("layout-grid", "TT Board", () => void this.activate());
    this.addCommand({
      id: "open-tt-board",
      name: "Открыть доску задач",
      callback: () => void this.activate(),
    });
  }

  private async activate(): Promise<void> {
    const existing = this.app.workspace.getLeavesOfType(VIEW_TYPE_TT_BOARD);
    if (existing.length > 0) {
      await this.app.workspace.revealLeaf(existing[0]);
      return;
    }
    const leaf = this.app.workspace.getLeaf("tab");
    await leaf.setViewState({ type: VIEW_TYPE_TT_BOARD, active: true });
    await this.app.workspace.revealLeaf(leaf);
  }
}
