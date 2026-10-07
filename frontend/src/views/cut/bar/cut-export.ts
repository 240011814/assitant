import { jsPDF } from 'jspdf';
import { canvasToBlob, downloadBlob, formatFileTimestamp } from '@/utils/cut-export';

/**
 * 一维切割示意图绘制与导出
 *
 * 每根料一条水平条(按 totalLength 比例), 内部按 cuts 顺序分段,
 * 相邻段之间画锯缝线; 每段内标尺寸数字, 小段放段下方引线;
 * 条首标 "#index [类型] 总长", 聚合行附带 "×N根", 余料段灰色。
 *
 * PDF 打印按页独立渲染(横向 A4): 单根料的图不跨页断开, 每页带页眉
 * (标题/汇总/页码/日期), 分辨率高于整图切片方案, 文字更清晰;
 * 文字统一由 canvas 绘制, 规避 jsPDF 内置字体不支持中文的问题。
 */

/** 聚合模式下附带 count(同型同切法根数), 打印/导出与页面聚合展示保持一致 */
export type BarRow = Api.Cut.BarResult & { count?: number };

interface BarLabel {
  x: number;
  text: string;
  slot: number;
}

interface BarLayout {
  bar: BarRow;
  labels: BarLabel[];
  leaderRows: number;
}

/** 行绘制涉及的字号/尺寸, 以 1600px 宽设计稿为基准, PDF 页面按比例放大 */
interface BarStyle {
  labelSize: number;
  statSize: number;
  inlineSize: number;
  leaderSize: number;
  labelHeight: number;
  barHeight: number;
  rowGap: number;
  kerfWidth: number;
  /** 段内直标的最小像素宽, 低于该值放段下方引线 */
  minInlineWidth: number;
  /** 引线标注的最小像素宽, 低于该值不标(过窄) */
  minLeaderWidth: number;
  leaderGap: number;
  leaderStep: number;
  marginX: number;
}

const COLOR_POOL = Array.from({ length: 50 }, (_, i) => `hsl(${(i * 30) % 360}, 70%, 50%)`);

const CANVAS_WIDTH = 1600;
const MARGIN_Y = 20;
const TITLE_HEIGHT = 34;

const BASE_STYLE: BarStyle = {
  labelSize: 14,
  statSize: 12,
  inlineSize: 12,
  leaderSize: 10,
  labelHeight: 22,
  barHeight: 40,
  rowGap: 20,
  kerfWidth: 2,
  minInlineWidth: 44,
  minLeaderWidth: 12,
  leaderGap: 14,
  leaderStep: 16,
  marginX: 24
};

function fontCss(px: number, bold = false): string {
  return `${bold ? 'bold ' : ''}${px}px sans-serif`;
}

function scaleStyle(style: BarStyle, k: number): BarStyle {
  return {
    labelSize: style.labelSize * k,
    statSize: style.statSize * k,
    inlineSize: style.inlineSize * k,
    leaderSize: style.leaderSize * k,
    labelHeight: style.labelHeight * k,
    barHeight: style.barHeight * k,
    rowGap: style.rowGap * k,
    kerfWidth: Math.max(1, style.kerfWidth * k),
    minInlineWidth: style.minInlineWidth * k,
    minLeaderWidth: style.minLeaderWidth * k,
    leaderGap: style.leaderGap * k,
    leaderStep: style.leaderStep * k,
    marginX: style.marginX * k
  };
}

