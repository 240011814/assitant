/**
 * 窗户分格模型与切割件生成 (纯函数, 供产品单编辑器/预览/导入共用)。
 *
 * 模型: 窗类产品 = 外框(宽 W×高 H, 框料宽 c) + C 列 × R 行分格, 每格固定格或开启扇。
 * 切割件口径 (与旧国标窗户公式逐项等价):
 * - 外框横梃 W×2 / 外框竖梃 H×2 (45° 拼角不扣尺); 推拉窗称 外框上下横/外框边封
 * - 竖向中梃 (H−2c)×(C−1) (推拉无竖向中梃, 两扇导轨重叠); 横向中梃 (W−2c)×(R−1)
 * - 每个开启扇: 扇横梃 = 格净宽×2, 扇竖梃 = 格净高×2; 推拉扇称 扇上下横/扇竖梃(光企/勾企)
 * 几何自洽: 列净宽和 + (推拉?0:(C−1)c) + 2c = W; 行净高和 + (R−1)c + 2c = H
 *
 * 部件族: 每个切割件归属一种型材 (外框/中梃/扇), 同族件从同一种库存型材下料;
 * 产品级只设材料厚度 (壁厚), 导入切割清单时行标签 = 部件族 + 厚度。
 */

/** 格子类型: 固定格 / 开启扇 */
export const CELL_FIXED = 1;
export const CELL_SASH = 2;

export type WindowCell = 1 | 2;

/** 切割件部件族 (对应库存型材分类) */
export type PieceFamily = '外框' | '中梃' | '扇';

/** 窗户分格规格 (与后端 CutWindowGrid 同构) */
export interface WindowGridSpec {
  /** 列净宽 (cm, 从左到右) */
  cols: number[];
  /** 行净高 (cm, 从上到下) */
  rows: number[];
  /** cells[行][列] 的格子类型 (1=固定格 2=开启扇) */
  cells: number[][];
  /** 推拉窗 (无竖向中梃) */
  sliding: boolean;
}

/** 一件产品 (与后端 CutProductItem 同构) */
export interface ProductItem {
  /** 产品模板 key (WINDOW_TEMPLATES 项的 key) */
  type: string;
  /** 总宽 cm */
  width: number;
  /** 总高 cm */
  height: number;
  /** 框料宽 cm */
  frameWidth: number;
  /** 材料厚度 (型材壁厚 mm, 如 "1.4") */
  thickness: string;
  /** 数量 */
  count: number;
  grid: WindowGridSpec;
}

/** 切割件草稿 (单件) */
export interface PieceDraft {
  name: string;
  /** 部件族 (外框/中梃/扇), 决定用哪种型材下料 */
  family: PieceFamily;
  length: number;
  quantity: number;
}

/** 内置窗型模板 (参考北方铝合金门窗; 尺寸为 cm 默认值) */
export interface WindowTemplateDef {
  key: string;
  label: string;
  cols: number;
  rows: number;
  sliding: boolean;
  /** 预置格子类型矩阵 rows×cols */
  cells: WindowCell[][];
  defaults: { width: number; height: number; frameWidth: number };
}

function fillCells(rows: number, cols: number, cell: WindowCell): WindowCell[][] {
  return Array.from({ length: rows }, () => Array.from({ length: cols }, () => cell));
}

