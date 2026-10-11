"""临时基准: 复现 "二维几十个件 2 分钟算不出来" 的性能问题 (排查用, 不进 CI)。

对比三种场景在 CP-SAT 下的耗时/状态:
  A. 无 hint 裸模型 (模拟 hint 被丢弃的情况)
  B. 带可行 hint (模拟 warm start 正常生效)
  C. 带可行 hint + 更大板候选 (模拟真实 Go 侧候选板规模)

用法: PYTHONPATH=. ./.venv/Scripts/python.exe verify/bench_cut2d.py
"""

import time

from cut_api.models_2d import Board, Item, Placement, SolveRequest
from cut_api.solver_2d import solve


def grid_hint(demand: int, per_board: int, pw: float, ph: float, cols: int) -> list[Placement]:
    """手工构造简单可行解: 每板 per_board 件网格摆放 (真实 Go hint 的粗糙替代)"""
    out: list[Placement] = []
    for i in range(demand):
        k = i % per_board
        out.append(Placement(item=0, board=i // per_board, x=(k % cols) * pw, y=(k // cols) * ph, rotated=False))
    return out


def run(label: str, req: SolveRequest) -> None:
    t0 = time.time()
    resp = solve(req)
    print(f"[{label}] status={resp.status} placed={len(resp.pieces)} "
          f"boards={len({p.board for p in resp.pieces})} wall={time.time()-t0:.1f}s elapsed_ms={resp.elapsed_ms}")


def main() -> None:
    budget = 60_000

    # 场景 A: 60 个 50×50 件 / 100×100 板 ×20, 无 hint
    run("A1 无hint 60件/20板", SolveRequest(
        items=[Item(width=50, height=50, demand=60)],
        boards=[Board(width=100, height=100) for _ in range(20)],
        time_limit_ms=budget))

    # 场景 B: 同上 + 可行 hint (每板 4 件网格)
    run("B1 带hint 60件/20板", SolveRequest(
        items=[Item(width=50, height=50, demand=60)],
        boards=[Board(width=100, height=100) for _ in range(20)],
        initial_solution=grid_hint(60, 4, 50, 50, 2),
        time_limit_ms=budget))

    # 场景 C: 混合两种件 + 带可行 hint (更接近真实订单)
    hint = grid_hint(40, 4, 50, 50, 2) + [
        Placement(item=1, board=15 + i // 2, x=(i % 2) * 60, y=0.0, rotated=False) for i in range(20)
    ]
    run("C1 带hint 40+20混合/22板", SolveRequest(
        items=[Item(width=50, height=50, demand=40), Item(width=60, height=40, demand=20)],
        boards=[Board(width=100, height=100) for _ in range(22)],
        initial_solution=hint,
        time_limit_ms=budget))

    # 场景 D: 同 C 但候选板多一倍 (模拟 Go 备用新板补足后的规模)
    run("D1 带hint 40+20混合/40板", SolveRequest(
        items=[Item(width=50, height=50, demand=40), Item(width=60, height=40, demand=20)],
        boards=[Board(width=100, height=100) for _ in range(40)],
        initial_solution=hint + [Placement(item=1, board=30 + i, x=0.0, y=0.0, rotated=False) for i in range(10)],
        time_limit_ms=budget))


if __name__ == "__main__":
    main()
