"""/cut2d/solve 路由 (供 Go 后端二维「精确模式」调用, 失败由 Go 侧回退 MaxRects 启发式)。"""

from __future__ import annotations

from fastapi import APIRouter

from cut2d_api.models import SolveRequest, SolveResponse
from cut2d_api.solver import solve

router = APIRouter()


@router.post("/cut2d/solve", response_model=SolveResponse)
def solve_route(req: SolveRequest) -> SolveResponse:
    return solve(req)
