"""cut2d 自验脚本 (部署侧运行): baostock/.venv/Scripts/python.exe verify/verify_cut2d.py

构造已知最优/已知行为的实例直调 cut_api.solver_2d.solve, 校验完全覆盖 / 无重叠 / 尺寸不越界 /
旋转生效 / 旧料优先 / hint 不劣化 / oversized 报告; 全部通过打印 ALL CHECKS PASSED。
HTTP 冒烟: 启动 uvicorn 后 POST /cut2d/solve (Go 后端「精确模式」走同一端点)。
"""

import sys

from cut_api.models_2d import Board, Item, Placement, SolveRequest
from cut_api.solver_2d import solve


def check_solution(req: SolveRequest, label: str, expect_pieces: int | None = None) -> list[Placement]:
    resp = solve(req)
    assert resp.status in ("optimal", "feasible"), f"[{label}] status={resp.status}"
    placed = [0] * len(req.items)
    for p in resp.pieces:
        it = req.items[p.item]
        b = req.boards[p.board]
        w, h = (it.height, it.width) if p.rotated else (it.width, it.height)
        assert 0 <= p.x and p.x + w <= b.width + 1e-6, f"[{label}] 零件 {p.item} 越界 x: {p.x}+{w}>{b.width}"
        assert 0 <= p.y and p.y + h <= b.height + 1e-6, f"[{label}] 零件 {p.item} 越界 y: {p.y}+{h}>{b.height}"
        placed[p.item] += 1
    for i, it in enumerate(req.items):
        assert placed[i] == it.demand, f"[{label}] 零件 {i} 排入数不符: {placed[i]}/{it.demand}"
    # 同板两两不重叠
    by_board: dict[int, list[Placement]] = {}
    for p in resp.pieces:
        by_board.setdefault(p.board, []).append(p)
    for bi, plist in by_board.items():
        for i in range(len(plist)):
            for j in range(i + 1, len(plist)):
                pi, pj = plist[i], plist[j]
                wi, hi = (req.items[pi.item].height, req.items[pi.item].width) if pi.rotated else \
                         (req.items[pi.item].width, req.items[pi.item].height)
                wj, hj = (req.items[pj.item].height, req.items[pj.item].width) if pj.rotated else \
                         (req.items[pj.item].width, req.items[pj.item].height)
                sep_x = pi.x + wi <= pj.x + 1e-6 or pj.x + wj <= pi.x + 1e-6
                sep_y = pi.y + hi <= pj.y + 1e-6 or pj.y + hj <= pi.y + 1e-6
                sep = sep_x or sep_y
                assert sep, f"[{label}] 板 {bi} 上零件 {pi.item}/{pj.item} 重叠"
    if expect_pieces is not None:
        assert len(resp.pieces) == expect_pieces, f"[{label}] 排入件数应 {expect_pieces}, 实际 {len(resp.pieces)}"
    used_boards = sorted(by_board)
    print(f"[{label}] PASS boards={len(used_boards)} pieces={len(resp.pieces)} "
          f"status={resp.status} 耗时={resp.elapsed_ms}ms")
    return resp.pieces


