import { jsPDF } from 'jspdf';
import { canvasToBlob, downloadBlob, formatFileTimestamp, printBlobUrl } from '@/utils/cut-export';

/**
 * 二维切割示意图绘制与导出
 *
 * 每个排版结果(bin)画到离屏 canvas: 板材外框 + 每件矩形(标签+尺寸),
 * 标题行标注材料类型与利用率。PNG 导出全部板材拼接纵向长图,
 * PDF 逐板分页(竖向 A4)。
 */

const MARGIN = 16;
const TITLE_HEIGHT = 26;
const GAP = 24;
/** 板材绘制的最大边长(px) */
const MAX_SHEET_PX = 1400;
const GRID_STEP = 10;

function fmt(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

/** 由标签生成确定性颜色, 同一零件每次绘制颜色一致 */
function labelHue(label: string): number {
  let hash = 0;
  for (let i = 0; i < label.length; i += 1) {
    hash = (hash * 31 + label.charCodeAt(i)) % 360;
  }
  return hash;
}

/** 把单块板材排版结果画到离屏 canvas */
export function drawBinToCanvas(bin: Api.Cut.BinResult): HTMLCanvasElement {
  const scale = MAX_SHEET_PX / Math.max(bin.materialWidth, bin.materialHeight, 1);
  const sheetW = Math.max(bin.materialWidth * scale, 10);
  const sheetH = Math.max(bin.materialHeight * scale, 10);

  const canvas = document.createElement('canvas');
  canvas.width = Math.ceil(sheetW + MARGIN * 2);
  canvas.height = Math.ceil(TITLE_HEIGHT + sheetH + MARGIN * 2);
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    throw new Error('canvas 2d context unavailable');
  }

  // 背景与标题(材料类型 + 尺寸 + 利用率)
  ctx.fillStyle = '#fff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.font = 'bold 14px sans-serif';
  ctx.fillStyle = '#111827';
  ctx.textAlign = 'left';
  ctx.fillText(
    `${bin.materialType || '材料'}  ${fmt(bin.materialWidth)} × ${fmt(bin.materialHeight)} cm  |  ` +
      `利用率 ${bin.utilization}%  |  件数 ${bin.pieces.length}`,
    MARGIN,
    MARGIN + 16
  );

  // 板材底色: 旧料绿色 / 新料蓝色
  const isScrap = (bin.materialType || '').includes('旧料');
  ctx.fillStyle = isScrap ? '#e8f5e8' : '#e3f2fd';
  ctx.fillRect(MARGIN, TITLE_HEIGHT, sheetW, sheetH);

  // 网格线(10cm)
  ctx.strokeStyle = '#d1d5db';
  ctx.lineWidth = 1;
  for (let gx = GRID_STEP; gx < bin.materialWidth; gx += GRID_STEP) {
    const px = MARGIN + gx * scale;
    ctx.beginPath();
    ctx.moveTo(px, TITLE_HEIGHT);
    ctx.lineTo(px, TITLE_HEIGHT + sheetH);
    ctx.stroke();
  }
  for (let gy = GRID_STEP; gy < bin.materialHeight; gy += GRID_STEP) {
    const py = TITLE_HEIGHT + gy * scale;
    ctx.beginPath();
    ctx.moveTo(MARGIN, py);
    ctx.lineTo(MARGIN + sheetW, py);
    ctx.stroke();
  }

  // 板材外框
  ctx.strokeStyle = '#111827';
  ctx.lineWidth = 2;
  ctx.strokeRect(MARGIN, TITLE_HEIGHT, sheetW, sheetH);

  // 零件矩形 + 标签 + 尺寸
  bin.pieces.forEach(piece => {
    const x = MARGIN + piece.x * scale;
    const y = TITLE_HEIGHT + piece.y * scale;
    const w = Math.max(piece.w * scale, 1);
    const h = Math.max(piece.h * scale, 1);

    ctx.fillStyle = `hsl(${labelHue(piece.label)}, 70%, 80%)`;
    ctx.fillRect(x, y, w, h);
    ctx.strokeStyle = '#111827';
    ctx.lineWidth = 1;
    ctx.strokeRect(x + 0.5, y + 0.5, Math.max(w - 1, 0), Math.max(h - 1, 0));

    if (w >= 48 && h >= 42) {
      ctx.fillStyle = 'rgba(255,255,255,0.85)';
      ctx.fillRect(x + 2, y + 2, Math.min(w - 4, 110), 34);
      ctx.fillStyle = '#1f2937';
      ctx.font = '11px sans-serif';
      ctx.textAlign = 'left';
      ctx.fillText(piece.label, x + 5, y + 14);
      ctx.font = '10px sans-serif';
      ctx.fillText(`${fmt(piece.w)}×${fmt(piece.h)}cm${piece.rotated ? ' (转)' : ''}`, x + 5, y + 28);
    }
  });

  return canvas;
}

/** 导出全部板材拼接纵向长图 PNG */
export async function exportPlaneCutPNG(results: Api.Cut.BinResult[]): Promise<void> {
  if (results.length === 0) {
    throw new Error('no plane cut results to export');
  }
  const canvases = results.map(bin => drawBinToCanvas(bin));
  const width = Math.max(...canvases.map(c => c.width));
  const height = canvases.reduce((sum, c) => sum + c.height + GAP, -GAP);

  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = Math.ceil(height);
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    throw new Error('canvas 2d context unavailable');
  }
  ctx.fillStyle = '#fff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  let y = 0;
  canvases.forEach(c => {
    ctx.drawImage(c, Math.floor((width - c.width) / 2), y);
    y += c.height + GAP;
  });

  const blob = await canvasToBlob(canvas);
  downloadBlob(blob, `切割图-${formatFileTimestamp()}.png`);
}

/** 生成 PDF (竖向 A4, 每块板材一页) */
export function buildPlaneCutPDF(results: Api.Cut.BinResult[]): jsPDF {
  if (results.length === 0) {
    throw new Error('no plane cut results to export');
  }
  const pdf = new jsPDF({ orientation: 'portrait', unit: 'pt', format: 'a4', compress: true });
  const pageW = pdf.internal.pageSize.getWidth();
  const pageH = pdf.internal.pageSize.getHeight();
  const margin = 20;
  const contentW = pageW - margin * 2;
  const contentH = pageH - margin * 2;

  results.forEach((bin, index) => {
    const c = drawBinToCanvas(bin);
    const fit = Math.min(contentW / c.width, contentH / c.height, 1);
    const drawW = c.width * fit;
    const drawH = c.height * fit;
    if (index > 0) {
      pdf.addPage();
    }
    pdf.addImage(c.toDataURL('image/png'), 'PNG', margin + (contentW - drawW) / 2, margin + (contentH - drawH) / 2, drawW, drawH);
  });
  return pdf;
}

/** 导出 PDF (竖向 A4, 每块板材一页) */
export async function exportPlaneCutPDF(results: Api.Cut.BinResult[]): Promise<void> {
  buildPlaneCutPDF(results).save(`切割图-${formatFileTimestamp()}.pdf`);
}

/** 直接打印切割图 (PDF autoPrint, 新窗口打开后自动弹出打印对话框) */
export async function printPlaneCut(results: Api.Cut.BinResult[]): Promise<boolean> {
  const pdf = buildPlaneCutPDF(results);
  pdf.autoPrint();
  const blobUrl = pdf.output('bloburl');
  return printBlobUrl(String(blobUrl));
}
