"""二维切割求解器的请求/响应模型 (pydantic)。"""

from __future__ import annotations

from pydantic import BaseModel, Field


class Item(BaseModel):
    label: str = ""
    width: float = Field(gt=0)
    height: float = Field(gt=0)
    demand: int = Field(ge=1)


class Board(BaseModel):
    label: str = ""
    width: float = Field(gt=0)
    height: float = Field(gt=0)
    is_scrap: bool = False  # 旧料: 成本 0 (优先消耗); 新板: 成本=面积


class Placement(BaseModel):
    item: int = Field(ge=0)   # items 下标
    board: int = Field(ge=0)  # boards 下标
    x: float = Field(ge=0)
    y: float = Field(ge=0)
    rotated: bool = False


class SolveRequest(BaseModel):
    items: list[Item]
    boards: list[Board]
    # Go 侧启发式解 (与展开件一一对应, 按 items 顺序展开) 作为完整 warm start:
    # 限时内 CP-SAT 至少返回不劣于启发式的解
    initial_solution: list[Placement] = []
    time_limit_ms: int = Field(default=120000, ge=200, le=180000)


class SolveResponse(BaseModel):
    pieces: list[Placement]  # 已排入零件 (坐标为原单位, x/y 为左上角)
    unplaced: list[dict]     # [{item_index, count, reason: "oversized"|"unfit"}]
    status: str              # optimal | feasible | failed
    elapsed_ms: int = 0