function fmt(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

function createMeasure(style: BarStyle): (text: string) => number {
  const canvas = document.createElement('canvas');
  const ctx = canvas.getContext('2d');
  return (text: string) => {
    if (!ctx) return text.length * style.leaderSize * 0.7;
    ctx.font = fontCss(style.leaderSize);
    return ctx.measureText(text).width;
  };
}

function leaderAreaHeight(layout: BarLayout, style: BarStyle): number {
  return layout.leaderRows > 0 ? style.leaderGap + layout.leaderRows * style.leaderStep : 0;
}

function rowHeight(layout: BarLayout, style: BarStyle): number {
  return style.labelHeight + style.barHeight + leaderAreaHeight(layout, style) + style.rowGap;
}

/** 行首标签: "#index [类型] 总长", 聚合行附带 "×N根" */
function rowLabelText(item: BarRow): string {
  let text = `#${item.index}`;
  const type = item.materialType?.trim();
  if (type) text += ` [${type}]`;
  text += ` 总长 ${fmt(item.totalLength)}cm`;
  const count = item.count ?? 1;
  if (count > 1) text += ` ×${count}根`;
  return text;
}

function layoutBar(item: BarRow, scale: number, measure: (text: string) => number, style: BarStyle): BarLayout {
  const labels: BarLabel[] = [];
  const slotEnds: number[] = [];

  const pushLabel = (center: number, text: string) => {
    const half = measure(text) / 2;
    let slot = slotEnds.findIndex(end => center - half > end + 6);
    if (slot === -1) {
      slotEnds.push(0);
      slot = slotEnds.length - 1;
    }
    slotEnds[slot] = center + half;
    labels.push({ x: center, text, slot });
  };

  let x = 0;
  item.cuts.forEach(cut => {
    const w = cut * scale;
    if (w < style.minInlineWidth && w >= style.minLeaderWidth) {
      pushLabel(x + w / 2, fmt(cut));
    }
    x += w;
  });
  if (item.remaining > 0) {
    const w = item.remaining * scale;
    if (w < style.minInlineWidth && w >= style.minLeaderWidth) {
      pushLabel(x + w / 2, `余${fmt(item.remaining)}`);
    }
  }

  return { bar: item, labels, leaderRows: slotEnds.length };
}

function drawBarRow(
  ctx: CanvasRenderingContext2D,
  layout: BarLayout,
  top: number,
  scale: number,
  barWidth: number,
  style: BarStyle
) {
  const item = layout.bar;
  const barTop = top + style.labelHeight;
  let x = style.marginX;

  // 条首标签 + 右侧统计
  ctx.font = fontCss(style.labelSize, true);
  ctx.fillStyle = '#1f2937';
  ctx.textAlign = 'left';
  ctx.fillText(rowLabelText(item), style.marginX, top + style.labelSize);
  ctx.font = fontCss(style.statSize);
  ctx.fillStyle = '#6b7280';
  ctx.textAlign = 'right';
  ctx.fillText(`已用 ${fmt(item.used)}cm | 余料 ${fmt(item.remaining)}cm`, style.marginX + barWidth, top + style.statSize);
  ctx.textAlign = 'center';

  item.cuts.forEach((cut, idx) => {
    const w = Math.max(cut * scale, 1);
    ctx.fillStyle = COLOR_POOL[idx % COLOR_POOL.length];
    ctx.fillRect(x, barTop, w, style.barHeight);
    ctx.strokeStyle = '#fff';
    ctx.lineWidth = 1;
    ctx.strokeRect(x + 0.5, barTop + 0.5, Math.max(w - 1, 0), style.barHeight - 1);
    // 相邻段之间画锯缝线
    if (idx > 0) {
      ctx.fillStyle = '#374151';
      ctx.fillRect(x - style.kerfWidth / 2, barTop, style.kerfWidth, style.barHeight);
    }
    if (w >= style.minInlineWidth) {
      ctx.fillStyle = '#fff';
      ctx.font = fontCss(style.inlineSize);
      ctx.fillText(fmt(cut), x + w / 2, barTop + style.barHeight / 2 + style.inlineSize * 0.35);
    }
    x += w;
  });

  // 余料段(灰色)
  if (item.remaining > 0) {
    const w = Math.max(item.remaining * scale, 1);
    ctx.fillStyle = '#c0c4cc';
    ctx.fillRect(x, barTop, w, style.barHeight);
    ctx.strokeStyle = '#9ca3af';
    ctx.lineWidth = 1;
    ctx.strokeRect(x + 0.5, barTop + 0.5, w - 1, style.barHeight - 1);
    if (w >= style.minInlineWidth) {
      ctx.fillStyle = '#333';
      ctx.font = fontCss(style.inlineSize);
      ctx.fillText(`余 ${fmt(item.remaining)}`, x + w / 2, barTop + style.barHeight / 2 + style.inlineSize * 0.35);
    }
  }

  // 段下方引线标注(小段)
  ctx.font = fontCss(style.leaderSize);
  layout.labels.forEach(lb => {
    const labelY = barTop + style.barHeight + style.leaderGap + lb.slot * style.leaderStep;
    ctx.strokeStyle = '#9ca3af';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(lb.x, barTop + style.barHeight);
    ctx.lineTo(lb.x, labelY - 3);
    ctx.stroke();
    ctx.fillStyle = '#374151';
    ctx.textAlign = 'center';
    ctx.fillText(lb.text, lb.x, labelY);
  });
}

function computeScale(results: BarRow[], barWidth: number): number {
  const maxTotal = Math.max(...results.map(r => r.totalLength));
  return maxTotal > 0 ? barWidth / maxTotal : 1;
}

/** 汇总概要文本(材料根数/零件件数/总长/利用率/余料), 聚合行按 count 折算零件数 */
function summaryText(results: BarRow[], summary?: Api.Cut.BarSummary | null): string {
  const pieceCount = results.reduce((sum, r) => sum + r.cuts.length * (r.count ?? 1), 0);
  if (summary) {
    return (
      `共 ${summary.materialCount} 根 | 零件 ${pieceCount} 件 | 材料总长 ${fmt(summary.totalMaterialLength)}cm | ` +
      `零件总长 ${fmt(summary.totalCutLength)}cm | 利用率 ${summary.utilization}% | 余料 ${fmt(summary.totalRemaining)}cm (${summary.scrapCount}根)`
    );
  }
  return `共 ${results.length} 根 | 零件 ${pieceCount} 件`;
}

/** 把一维切割结果画成切割示意图 canvas (单张长图, 用于 PNG 导出) */
export function drawBarCutChart(results: BarRow[], summary?: Api.Cut.BarSummary | null): HTMLCanvasElement {
  if (results.length === 0) {
    throw new Error('no bar cut results to draw');
  }

  const style = BASE_STYLE;
  const measure = createMeasure(style);
  const barWidth = CANVAS_WIDTH - style.marginX * 2;
  const scale = computeScale(results, barWidth);
  const layouts = results.map(item => layoutBar(item, scale, measure, style));
  const contentHeight = layouts.reduce((sum, l) => sum + rowHeight(l, style), 0);

  const canvas = document.createElement('canvas');
  canvas.width = CANVAS_WIDTH;
  canvas.height = Math.ceil(MARGIN_Y + TITLE_HEIGHT + contentHeight + MARGIN_Y);
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    throw new Error('canvas 2d context unavailable');
  }

  // 背景与标题
  ctx.fillStyle = '#fff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.font = fontCss(16, true);
  ctx.fillStyle = '#111827';
  ctx.textAlign = 'left';
  ctx.fillText(`切割示意图  ${summaryText(results, summary)}`, style.marginX, MARGIN_Y + 22);

  let y = MARGIN_Y + TITLE_HEIGHT;
  layouts.forEach(layout => {
    drawBarRow(ctx, layout, y, scale, barWidth, style);
    y += rowHeight(layout, style);
  });
  return canvas;
}

