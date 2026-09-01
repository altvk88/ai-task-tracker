// Рендер тела таски (markdown) в HTML для панели.
//
// Парсер — marked: из коробки поддерживает GFM (таблицы, чек-листы,
// зачёркивание), это покрывает всё, что реально встречается в тасках
// (заголовки, списки, чек-боксы, таблицы, код, ссылки, цитаты) без
// написания собственного парсера.
//
// Проверено на практике, а не только по описанию пакета: альтернатива
// micromark + micromark-extension-gfm (модульный парсер, «тяни только
// нужные токенайзеры») в реальной сборке Vite дала больший бандл — 132 КБ
// против 101 КБ у marked, потому что gfm тянет за собой сразу все свои
// подпакеты (footnote, autolink, strikethrough, tagfilter, table) и
// служебные micromark-util-* с таблицами символов, которые Vite не
// вырезает. marked — один пакет без такого шлейфа, поэтому и легче.
//
// Сырой HTML внутри markdown НЕ выполняется: и блочный, и инлайновый HTML
// в marked проходят через один и тот же Renderer.html(token) — здесь он
// экранирует текст вместо вставки как есть. Отдельный санитайзер (DOMPurify
// и т.п.) не нужен: просто не пропускаем сырую разметку дальше escapeHtml.
//
// Ссылки: marked с версии 5 сам больше не фильтрует схемы href (только
// encodeURI), поэтому `javascript:`/`vbscript:` пропустил бы как есть —
// renderer.link() ниже отклоняет любую схему кроме http(s)/mailto/относительных.
//
// Переопределения переданы объектом (не подклассом Renderer): marked.use()
// копирует именно перечислимые свойства объекта, а методы класса в
// прототипе таковыми не являются — с классом почти все переопределения
// молча не подключились бы.
import { Marked } from 'marked';

function escapeHtml(text) {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

const SAFE_SCHEME = /^(https?|mailto):/i;
const HAS_SCHEME = /^[a-z][a-z0-9+.-]*:/i;

const md = new Marked({
  gfm: true,
  breaks: false,
  renderer: {
    // И блочный <div>, и инлайновый <span onclick=...> HTML из тела таски
    // попадают сюда — рендерим как текст, не как разметку.
    html(token) {
      return escapeHtml(token.text);
    },
    // `false` — сигнал marked.use() отрендерить ссылку своим стандартным
    // способом; отклоняем только заведомо опасные/незнакомые схемы.
    link(token) {
      const href = token.href || '';
      if (HAS_SCHEME.test(href) && !SAFE_SCHEME.test(href)) {
        return this.parser.parseInline(token.tokens);
      }
      return false;
    },
    // Та же проверка схемы для картинок. `<img src="javascript:...">` в
    // современных браузерах инертен, но оставлять дыру того же класса,
    // которую закрыли у ссылок, незачем — фильтр один и тот же.
    image(token) {
      const href = token.href || '';
      if (HAS_SCHEME.test(href) && !SAFE_SCHEME.test(href)) {
        return escapeHtml(token.text || '');
      }
      return false;
    },
  },
});

// Длинную таблицу или блок кода прокручиваем внутри себя, а не раздвигаем
// панель — см. `.markdown-body table`/`pre` в app.css (display:block +
// overflow-x:auto на самой таблице избавляет от обёрточного <div>).

// `[[вики-ссылки]]` (наследие Obsidian) в теле тасок встречаются редко и
// без единой схемы разрешения адресов — превращать их в кликабельные ссылки
// внутри доски незачем. GFM и так не даёт им особого смысла, поэтому они
// просто остаются видимым текстом — простой и достаточный ответ.

export function renderMarkdown(text) {
  if (!text) return '';
  return md.parse(text);
}
