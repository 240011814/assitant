"""切割优化精确求解路由 (供 Go 后端「精确模式」调用; 失败由 Go 侧回退内置算法)。

  POST /cut1d/solve  一维列生成 (solver_1d)
  POST /cut2d/solve  二维 CP-SAT NoOverlap2D (solver_2d)
"""

from __future__ import annotations

from fastapi import APIRouter

from cut_api.models_1d import SolveRequest as SolveRequest1D
from cut_api.models_1d import SolveResponse as SolveResponse1D
from cut_api.models_2d import SolveRequest as SolveRequest2D
from cut_api.models_2d import SolveResponse as SolveResponse2D
from cut_api.solver_1d import solve as solve_1d
from cut_api.solver_2d import solve as solve_2d

router = APIRouter()


@router.post("/cut1d/solve", response_model=SolveResponse1D)
def solve_1d_route(req: SolveRequest1D) -> SolveResponse1D:
    return solve_1d(req)


@router.post("/cut2d/solve", response_model=SolveResponse2D)
def solve_2d_route(req: SolveRequest2D) -> SolveResponse2D:
    return solve_2d(req)
