import { jsPDF } from 'jspdf';
import { canvasToBlob, downloadBlob, formatFileTimestamp, printBlobUrl } from '@/utils/cut-export';

/**
 * 一维切割示意图绘制与导出
 *
 * 每根料一条水平条(按 totalLength 比例), 内部按 cuts 顺序分段,
 * 相邻段之间画锯缝线; 每段内标尺寸数字, 小段放段下方引线;
 * 条首标 "#index 总长", 余料段灰色。
 */

const COLOR_POOL = Array.from({ length: 50 }, (_, i) => `hsl(${(i * 30) % 360}, 70%, 50%)`);

const CANVAS_WIDTH = 1600;
const MARGIN_X = 24;
const MARGIN_Y = 20;
const TITLE_HEIGHT = 34;
const LABEL_HEIGHT = 22;
const BAR_HEIGHT = 40;
const ROW_GAP = 20;
const KERF_WIDTH = 2;
/** 段内直标的最小像素宽, 低于该值放段下方引线 */
const MIN_INLINE_LABEL_WIDTH = 44;
/** 引线标注的最小像素宽, 低于该值不标(过窄) */
const MIN_LEADER_LABEL_WIDTH = 12;
const LEADER_LINE_GAP = 14;
const LEADER_STEP = 16;

interface BarLabel {
  x: number;
  text: string;
  slot: number;
}

interface BarLayout {
  bar: Api.Cut.BarResult;
  labels: BarLabel[];
  leaderRows: number;
}

function fmt(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

function leaderAreaHeight(layout: BarLayout): number {
  return layout.leaderRows > 0 ? LEADER_LINE_GAP + layout.leaderRows * LEADER_STEP : 0;
}

function rowHeight(layout: BarLayout): number {
  return LABEL_HEIGHT + BAR_HEIGHT + leaderAreaHeight(layout) + ROW_GAP;
}

function layoutBar(item: Api.Cut.BarResult, scale: number, measure: (text: string) => number): BarLayout {
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
    if (w < MIN_INLINE_LABEL_WIDTH && w >= MIN_LEADER_LABEL_WIDTH) {
      pushLabel(x + w / 2, fmt(cut));
    }
    x += w;
  });
  if (item.remaining > 0) {
    const w = item.remaining * scale;
    if (w < MIN_INLINE_LABEL_WIDTH && w >= MIN_LEADER_LABEL_WIDTH) {
      pushLabel(x + w / 2, `余${fmt(item.remaining)}`);
    }
  }

  return { bar: item, labels, leaderRows: slotEnds.length };
}

function drawBarRow(ctx: CanvasRenderingContext2D, layout: BarLayout, top: number, scale: number, barWidth: number) {
  const item = layout.bar;

  // 条首标签 + 右侧统计
  ctx.font = 'bold 14px sans-serif';
  ctx.fillStyle = '#1f2937';
  ctx.textAlign = 'left';
  ctx.fillText(`#${item.index} 总长 ${fmt(item.totalLength)}cm`, MARGIN_X, top + 15);
  ctx.font = '12px sans-serif';
  ctx.fillStyle = '#6b7280';
  ctx.textAlign = 'right';
  ctx.fillText(`已用 ${fmt(item.used)}cm | 余料 ${fmt(item.remaining)}cm`, MARGIN_X + barWidth, top + 15);
  ctx.textAlign = 'center';

  const barTop = top + LABEL_HEIGHT;
  let x = MARGIN_X;

  item.cuts.forEach((cut, idx) => {
    const w = Math.max(cut * scale, 1);
    ctx.fillStyle = COLOR_POOL[idx % COLOR_POOL.length];
    ctx.fillRect(x, barTop, w, BAR_HEIGHT);
    ctx.strokeStyle = '#fff';
    ctx.lineWidth = 1;
    ctx.strokeRect(x + 0.5, barTop + 0.5, Math.max(w - 1, 0), BAR_HEIGHT - 1);
    // 相邻段之间画锯缝线
    if (idx > 0) {
      ctx.fillStyle = '#374151';
      ctx.fillRect(x - KERF_WIDTH / 2, barTop, KERF_WIDTH, BAR_HEIGHT);
    }
    if (w >= MIN_INLINE_LABEL_WIDTH) {
      ctx.fillStyle = '#fff';
      ctx.font = '12px sans-serif';
      ctx.fillText(fmt(cut), x + w / 2, barTop + BAR_HEIGHT / 2 + 4);
    }
    x += w;
  });

  // 余料段(灰色)
  if (item.remaining > 0) {
    const w = Math.max(item.remaining * scale, 1);
    ctx.fillStyle = '#c0c4cc';
    ctx.fillRect(x, barTop, w, BAR_HEIGHT);
    ctx.strokeStyle = '#9ca3af';
    ctx.lineWidth = 1;
    ctx.strokeRect(x + 0.5, barTop + 0.5, w - 1, BAR_HEIGHT - 1);
    if (w >= MIN_INLINE_LABEL_WIDTH) {
      ctx.fillStyle = '#333';
      ctx.font = '12px sans-serif';
      ctx.fillText(`余 ${fmt(item.remaining)}`, x + w / 2, barTop + BAR_HEIGHT / 2 + 4);
    }
  }

  // 段下方引线标注(小段)
  ctx.font = '10px sans-serif';
  layout.labels.forEach(lb => {
    const labelY = barTop + BAR_HEIGHT + LEADER_LINE_GAP + lb.slot * LEADER_STEP;
    ctx.strokeStyle = '#9ca3af';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(lb.x, barTop + BAR_HEIGHT);
    ctx.lineTo(lb.x, labelY - 3);
    ctx.stroke();
    ctx.fillStyle = '#374151';
    ctx.textAlign = 'center';
    ctx.fillText(lb.text, lb.x, labelY);
  });
}

