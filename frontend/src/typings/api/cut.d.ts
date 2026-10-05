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
    }

    /** 一维切割响应 */
    interface BarCutResponse {
      results: BarResult[];
      summary: BarSummary;
      /** useInventory=true 时, 本次计算消费掉的库存条目 id */
      consumedScrapIds?: number[];
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
      /** 余料名称/来源材料规格 */
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

    /** 余料入库请求 (批量) */
    interface AddCutScrapRequest {
      scrapType: 1 | 2;
      /** 余料名称/来源材料规格 */
      label?: string;
      lengthValue?: number;
      widthValue?: number;
      heightValue?: number;
      quantity: number;
      note?: string;
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
      /** 求解模式: fast (默认, DP+贪心) / precise (OR-Tools 列生成, 需服务端配置求解地址) */
      mode?: 'fast' | 'precise';
      /** 自动导入当前用户的一维余料库存参与计算 */
      useInventory?: boolean;
    }
  }
}