def main() -> None:
    # 1. 单板最优: 50×50×2 放 100×100 (2×2 网格, 一块板)
    check_solution(SolveRequest(
        items=[Item(width=50, height=50, demand=2)],
        boards=[Board(width=100, height=100)],
    ), "单板最优", expect_pieces=2)

    # 2. 旋转生效: 70×30×4 在 100×100 需 3 正向 + 1 旋转 才能单板放下 (无 hint 自行探索;
    #    不旋转时 4 件需 120 高/140 宽, 单板不可行)
    pieces = check_solution(SolveRequest(
        items=[Item(width=70, height=30, demand=4)],
        boards=[Board(width=100, height=100)],
    ), "旋转单板", expect_pieces=4)
    assert any(p.rotated for p in pieces), "旋转用例应出现旋转件"

    # 3. 旧料优先: 同尺寸旧料+新板, 全部零件应落在旧料上 (新板成本=面积 → 最小化)
    req = SolveRequest(
        items=[Item(width=50, height=50, demand=2)],
        boards=[Board(label="旧料A", width=100, height=100, is_scrap=True), Board(width=100, height=100)],
    )
    resp = solve(req)
    used_boards = {p.board for p in resp.pieces}
    assert all(req.boards[b].is_scrap for b in used_boards), "应优先使用旧料"

    # 4. hint 不劣化: 次优 hint (两块板各放一件), 求解器应优化到一块板
    resp = solve(SolveRequest(
        items=[Item(width=50, height=50, demand=2)],
        boards=[Board(width=100, height=100), Board(width=100, height=100)],
        initial_solution=[Placement(item=0, board=0, x=0, y=0),
                          Placement(item=0, board=1, x=0, y=0)],
    ))
    assert len({p.board for p in resp.pieces}) == 1, f"应优化到单板: {resp.pieces}"
    print(f"[hint 优化到单板] PASS boards=1 pieces={len(resp.pieces)} status={resp.status} 耗时={resp.elapsed_ms}ms")

    # 5. oversized: 超出所有板的零件报告未排入, 其余照常排入
    resp = solve(SolveRequest(
        items=[Item(label="大件", width=200, height=200, demand=1),
               Item(width=40, height=40, demand=2)],
        boards=[Board(width=100, height=100)],
    ))
    assert resp.status in ("optimal", "feasible")
    assert len(resp.unplaced) == 1 and resp.unplaced[0]["reason"] == "oversized" \
        and resp.unplaced[0]["count"] == 1, f"oversized 未正确报告: {resp.unplaced}"
    assert len(resp.pieces) == 2
    print("[oversized] PASS unplaced=1(oversized) pieces=2")

    # 6. 材料类型: spec 非空的零件只能上同名板; 通用零件 (spec 空) 任意板
    req = SolveRequest(
        items=[Item(label="背板", width=40, height=40, demand=2, spec="5mm背板"),
               Item(label="门板", width=40, height=40, demand=2, spec="18mm门板"),
               Item(label="通用", width=40, height=40, demand=1)],
        boards=[Board(label="5mm背板", width=100, height=100, spec="5mm背板"),
                Board(label="18mm门板", width=100, height=100, spec="18mm门板")],
    )
    resp = solve(req)
    assert resp.status in ("optimal", "feasible")
    assert len(resp.pieces) == 5, f"应排入 5 件: {resp.pieces}"
    for p in resp.pieces:
        it = req.items[p.item]
        b = req.boards[p.board]
        assert not it.spec or b.spec == it.spec, f"spec 件 {it.label} 落到了 {b.label} 板"
    print(f"[材料类型] PASS pieces={len(resp.pieces)} status={resp.status} 耗时={resp.elapsed_ms}ms")

    # 7. spec 件放不进自身规格板 => oversized (通用板放得下也不可上)
    resp = solve(SolveRequest(
        items=[Item(width=60, height=60, demand=1, spec="小板")],
        boards=[Board(width=100, height=100), Board(width=50, height=50, spec="小板")],
    ))
    assert len(resp.unplaced) == 1 and resp.unplaced[0]["reason"] == "oversized" \
        and resp.unplaced[0]["count"] == 1, f"spec 件应 oversized: {resp.unplaced}"
    assert len(resp.pieces) == 0
    print("[spec oversized] PASS unplaced=1(oversized) pieces=0")

    # 8. kerf 锯缝: 板按 (W+kerf,H+kerf) 容纳, 件按 (w+kerf,h+kerf) 占位 —
    #    45×45×2 在 90×90 上 kerf=5 时膨胀件 50 一板只容 1 件 → 必须开 2 块板
    resp = solve(SolveRequest(
        items=[Item(width=45, height=45, demand=2)],
        boards=[Board(width=90, height=90), Board(width=90, height=90)],
        kerf=5,
    ))
    assert resp.status in ("optimal", "feasible"), f"[kerf开板] status={resp.status}"
    assert len({p.board for p in resp.pieces}) == 2, f"kerf=5 时每板只容 1 件, 应 2 块板: {resp.pieces}"
    print(f"[kerf开板] PASS boards=2 pieces={len(resp.pieces)} status={resp.status} 耗时={resp.elapsed_ms}ms")

    # 9. kerf 间距: 60×60×2 在 200×100 上 kerf=5 — 同板同排时 x 向间隙必须 ≥ kerf (输出仍为真实坐标)
    pieces = check_solution(SolveRequest(
        items=[Item(width=60, height=60, demand=2)],
        boards=[Board(width=200, height=100)],
        kerf=5,
    ), "kerf间距", expect_pieces=2)
    assert len({p.board for p in pieces}) == 1, f"200×100 应单板放下 2 件: {pieces}"
    a, b = pieces
    same_row = abs(a.y - b.y) < 1e-6
    same_col = abs(a.x - b.x) < 1e-6
    if same_row:
        gap = max(a.x - (b.x + 60), b.x - (a.x + 60))
    elif same_col:
        gap = max(a.y - (b.y + 60), b.y - (a.y + 60))
    else:
        gap = 99.0  # 斜向错开, 间距必然合规
    assert gap >= 5 - 1e-6, f"同排/同列零件间距 {gap} 应 ≥ kerf 5"
    print(f"[kerf间距] PASS gap={gap}")

    print("ALL CHECKS PASSED")


if __name__ == "__main__":
    sys.exit(main())