export const WINDOW_TEMPLATES: WindowTemplateDef[] = [
  { key: 'fixed', label: '固定窗', cols: 1, rows: 1, sliding: false, cells: fillCells(1, 1, CELL_FIXED), defaults: { width: 150, height: 200, frameWidth: 5 } },
  { key: 'casement', label: '平开窗 (单扇)', cols: 1, rows: 1, sliding: false, cells: fillCells(1, 1, CELL_SASH), defaults: { width: 150, height: 200, frameWidth: 5 } },
  { key: 'casement2', label: '平开窗 (双扇)', cols: 2, rows: 1, sliding: false, cells: fillCells(1, 2, CELL_SASH), defaults: { width: 180, height: 200, frameWidth: 5 } },
  { key: 'topHung', label: '上悬窗', cols: 1, rows: 1, sliding: false, cells: fillCells(1, 1, CELL_SASH), defaults: { width: 60, height: 120, frameWidth: 5 } },
  { key: 'sliding2', label: '推拉窗 (两扇)', cols: 2, rows: 1, sliding: true, cells: fillCells(1, 2, CELL_SASH), defaults: { width: 150, height: 150, frameWidth: 5 } },
  { key: 'sliding3', label: '推拉窗 (三扇)', cols: 3, rows: 1, sliding: true, cells: fillCells(1, 3, CELL_SASH), defaults: { width: 240, height: 150, frameWidth: 5 } },
  { key: 'sliding4', label: '推拉窗 (四扇)', cols: 4, rows: 1, sliding: true, cells: fillCells(1, 4, CELL_SASH), defaults: { width: 300, height: 150, frameWidth: 5 } },
  // 带亮子: 上行固定亮, 下行开启
  { key: 'casementTransom', label: '平开窗带亮子 (上固下开)', cols: 1, rows: 2, sliding: false, cells: [[CELL_FIXED], [CELL_SASH]], defaults: { width: 150, height: 240, frameWidth: 5 } },
  { key: 'slidingTransom', label: '推拉窗带亮子 (上固下推拉)', cols: 2, rows: 2, sliding: true, cells: [[CELL_FIXED, CELL_FIXED], [CELL_SASH, CELL_SASH]], defaults: { width: 240, height: 240, frameWidth: 5 } }
];

export function findWindowTemplate(key: string): WindowTemplateDef | undefined {
  return WINDOW_TEMPLATES.find(t => t.key === key);
}

/** 解析产品单 spec JSON (坏数据按空单处理, 不抛错; 兼容旧版窗户单的 windows key) */
export function parseProductSpec(record: Pick<Api.Cut.CutProduct, 'spec'>): Api.Cut.ProductSpec {
  try {
    const spec = JSON.parse(record.spec) as Partial<Api.Cut.ProductSpec> & { windows?: Api.Cut.ProductItem[] };
    if (spec && Array.isArray(spec.items)) return { items: spec.items };
    if (spec && Array.isArray(spec.windows)) return { items: spec.windows };
  } catch {
    // 坏数据按空单处理
  }
  return { items: [] };
}

export function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

/** 分格可用净宽合计 (总宽 − 2×框料宽 − 竖向中梃) */
export function availableColWidth(width: number, frameWidth: number, cols: number, sliding: boolean): number {
  return width - 2 * frameWidth - (sliding ? 0 : (cols - 1) * frameWidth);
}

/** 分格可用净高合计 (总高 − 2×框料宽 − 横向中梃) */
export function availableRowHeight(height: number, frameWidth: number, rows: number): number {
  return height - 2 * frameWidth - (rows - 1) * frameWidth;
}

/** 几何自洽校验 (误差 ±0.01cm 内视为相符); 返回错误信息, null = 通过 */
export function checkGridGeometry(item: Pick<ProductItem, 'width' | 'height' | 'frameWidth' | 'grid'>): string | null {
  const { width, height, frameWidth: c, grid } = item;
  const colSum = grid.cols.reduce((s, v) => s + v, 0);
  const rowSum = grid.rows.reduce((s, v) => s + v, 0);
  const expectW = colSum + (grid.sliding ? 0 : (grid.cols.length - 1) * c) + 2 * c;
  const expectH = rowSum + (grid.rows.length - 1) * c + 2 * c;
  const eps = 0.011;
  if (width <= 0 || height <= 0) return '窗户宽高必须大于 0';
  if (grid.cols.some(v => v <= 0) || grid.rows.some(v => v <= 0)) return '分格净宽/净高必须大于 0';
  if (Math.abs(expectW - width) > eps) return '列净宽合计与总宽不符';
  if (Math.abs(expectH - height) > eps) return '行净高合计与总高不符';
  return null;
}

