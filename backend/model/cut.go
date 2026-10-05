package model

import (
	"encoding/json"
	"time"
)

// CutRecord 切割记录实体
type CutRecord struct {
	ID         string    `gorm:"primaryKey;size:100" json:"id"`
	Type       string    `gorm:"size:20;index" json:"type"`
	Request    string    `gorm:"type:text" json:"request"`
	Response   string    `gorm:"type:text" json:"response"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UserID     uint      `gorm:"not null;index" json:"userId"`
	Code       string    `gorm:"size:100" json:"code"`
	Name       string    `gorm:"size:100;not null" json:"name"`
}

func (CutRecord) TableName() string {
	return "cut_record"
}

// BarMaterial 一维材料 (旧料/新材料规格统一模型, 带类型名)
type BarMaterial struct {
	Label  string  `json:"label"` // 类型名 (如: 长料 / 45#方管余料), 可空
	Length float64 `json:"length" binding:"required,min=1"`
}

// BarMaterialList 兼容两种 JSON 形态: 旧请求的 [6000,4000] (纯长度) 与新的 [{label,length}]
type BarMaterialList []BarMaterial

func (l *BarMaterialList) UnmarshalJSON(data []byte) error {
	var numbers []int
	if err := json.Unmarshal(data, &numbers); err == nil {
		for _, n := range numbers {
			*l = append(*l, BarMaterial{Length: float64(n)})
		}
		return nil
	}
	var items []BarMaterial
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	*l = items
	return nil
}

// BarItem 一维切割尺寸 (零件), 可指定归属材料规格
type BarItem struct {
	Length float64 `json:"length"`
	Spec   string  `json:"spec"` // 归属材料规格名 (newMaterials 的 label); 空=通用 (算法自由选料)
}

// BarItemList 兼容两种 JSON 形态: 旧请求的 [2000,1500] (纯长度, 无规格) 与新的 [{length, spec}]
type BarItemList []BarItem

func (l *BarItemList) UnmarshalJSON(data []byte) error {
	var numbers []int
	if err := json.Unmarshal(data, &numbers); err == nil {
		for _, n := range numbers {
			*l = append(*l, BarItem{Length: float64(n)})
		}
		return nil
	}
	var items []BarItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	*l = items
	return nil
}

// IntItems 由纯长度列表构造 ItemList (测试/内部便捷用)
func IntItems(lengths ...float64) BarItemList {
	out := make(BarItemList, 0, len(lengths))
	for _, n := range lengths {
		out = append(out, BarItem{Length: n})
	}
	return out
}

// 一维求解模式
const (
	BarModeFast    = "fast"    // 内置 DP+贪心 (默认)
	BarModePrecise = "precise" // OR-Tools sidecar 精确求解 (需系统配置 cut_solver_url; 失败自动回退 fast)
)

// BarRequest 一维切割请求
type BarRequest struct {
	Items     BarItemList    `json:"items" binding:"required,min=1"`
	Materials BarMaterialList `json:"materials"` // 旧料 (带类型, 兼容纯长度数组)
	// NewMaterials 新材料类型列表; 为空时回退 NewMaterialLength 单一规格 (兼容旧请求)
	NewMaterials      []BarMaterial `json:"newMaterials"`
	NewMaterialLength float64       `json:"newMaterialLength"`
	Loss              float64       `json:"loss"`
	UtilizationWeight float64       `json:"utilizationWeight"`
	// Mode 求解模式: fast (默认) / precise (OR-Tools 列生成; 未配置求解地址或求解失败时自动回退 fast)
	Mode string `json:"mode"`
	// UseInventory 自动导入余料库存: 把当前用户的一维余料库存并入旧料参与计算,
	// 被消费的库存条目 id 通过响应 consumedScrapIds 返回 (由前端确认后调 consume 扣减)
	UseInventory bool `json:"useInventory"`
}

// BarResult 一维切割结果
type BarResult struct {
	Index        int       `json:"index"`
	TotalLength  float64   `json:"totalLength"`
	Cuts         []float64 `json:"cuts"`
	Used         float64   `json:"used"`
	Remaining    float64   `json:"remaining"`
	MaterialType string    `json:"materialType"` // 材料类型 (新料规格名/旧料类型名, 空则前端显示"新材料")
}

// BarSummary 一维切割汇总 (利用率/余料统计, 供结果页展示与余料入库)
type BarSummary struct {
	MaterialCount       int     `json:"materialCount"`       // 使用材料根数
	TotalMaterialLength float64 `json:"totalMaterialLength"` // 材料总长
	TotalCutLength      float64 `json:"totalCutLength"`      // 零件总长
	TotalRemaining      float64 `json:"totalRemaining"`      // 余料总长
	Utilization         float64 `json:"utilization"`         // 整体利用率 (百分比)
	ScrapCount          int     `json:"scrapCount"`          // 可入库余料根数 (Remaining > 0)
}

// BarCutResponse 一维切割响应
type BarCutResponse struct {
	Results []BarResult `json:"results"`
	Summary BarSummary  `json:"summary"`
	// ConsumedScrapIds 本计算消费掉的库存余料条目 id (useInventory=true 时返回, 前端确认后调 consume 扣减)
	ConsumedScrapIds []uint `json:"consumedScrapIds"`
}

// ConsumeScrapsRequest 扣减库存余料 (切割确认后调用)
type ConsumeScrapsRequest struct {
	IDs []uint `json:"ids" binding:"required,min=1"`
}

// PlaneSummary 平面切割汇总
type PlaneSummary struct {
	BinCount      int     `json:"binCount"`      // 用料总数 (旧料 + 新板材)
	UsedArea      float64 `json:"usedArea"`      // 已排入零件总面积
	TotalArea     float64 `json:"totalArea"`     // 用料总面积
	Utilization   float64 `json:"utilization"`   // 整体利用率 (百分比)
	UnplacedCount int     `json:"unplacedCount"` // 未排入件数
}

// UnplacedItem 未排入零件 (带数量归并): 件尺寸超过所有可用材料或材料耗尽时产生
type UnplacedItem struct {
	Label    string  `json:"label"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Quantity int     `json:"quantity"`
	Reason   string  `json:"reason"` // oversized=超过所有可用材料尺寸 / exhausted=材料用尽
}

