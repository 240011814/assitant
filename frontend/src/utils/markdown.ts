import DOMPurify from 'dompurify';
import MarkdownIt from 'markdown-it';
import texmath from 'markdown-it-texmath';
import katex from 'katex';
// lib/common 只打包常用语言 (~40 种), 比全量 highlight.js 小很多
import hljs from 'highlight.js/lib/common';
import 'katex/dist/katex.min.css';

/** 与 markdown-it utils.escapeHtml 等价的独立实现 (highlight 回调里不能引用 md 自身, 否则循环类型推断报 TS7022) */
const escapeHtml = (str: string): string =>
  str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');

const md = new MarkdownIt({
  html: true,
  linkify: true,
  typographer: true,
  highlight(code, lang): string {
    // 有语言标注且 hljs 认识时精确高亮; 不认识/无标注时转义输出 (不做 auto-detect, 免得误判+拖慢流式渲染)
    if (lang && hljs.getLanguage(lang)) {
      try {
        return `<pre class="hljs"><code>${hljs.highlight(code, { language: lang, ignoreIllegals: true }).value}</code></pre>`;
      } catch {
        /* 落到下方转义输出 */
      }
    }
    return `<pre class="hljs"><code>${escapeHtml(code)}</code></pre>`;
  }
}).use(texmath, { engine: katex, delimiters: 'dollars' });

/** 渲染 markdown 并用 DOMPurify 清洗输出 (保留 KaTeX 所需的 svg/mathml), 防止存储型 XSS */
export function renderMarkdown(content: string): string {
  const html = md.render(content);
  return DOMPurify.sanitize(html, { USE_PROFILES: { html: true, svg: true, mathMl: true } });
}