/** 由模板定义构造初始产品 (净宽高均分) */
export function productItemFromTemplate(def: WindowTemplateDef, thickness = ''): ProductItem {
  const { width, height, frameWidth } = def.defaults;
  const col = round2(availableColWidth(width, frameWidth, def.cols, def.sliding) / def.cols);
  const row = round2(availableRowHeight(height, frameWidth, def.rows) / def.rows);
  return {
    type: def.key,
    width,
    height,
    frameWidth,
    thickness,
    count: 1,
    grid: {
      cols: Array.from({ length: def.cols }, () => col),
      rows: Array.from({ length: def.rows }, () => row),
      cells: def.cells.map(r => [...r]),
      sliding: def.sliding
    }
  };
}

/** 生成单件切割件 (同名同长合并) */
export function buildPieces(item: Pick<ProductItem, 'width' | 'height' | 'frameWidth' | 'grid'>): PieceDraft[] {
  const { width: w, height: h, frameWidth: c, grid } = item;
  const pieces: PieceDraft[] = [];
  const push = (family: PieceFamily, name: string, length: number, quantity: number) => {
    if (length > 0 && quantity > 0) pieces.push({ family, name, length: round2(length), quantity });
  };

  if (grid.sliding) {
    push('外框', '外框上下横', w, 2);
    push('外框', '外框边封', h, 2);
  } else {
    push('外框', '外框横梃', w, 2);
    push('外框', '外框竖梃', h, 2);
  }
  if (!grid.sliding && grid.cols.length > 1) {
    push('中梃', '竖向中梃', h - 2 * c, grid.cols.length - 1);
  }
  if (grid.rows.length > 1) {
    push('中梃', '横向中梃', w - 2 * c, grid.rows.length - 1);
  }
  grid.cells.forEach((rowCells, r) => {
    rowCells.forEach((cell, ci) => {
      if (cell !== CELL_SASH) return;
      const colW = grid.cols[ci] ?? 0;
      const rowH = grid.rows[r] ?? 0;
      if (grid.sliding) {
        push('扇', '扇上下横', colW, 2);
        push('扇', '扇竖梃 (光企/勾企)', rowH, 2);
      } else {
        push('扇', '扇横梃', colW, 2);
        push('扇', '扇竖梃', rowH, 2);
      }
    });
  });
  return mergePieces(pieces);
}

/** 同名同长合并 (数量累加, 保持首次出现顺序) */
export function mergePieces(pieces: PieceDraft[]): PieceDraft[] {
  const map = new Map<string, PieceDraft>();
  for (const p of pieces) {
    const key = `${p.name}|${p.length}`;
    const exist = map.get(key);
    if (exist) {
      exist.quantity += p.quantity;
    } else {
      map.set(key, { ...p });
    }
  }
  return [...map.values()];
}

/** 导入切割行: 一批产品的切割件 × 数量, 按 (部件族+厚度, 长度) 合并;
 * 行标签即一维切割的材料类型 (如 "外框 1.4"), 与库存型材的分类口径一致 */
export function productCutRows(items: ProductItem[]): Array<{ label: string; length: number; quantity: number }> {
  const map = new Map<string, { label: string; length: number; quantity: number }>();
  for (const item of items) {
    if (item.width <= 0 || item.height <= 0 || item.count < 1) continue;
    const suffix = item.thickness.trim() ? ` ${item.thickness.trim()}` : '';
    for (const p of buildPieces(item)) {
      const label = `${p.family}${suffix}`;
      const quantity = p.quantity * item.count;
      const key = `${label}|${p.length}`;
      const exist = map.get(key);
      if (exist) {
        exist.quantity += quantity;
      } else {
        map.set(key, { label, length: p.length, quantity });
      }
    }
  }
  return [...map.values()];
}
