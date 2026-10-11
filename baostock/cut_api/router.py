"""切割优化精确求解路由 (供 Go 后端「精确模式」调用; 失败由 Go 侧直接报错给前端)。

  POST /cut1d/solve  一维列生成 (solver_1d)
  POST /cut2d/solve  二维 CP-SAT NoOverlap2D (solver_2d)

并发治理: OR-Tools 求解是 CPU 密集任务 (CP-SAT 单请求可开多线程), 用信号量限制
同时求解数, 超限直接 503 让调用方稍后重试, 避免多请求并发时超订 CPU 拖垮整个 sidecar
(该进程还承载行情查询路由)。
"""

from __future__ import annotations

import threading

from fastapi import APIRouter, HTTPException

from cut_api.models_1d import SolveRequest as SolveRequest1D
from cut_api.models_1d import SolveResponse as SolveResponse1D
from cut_api.models_2d import SolveRequest as SolveRequest2D
from cut_api.models_2d import SolveResponse as SolveResponse2D
from cut_api.solver_1d import solve as solve_1d
from cut_api.solver_2d import solve as solve_2d

router = APIRouter()

# 同时求解上限 (每个求解内部还会开 2~8 线程, 2 个并发已接近小机型 CPU 饱和)
MAX_CONCURRENT_SOLVES = 2
_solve_slots = threading.BoundedSemaphore(MAX_CONCURRENT_SOLVES)


def _acquire_slot() -> None:
    if not _solve_slots.acquire(blocking=False):
        raise HTTPException(status_code=503, detail="求解服务繁忙, 请稍后重试")


@router.post("/cut1d/solve", response_model=SolveResponse1D)
def solve_1d_route(req: SolveRequest1D) -> SolveResponse1D:
    _acquire_slot()
    try:
        return solve_1d(req)
    finally:
        _solve_slots.release()


@router.post("/cut2d/solve", response_model=SolveResponse2D)
def solve_2d_route(req: SolveRequest2D) -> SolveResponse2D:
    _acquire_slot()
    try:
        return solve_2d(req)
    finally:
        _solve_slots.release()
