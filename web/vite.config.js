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
  },
});
