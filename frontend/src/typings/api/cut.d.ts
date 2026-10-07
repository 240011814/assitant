declare namespace Api {
  namespace Cut {
    interface BarResult {
      index: number;
      totalLength: number;
      cuts: number[];
      used: number;
      remaining: number;
      /** 材料类型: 新料规格名/旧料类型名/库存余料名, 空=未命名(新材料) */
      materialType?: string;
    }

    /** 一维按材料类型分组统计 (口径同整体汇总) */
    interface BarMaterialTypeSummary {
      /** 材料类型名 (空=未命名的新材料规格) */
      materialType: string;
      /** 该类型用料根数 */
      count: number;
      /** 该类型材料总长 */
      totalMaterialLength: number;
      /** 该类型零件总长 */
      totalCutLength: number;
      /** 该类型余料总长 */
      totalRemaining: number;
      /** 该类型可入库余料根数 (remaining>0) */
      scrapCount: number;
      /** 该类型利用率 (已乘100的百分数) */
      utilization: number;
    }

    /** 一维切割汇总 */
    interface BarSummary {
      /** 使用材料根数 */
      materialCount: number;
      /** 材料总长 */
      totalMaterialLength: number;
      /** 零件总长 */
      totalCutLength: number;
      /** 余料总长 */
      totalRemaining: number;
      /** 整体利用率 (已乘100的百分数) */
      utilization: number;
      /** 可入库余料根数 (remaining>0) */
      scrapCount: number;
      /** 按材料类型分组统计 (旧记录无此字段) */
      byMaterialType?: BarMaterialTypeSummary[];
    }

    /** 窗户分格规格 (cols/rows 为格净尺寸 cm; cells[r][c]: 1=固定格 2=开启扇; sliding=推拉无竖向中梃) */
    interface WindowGridSpec {
      cols: number[];
      rows: number[];
      cells: number[][];
      sliding: boolean;
    }

    /** 产品单内的一件产品 (type 为前端产品模板 key; 窗类产品带分格 grid) */
    interface ProductItem {
      type: string;
      width: number;
      height: number;
      /** 框料宽 cm (由框料截面宽度/系列换算而来) */
      frameWidth: number;
      /** 框料截面宽度 mm (即型材系列: 55/60/65/70/75/80) */
      series: string;
      /** 拼装搭接参数 mm (省略的字段走默认) */
      fit?: ProductFit;
      /** 是否计算玻璃压条 (固定格与开启扇均出压条件) */
      bead?: boolean;
      count: number;
      grid: WindowGridSpec;
    }

    /** 拼装搭接参数 (mm): 扇料下料 = 开口 ∓ 缝隙/搭接 */
    interface ProductFit {
      /** 活动缝隙: 平开每边缩尺 / 推拉边封侧缝 (默认 5) */
      gap?: number;
      /** 推拉扇上下轨道搭入, 每边 (默认 10) */
      reach?: number;
      /** 推拉相邻扇光企/勾企互搭量 (默认 10) */
      overlap?: number;
      /** 玻璃压条每根扣尺 (mm, 默认 10) */
      beadDeduct?: number;
    }

    /** 产品单内容 (一单可含多种类型的多件产品) */
    interface ProductSpec {
      items: ProductItem[];
    }

    /** 待切割产品单 (spec 为 JSON 字符串) */
    interface CutProduct {
      id: number;
      userId: number;
      name: string;
      spec: string;
      createdAt: string;
      updatedAt: string;
    }

    /** 新增/更新产品单 (id 缺省为新增) */
    interface SaveCutProductRequest {
      id?: number;
      name?: string;
      spec: ProductSpec;
    }

    /** 一维切割响应 */
    interface BarCutResponse {
      results: BarResult[];
      summary: BarSummary;
    }

    /** 二维切割未排入零件 */
    interface UnplacedItem {
      label: string;
      width: number;
      height: number;
      quantity: number;
      /** oversized=超过材料尺寸 / exhausted=材料已用完 */
      reason: 'oversized' | 'exhausted';
    }

    /** 二维切割汇总 */
    interface PlaneSummary {
      /** 用料块数 (旧料 + 新板材) */
      binCount: number;
      /** 已排入零件总面积 */
      usedArea: number;
      /** 用料总面积 */
      totalArea: number;
      /** 整体利用率 (已乘100的百分数) */
      utilization: number;
      /** 未排入件数 */
      unplacedCount: number;
    }

    /** 二维切割响应 */
    interface PlaneCutResponse {
      results: BinResult[];
      unplaced: UnplacedItem[];
      summary: PlaneSummary;
    }

    /** 余料库存 (scrapType: 1=一维余料(长度) 2=二维余料(板材)) */
    interface CutScrap {
      id: number;
      userId: number;
      scrapType: 1 | 2;
      /** 材料类型/来源材料规格 (与名称分开维护) */
      materialType?: string;
      /** 余料名称 (用户自定) */
      label?: string;
      /** 一维: 长度 */
      lengthValue: number;
      /** 二维: 宽 */
      widthValue: number;
      /** 二维: 高 */
      heightValue: number;
      quantity: number;
      note: string;
      createdAt: string;
      updatedAt: string;
    }

    /** 余料库存列表查询参数 (分页可省略, 省略时返回全部) */
    interface CutScrapSearchParams {
      /** 0=全部 1=一维 2=二维 */
      scrapType?: 0 | 1 | 2;
      current?: number;
      size?: number;
      /** 名称模糊 */
      name?: string;
      /** 长度范围 (cm, 含边界), 仅匹配一维余料 */
      lengthMin?: number;
      lengthMax?: number;
    }

    /** 余料库存列表响应 (分页) */
    interface CutScrapListResponse {
      total: number;
      records: CutScrap[];
    }

    /** 余料入库请求 (批量) */
    interface AddCutScrapRequest {
      scrapType: 1 | 2;
      /** 材料类型/来源材料规格 (与名称分开) */
      materialType?: string;
      /** 余料名称 (用户自定) */
      label?: string;
      lengthValue?: number;
      widthValue?: number;
      heightValue?: number;
      quantity: number;
      note?: string;
      /** 历史记录余料入库时传记录 ID: 入库成功同时把该记录标记为已入库 */
      recordId?: string;
    }

    interface BinResult {
      binId: number;
      materialType: string; // 新增：材料类型
      materialWidth: number; // 新增：材料宽度
      materialHeight: number; // 新增：材料高度
      pieces: Piece[];
      utilization: number;
    }

    interface CutRecord {
      id: string;

      type: string;

      request: string;

      response: string;

      createTime: string;

      code: string;

      name: string;

      /** 该记录的余料是否已从历史记录入库 (入库后不再展示入库入口) */
      scrapImported: boolean;
    }

    interface CutRecordSearchParams extends Api.Common.CommonSearchParams {
      name?: string | null;
      type?: string | null;
      startTime?: number | null;
      endTime?: number | null;
    }

    interface Piece {
      label: string;
      x: number;
      y: number;
      w: number;
      h: number;
      rotated: boolean;
    }

    interface BinRequest {
      items: Item[];
      materials: Item[];
      height: number;
      width: number;
      strategy: string;
    }

    interface RecordRequest {
      type: string;
      request: string;
      response: string;
      name: string;
      /** 保存时扣减的库存余料 (从库存带入且被本次切割消耗的旧料) */
      deductScraps?: { id: number; count: number }[];
    }

    /** 修改库存余料 */
    interface UpdateCutScrapRequest {
      label: string;
      /** 材料类型/来源材料规格 (与名称分开维护) */
      materialType: string;
      quantity: number;
      note: string;
    }

    /** 批量删除库存余料 */
    interface BatchDeleteCutScrapRequest {
      ids: number[];
    }

    interface Item {
      label: string;
      width: number;
      height: number;
      quantity?: number;
    }

    interface BarItem {
      label?: string;
      length: number;
      quantity: number;
    }

    /** 新材料规格(多规格) */
    interface NewMaterialSpec {
      label?: string;
      length: number;
    }

    interface BarRequest {
      /** 零件: 纯长度数组 或 带材料规格名的对象数组 [{ length, spec }] */
      items: Array<number | { length: number; spec?: string }>;
      /** 旧料: 纯长度数组 或 带类型名对象数组(页面内部用对象形态) */
      materials: Array<number | { label?: string; length: number }>;
      newMaterialLength: number;
      /** 多材料规格(可选): 不传时按 newMaterialLength 单规格计算 */
      newMaterials?: NewMaterialSpec[];
      loss: number;
      utilizationWeight: number;
      /** 求解模式: fast (默认, DP+贪心) / precise (OR-Tools 列生成, 走服务端 BAOSTOCK_API_URL 求解服务) */
      mode?: 'fast' | 'precise';
      /** 材料保护: 开启后余料长度不允许落在 [protectMin, protectMax] 区间内 (cm, 含边界) */
      protectEnabled?: boolean;
      protectMin?: number;
      protectMax?: number;
    }
  }
}
