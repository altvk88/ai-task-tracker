import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Собираем в web/dist — этот каталог вшивается в бинарник через go:embed
// (internal/server/static.go). base: './' — чтобы бандл открывался с любого
// пути, а не только с корня (сервер отдаёт его на всех путях, кроме /api).
export default defineConfig({
  plugins: [svelte()],
  base: './',
  build: {
    outDir: 'dist',
    // Каталог не чистим: в нём лежит .gitkeep — единственный отслеживаемый
    // файл в dist, нужный чтобы go:embed не падал на чистом клоне без
    // собранного фронта. С emptyOutDir: true vite удалял его при каждой
    // сборке, и дерево пачкалось изменением, которое коммитить нельзя.
    emptyOutDir: false,
    // Имена без хеша — обязательное следствие emptyOutDir: false. С хешами
    // каждая сборка добавляла в dist новый файл, старые оставались, и
    // go:embed all:dist вшивал их все: за десяток сборок набежало 424 КБ
    // мёртвых бандлов вместо 57. Кэш-бастинг здесь и не нужен — бандл вшит
    // в бинарник и отдаётся локально.
    rollupOptions: {
      output: {
        entryFileNames: 'assets/index.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: 'assets/[name][extname]',
      },
    },
  },
});