// PlaneCutResponse 平面切割响应
type PlaneCutResponse struct {
	Results  []BinResult    `json:"results"`
	Unplaced []UnplacedItem `json:"unplaced"`
	Summary  PlaneSummary   `json:"summary"`
}

// CutScrap 余料库存实体 (切割后剩余材料登记, 下次计算可复用)
type CutScrap struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint      `gorm:"not null;index" json:"userId"`
	ScrapType   int       `json:"scrapType"`   // 1=一维余料(长度) 2=二维余料(板材)
	Label       string    `json:"label"`       // 余料名称/来源材料规格 (多材料切割时标注, 如: 长料余料)
	LengthValue float64   `json:"lengthValue"` // 一维: 长度
	WidthValue  float64   `json:"widthValue"`  // 二维: 宽
	HeightValue float64   `json:"heightValue"` // 二维: 高
	Quantity    int       `json:"quantity"`    // 数量
	Note        string    `json:"note"`        // 备注
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (CutScrap) TableName() string {
	return "cut_scrap_inventory"
}

// AddCutScrapRequest 登记余料
type AddCutScrapRequest struct {
	ScrapType   int     `json:"scrapType" binding:"required,oneof=1 2"`
	Label       string  `json:"label"` // 来源材料规格名 (多材料切割时标注)
	LengthValue float64 `json:"lengthValue"`
	WidthValue  float64 `json:"widthValue"`
	HeightValue float64 `json:"heightValue"`
	Quantity    int     `json:"quantity" binding:"required,min=1"`
	Note        string  `json:"note"`
}

// Item 切割项目
type Item struct {
	Label    string  `json:"label"`
	Width    float64 `json:"width" binding:"required,min=1"`
	Height   float64 `json:"height" binding:"required,min=1"`
	Quantity int     `json:"quantity"`
}

// Piece 切割块
type Piece struct {
	Label   string  `json:"label"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	W       float64 `json:"w"`
	H       float64 `json:"h"`
	Rotated bool    `json:"rotated"`
}

// BinRequest 平面切割请求
type BinRequest struct {
	Items     []Item  `json:"items" binding:"required,min=1"`
	Materials []Item  `json:"materials"`
	Height    float64 `json:"height" binding:"required,min=1"`
	Width     float64 `json:"width" binding:"required,min=1"`
	Strategy  string  `json:"strategy" binding:"required"`
}

// BinResult 平面切割结果
type BinResult struct {
	BinID          int     `json:"binId"`
	MaterialType   string  `json:"materialType"`
	MaterialWidth  float64 `json:"materialWidth"`
	MaterialHeight float64 `json:"materialHeight"`
	Pieces         []Piece `json:"pieces"`
	Utilization    float64 `json:"utilization"`
}

// RecordRequest 保存切割记录请求
type RecordRequest struct {
	Type     string `json:"type" binding:"required"`
	Request  string `json:"request" binding:"required"`
	Response string `json:"response" binding:"required"`
	Name     string `json:"name" binding:"required"`
}

// CutRecordSearchParams 切割记录搜索参数
type CutRecordSearchParams struct {
	Page      int    `form:"current" binding:"required,min=1"`
	PageSize  int    `form:"size" binding:"required,min=1,max=100"`
	Name      string `form:"name"`
	Type      string `form:"type"`
	StartTime *int64 `form:"startTime"`
	EndTime   *int64 `form:"endTime"`
}

// CutRecordListResponse 切割记录列表响应
type CutRecordListResponse struct {
	Total   int64       `json:"total"`
	Records []CutRecord `json:"records"`
}
