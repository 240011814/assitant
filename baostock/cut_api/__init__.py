"""cut_api: 切割优化精确求解 (OR-Tools), 挂载在 baostock sidecar。

一维列生成 (solver_1d, POST /cut1d/solve) 与二维 CP-SAT NoOverlap2D (solver_2d,
POST /cut2d/solve) 两个端点共用本包, main.py `from cutopt import router` 引入;
失败/超时由 Go 后端回退内置算法。两类求解的模型独立在 models_1d / models_2d。
"""

from cut_api.models_1d import Bar, Material, Scrap
from cut_api.models_1d import Item as Item1D
from cut_api.models_1d import SolveRequest as SolveRequest1D
from cut_api.models_1d import SolveResponse as SolveResponse1D
from cut_api.models_2d import Board, Placement
from cut_api.models_2d import Item as Item2D
from cut_api.models_2d import SolveRequest as SolveRequest2D
from cut_api.models_2d import SolveResponse as SolveResponse2D
from cut_api.router import router
from cut_api.solver_1d import solve as solve_1d
from cut_api.solver_2d import solve as solve_2d

__all__ = [
    "Bar", "Material", "Scrap", "Item1D", "SolveRequest1D", "SolveResponse1D",
    "Board", "Placement", "Item2D", "SolveRequest2D", "SolveResponse2D",
    "router", "solve_1d", "solve_2d",
]
