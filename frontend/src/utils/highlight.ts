// highlight.js 唯一引入点: lib/common 只打包常用语言 (~40 种), 比全量小很多;
// markdown 渲染与其他直接做 JSON 高亮的地方共用同一实例
import hljs from 'highlight.js/lib/common';

export default hljs;
