"""cut2d-api: 二维切割精确求解 (OR-Tools CP-SAT NoOverlap2D), 挂载在 baostock sidecar。

与 baostock_api 平级的子包: models 定义请求/响应, solver 为纯算法实现,
router 挂载 POST /cut2d/solve; main.py `from cut2d_api import router` 引入。
"""

from cut2d_api.models import Board, Item, Placement, SolveRequest, SolveResponse
from cut2d_api.router import router
from cut2d_api.solver import solve

__all__ = ["Board", "Item", "Placement", "SolveRequest", "SolveResponse", "router", "solve"]
