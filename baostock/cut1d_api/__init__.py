"""cut1d-api: 一维切割精确求解 (OR-Tools 列生成), 挂载在 baostock sidecar。

与 baostock_api 平级的子包: models 定义请求/响应, solver 为纯算法实现,
router 挂载 POST /cut1d/solve; main.py `from cut1d_api import router` 引入。
"""

from cut1d_api.models import Bar, Item, Material, Scrap, SolveRequest, SolveResponse
from cut1d_api.router import router
from cut1d_api.solver import solve

__all__ = ["Bar", "Item", "Material", "Scrap", "SolveRequest", "SolveResponse", "router", "solve"]
