"""/cut1d/solve 路由 (供 Go 后端「精确模式」调用, 失败由 Go 侧回退内置快速算法)。"""

from __future__ import annotations

from fastapi import APIRouter

from cut1d_api.models import SolveRequest, SolveResponse
from cut1d_api.solver import solve

router = APIRouter()


@router.post("/cut1d/solve", response_model=SolveResponse)
def solve_route(req: SolveRequest) -> SolveResponse:
    return solve(req)
