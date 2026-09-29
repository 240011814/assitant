import DOMPurify from 'dompurify';
import MarkdownIt from 'markdown-it';
import texmath from 'markdown-it-texmath';
import katex from 'katex';
import 'katex/dist/katex.min.css';

const md = new MarkdownIt({
  html: true,
  linkify: true,
  typographer: true
}).use(texmath, { engine: katex, delimiters: 'dollars' });

/** 渲染 markdown 并用 DOMPurify 清洗输出 (保留 KaTeX 所需的 svg/mathml), 防止存储型 XSS */
export function renderMarkdown(content: string): string {
  const html = md.render(content);
  return DOMPurify.sanitize(html, { USE_PROFILES: { html: true, svg: true, mathMl: true } });
}