/** 导出切割示意图 PNG */
export async function exportBarCutPNG(results: BarRow[], summary?: Api.Cut.BarSummary | null): Promise<void> {
  const canvas = drawBarCutChart(results, summary);
  const blob = await canvasToBlob(canvas);
  downloadBlob(blob, `切割图-${formatFileTimestamp()}.png`);
}

// ---- PDF 打印: 横向 A4 按页独立渲染 ----

/** 页面画布像素/pt (2.8*72 ≈ 202dpi) */
const PAGE_PPI = 2.8;
const PAGE_MARGIN_PT = 18;

/**
 * 生成切割示意图 PDF (横向 A4)
 *
 * 分页规则: 以行为单位贪心排版, 单根料的图(含引线标注)不跨页断开;
 * 每页绘制页眉(标题/汇总/页码/日期)后从完整一行继续。
 */
export function buildBarCutPDF(results: BarRow[], summary?: Api.Cut.BarSummary | null): jsPDF {
  if (results.length === 0) {
    throw new Error('no bar cut results to draw');
  }

  const pdf = new jsPDF({ orientation: 'landscape', unit: 'pt', format: 'a4', compress: true });
  const pageW = pdf.internal.pageSize.getWidth();
  const pageH = pdf.internal.pageSize.getHeight();
  const k = (pageW * PAGE_PPI) / CANVAS_WIDTH;
  const style = scaleStyle(BASE_STYLE, k);
  const measure = createMeasure(style);

  const canvasW = Math.round(pageW * PAGE_PPI);
  const canvasH = Math.round(pageH * PAGE_PPI);
  const barWidth = (CANVAS_WIDTH - BASE_STYLE.marginX * 2) * k;
  const scale = computeScale(results, barWidth);
  const layouts = results.map(item => layoutBar(item, scale, measure, style));

  // 页眉几何
  const left = style.marginX;
  const right = canvasW - style.marginX;
  const titlePx = Math.round(12 * PAGE_PPI);
  const smallPx = Math.round(8.5 * PAGE_PPI);
  const titleY = PAGE_MARGIN_PT * PAGE_PPI + titlePx;
  const statsY = titleY + Math.round(10 * PAGE_PPI);
  const dividerY = statsY + Math.round(8 * PAGE_PPI);
  const contentTop = dividerY + Math.round(6 * PAGE_PPI);
  const contentBottom = canvasH - PAGE_MARGIN_PT * PAGE_PPI;

  // 第一遍: 按行贪心分页
  const pages: BarLayout[][] = [];
  let current: BarLayout[] = [];
  let y = contentTop;
  layouts.forEach(layout => {
    const h = rowHeight(layout, style);
    if (current.length > 0 && y + h > contentBottom) {
      pages.push(current);
      current = [];
      y = contentTop;
    }
    current.push(layout);
    y += h;
  });
  if (current.length > 0) pages.push(current);

  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  const dateText = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}`;
  const summaryLine = summaryText(results, summary);

  // 第二遍: 逐页渲染
  pages.forEach((pageLayouts, pageIndex) => {
    const canvas = document.createElement('canvas');
    canvas.width = canvasW;
    canvas.height = canvasH;
    const ctx = canvas.getContext('2d');
    if (!ctx) {
      throw new Error('canvas 2d context unavailable');
    }
    ctx.fillStyle = '#fff';
    ctx.fillRect(0, 0, canvasW, canvasH);

    // 页眉: 标题 + 页码 / 汇总 + 日期 / 分隔线
    ctx.fillStyle = '#111827';
    ctx.textAlign = 'left';
    ctx.font = fontCss(titlePx, true);
    ctx.fillText('切割示意图', left, titleY);
    ctx.font = fontCss(smallPx);
    ctx.fillStyle = '#6b7280';
    ctx.textAlign = 'right';
    ctx.fillText(`第 ${pageIndex + 1} / ${pages.length} 页`, right, titleY);
    ctx.textAlign = 'left';
    ctx.fillText(summaryLine, left, statsY);
    ctx.textAlign = 'right';
    ctx.fillText(dateText, right, statsY);
    ctx.strokeStyle = '#d1d5db';
    ctx.lineWidth = Math.max(1, Math.round(PAGE_PPI * 0.5));
    ctx.beginPath();
    ctx.moveTo(left, dividerY);
    ctx.lineTo(right, dividerY);
    ctx.stroke();

    let rowY = contentTop;
    pageLayouts.forEach(layout => {
      drawBarRow(ctx, layout, rowY, scale, barWidth, style);
      rowY += rowHeight(layout, style);
    });

    if (pageIndex > 0) {
      pdf.addPage();
    }
    pdf.addImage(canvas.toDataURL('image/png'), 'PNG', 0, 0, pageW, pageH);
  });

  return pdf;
}

/** 导出切割示意图 PDF (横向 A4, 单根料不跨页) */
export async function exportBarCutPDF(results: BarRow[], summary?: Api.Cut.BarSummary | null): Promise<void> {
  buildBarCutPDF(results, summary).save(`切割图-${formatFileTimestamp()}.pdf`);
}
