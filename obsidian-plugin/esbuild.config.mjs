// Сборка плагина в один main.js. Obsidian предоставляет свой рантайм,
// поэтому сам модуль obsidian и электронные встроенные модули не бандлим.
import esbuild from "esbuild";

const watch = process.argv.includes("--watch");

const ctx = await esbuild.context({
  entryPoints: ["src/main.ts"],
  bundle: true,
  external: ["obsidian", "electron"],
  format: "cjs",
  target: "es2022",
  logLevel: "info",
  sourcemap: watch ? "inline" : false,
  minify: !watch,
  outfile: "main.js",
});

if (watch) {
  await ctx.watch();
} else {
  await ctx.rebuild();
  await ctx.dispose();
}