/** 把一维切割结果画成切割示意图 canvas */
export function drawBarCutChart(results: Api.Cut.BarResult[], summary?: Api.Cut.BarSummary | null): HTMLCanvasElement {
  if (results.length === 0) {
    throw new Error('no bar cut results to draw');
  }

  const measureCanvas = document.createElement('canvas');
  const mctx = measureCanvas.getContext('2d');
  const measure = (text: string) => {
    if (!mctx) return text.length * 7;
    mctx.font = '10px sans-serif';
    return mctx.measureText(text).width;
  };

  const barWidth = CANVAS_WIDTH - MARGIN_X * 2;
  const maxTotal = Math.max(...results.map(r => r.totalLength));
  const scale = maxTotal > 0 ? barWidth / maxTotal : 1;
  const layouts = results.map(item => layoutBar(item, scale, measure));
  const contentHeight = layouts.reduce((sum, l) => sum + rowHeight(l), 0);

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
  ctx.font = 'bold 16px sans-serif';
  ctx.fillStyle = '#111827';
  ctx.textAlign = 'left';
  const title = summary
    ? `切割示意图  共 ${summary.materialCount} 根 | 材料总长 ${fmt(summary.totalMaterialLength)}cm | ` +
      `零件总长 ${fmt(summary.totalCutLength)}cm | 利用率 ${summary.utilization}% | 余料 ${fmt(summary.totalRemaining)}cm (${summary.scrapCount}根)`
    : `切割示意图  共 ${results.length} 根`;
  ctx.fillText(title, MARGIN_X, MARGIN_Y + 22);

  let y = MARGIN_Y + TITLE_HEIGHT;
  layouts.forEach(layout => {
    drawBarRow(ctx, layout, y, scale, barWidth);
    y += rowHeight(layout);
  });
  return canvas;
}

/** 导出切割示意图 PNG */
export async function exportBarCutPNG(results: Api.Cut.BarResult[], summary?: Api.Cut.BarSummary | null): Promise<void> {
  const canvas = drawBarCutChart(results, summary);
  const blob = await canvasToBlob(canvas);
  downloadBlob(blob, `切割图-${formatFileTimestamp()}.png`);
}

/** 生成切割示意图 PDF (横向 A4, 多根料自动分页) */
export function buildBarCutPDF(results: Api.Cut.BarResult[], summary?: Api.Cut.BarSummary | null): jsPDF {
  const canvas = drawBarCutChart(results, summary);
  const pdf = new jsPDF({ orientation: 'landscape', unit: 'pt', format: 'a4', compress: true });
  const pageW = pdf.internal.pageSize.getWidth();
  const pageH = pdf.internal.pageSize.getHeight();
  const margin = 16;
  const contentW = pageW - margin * 2;
  const contentH = pageH - margin * 2;
  // canvas 像素与 PDF pt 的换算比例
  const pxPerPt = canvas.width / contentW;
  const pagePxH = Math.max(Math.floor(contentH * pxPerPt), 1);

  let firstPage = true;
  for (let y = 0; y < canvas.height; y += pagePxH) {
    const sliceH = Math.min(pagePxH, canvas.height - y);
    const slice = document.createElement('canvas');
    slice.width = canvas.width;
    slice.height = sliceH;
    const sctx = slice.getContext('2d');
    if (!sctx) break;
    sctx.fillStyle = '#fff';
    sctx.fillRect(0, 0, slice.width, slice.height);
    sctx.drawImage(canvas, 0, -y);

    if (!firstPage) {
      pdf.addPage();
    }
    pdf.addImage(slice.toDataURL('image/png'), 'PNG', margin, margin, contentW, sliceH / pxPerPt);
    firstPage = false;
  }
  return pdf;
}

/** 导出切割示意图 PDF (横向 A4, 多根料自动分页) */
export async function exportBarCutPDF(results: Api.Cut.BarResult[], summary?: Api.Cut.BarSummary | null): Promise<void> {
  buildBarCutPDF(results, summary).save(`切割图-${formatFileTimestamp()}.pdf`);
}

/** 直接打印切割示意图 (PDF autoPrint, 新窗口打开后自动弹出打印对话框) */
export async function printBarCut(results: Api.Cut.BarResult[], summary?: Api.Cut.BarSummary | null): Promise<boolean> {
  const pdf = buildBarCutPDF(results, summary);
  pdf.autoPrint();
  const blobUrl = pdf.output('bloburl');
  return printBlobUrl(String(blobUrl));
}
