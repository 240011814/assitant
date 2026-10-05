"""一维切割求解器的请求/响应模型 (pydantic)。"""

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field


class Item(BaseModel):
    length: float = Field(gt=0)
    demand: int = Field(ge=0)


class Material(BaseModel):
    label: str = ""
    length: float = Field(gt=0)


class Scrap(BaseModel):
    length: float = Field(gt=0)


class SolveRequest(BaseModel):
    kerf: float = Field(default=0, ge=0)
    items: list[Item]
    materials: list[Material] = []
    scraps: list[Scrap] = []
    time_limit_ms: int = Field(default=3000, ge=200, le=60000)
    utilization_weight: float = Field(default=5, ge=1, le=8)


class Bar(BaseModel):
    source: str  # "material" | "scrap"
    material_index: Optional[int] = None
    scrap_length: Optional[float] = None
    pattern: list[int]  # 每种零件的切割数量 (按 items 下标)
    count: int = 1      # 同模式同来源的根数


class SolveResponse(BaseModel):
    bars: list[Bar]
    unplaced: list[dict]  # [{item_index, count}]
    iterations: int = 0
    elapsed_ms: int = 0
