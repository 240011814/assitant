"""二维切割精确求解 (OR-Tools CP-SAT NoOverlap2D)。

模型:
  - 零件按 items 顺序×demand 展开, 每件恰好放入一块候选板 (ExactlyOne assignment);
  - 旋转是每件一个布尔 (一件只放一处), 用变长区间表达 (尺寸在 {w×h, h×w} 间切换);
  - 每块板 AddNoOverlap2D 保证互不重叠;
  - 目标: min Σ 板成本×启用, 旧料成本 0、新板成本=面积 → 旧料优先, 新板总面积最小;
  - Go 侧启发式解作为完整 hint 传入, 限时内返回解不劣于启发式;
  - 同类型零件可互换: 按展开序施加"板下标单调"约束破除对称。

规模防御: 展开件数/板数/件×板乘积超限返回 failed, 由 Go 回退启发式。
放不进任何候选板的零件按 oversized 剔除并报告 (Go 视为合法未排入, 与启发式口径一致)。
材料类型: 零件 spec 非空时只能排入同 spec 的板 (空 spec = 通用, 任意板)。
"""

from __future__ import annotations

import os
import time

from ortools.sat.python import cp_model

from cut_api.models_2d import Placement, SolveRequest, SolveResponse

EPS = 1e-6
MAX_PIECES = 400   # 展开件数上限 (Go 侧已先挡, 这里兜底)
MAX_BOARDS = 40    # 候选板数上限
MAX_PAIRS = 9600   # 件×板 乘积上限 (模型规模防御)


def _scale_for(dims: list[float]) -> int:
    """离散化倍率: 使所有尺寸×scale 为整数 (常规精度 0.01)"""
    def integral(v: float) -> bool:
        return abs(v - round(v)) < 1e-6

    for s in (1, 10, 100, 1000):
        if all(integral(d * s) for d in dims):
            return s
    return 100


def _spec_ok(item_spec: str, board_spec: str) -> bool:
    """材料类型兼容: 通用零件 (spec 空) 可上任意板; 指定 spec 的零件只能上同名板"""
    return not item_spec or item_spec == board_spec


def solve(req: SolveRequest) -> SolveResponse:
    t0 = time.time()
    elapsed_ms = lambda: int((time.time() - t0) * 1000)  # noqa: E731

    def failed() -> SolveResponse:
        return SolveResponse(pieces=[], unplaced=[], status="failed", elapsed_ms=elapsed_ms())

    scale = _scale_for([d for it in req.items for d in (it.width, it.height)]
                       + [d for b in req.boards for d in (b.width, b.height)])

    # 展开零件 (展开顺序 = items 顺序×demand, 与 Go 侧一致, hint 按类型内出现序对齐)
    pieces: list[tuple[int, int, int]] = []  # (type_idx, w, h) 已离散化
    for ti, it in enumerate(req.items):
        for _ in range(it.demand):
            pieces.append((ti, round(it.width * scale), round(it.height * scale)))
    total = len(pieces)
    boards = [(round(b.width * scale), round(b.height * scale), b) for b in req.boards]

    if total == 0:
        return SolveResponse(pieces=[], unplaced=[], status="feasible", elapsed_ms=elapsed_ms())
    if not boards or total > MAX_PIECES or len(boards) > MAX_BOARDS or total * len(boards) > MAX_PAIRS:
        return failed()

    def fits(w: int, h: int, bw: int, bh: int) -> bool:
        return (w <= bw + EPS and h <= bh + EPS) or (h <= bw + EPS and w <= bh + EPS)

    # 放不进任何兼容候选板的零件: oversized, 不进模型 (Go 侧与启发式同口径报告未排入)
    placeable: list[int] = []
    oversized: dict[int, int] = {}
    for pi, (ti, w, h) in enumerate(pieces):
        if any(_spec_ok(req.items[ti].spec, b.spec) and fits(w, h, bw, bh) for bw, bh, b in boards):
            placeable.append(pi)
        else:
            oversized[ti] = oversized.get(ti, 0) + 1

    if not placeable:
        unplaced = [{"item_index": ti, "count": c, "reason": "oversized"} for ti, c in oversized.items()]
        return SolveResponse(pieces=[], unplaced=unplaced, status="feasible", elapsed_ms=elapsed_ms())

    m = cp_model.CpModel()

    # 每件: 旋转布尔 + 变长尺寸 (正方形件无旋转)
    rot: dict[int, object] = {}
    sx: dict[int, object] = {}
    sy: dict[int, object] = {}
    for pi in placeable:
        _, w, h = pieces[pi]
        if w == h:
            sx[pi], sy[pi] = w, h
            continue
        r = m.NewBoolVar(f"rot{pi}")
        rot[pi] = r
        lo, hi = min(w, h), max(w, h)
        sx[pi] = m.NewIntVar(lo, hi, f"sx{pi}")
        sy[pi] = m.NewIntVar(lo, hi, f"sy{pi}")
        m.Add(sx[pi] == w).OnlyEnforceIf(r.Not())
        m.Add(sy[pi] == h).OnlyEnforceIf(r.Not())
        m.Add(sx[pi] == h).OnlyEnforceIf(r)
        m.Add(sy[pi] == w).OnlyEnforceIf(r)

    # 每件恰好放入一块板; (件,板) 可行对上建 presence 与变长区间
    a: dict[tuple[int, int], object] = {}
    px: dict[tuple[int, int], object] = {}
    py: dict[tuple[int, int], object] = {}
    by_piece: dict[int, list[int]] = {}
    intervals_x: list[list] = [[] for _ in boards]
    intervals_y: list[list] = [[] for _ in boards]
    for pi in placeable:
        ti, w, h = pieces[pi]
        item_spec = req.items[ti].spec
        lits = []
        for bi, (bw, bh, _b) in enumerate(boards):
            if not _spec_ok(item_spec, _b.spec):
                continue  # 材料类型不兼容的板不建 (件,板) 变量
            normal_ok = w <= bw + EPS and h <= bh + EPS
            rot_ok = h <= bw + EPS and w <= bh + EPS
            if not normal_ok and not rot_ok:
                continue
            presence = m.NewBoolVar(f"a{pi}_{bi}")
            a[(pi, bi)] = presence
            lits.append(presence)
            by_piece.setdefault(pi, []).append(bi)
            if pi in rot:
                # 该板只容得下单一定向时锁死旋转
                if normal_ok and not rot_ok:
                    m.Add(rot[pi].Not()).OnlyEnforceIf(presence)
                elif rot_ok and not normal_ok:
                    m.Add(rot[pi]).OnlyEnforceIf(presence)

            start_x = m.NewIntVar(0, bw, f"px{pi}_{bi}")
            start_y = m.NewIntVar(0, bh, f"py{pi}_{bi}")
            px[(pi, bi)] = start_x
            py[(pi, bi)] = start_y
            intervals_x[bi].append(
                m.NewOptionalIntervalVar(start_x, sx[pi], m.NewIntVar(0, bw, f"ex{pi}_{bi}"), presence, f"ivx{pi}_{bi}"))
            intervals_y[bi].append(
                m.NewOptionalIntervalVar(start_y, sy[pi], m.NewIntVar(0, bh, f"ey{pi}_{bi}"), presence, f"ivy{pi}_{bi}"))
        m.AddExactlyOne(lits)

    for bi in range(len(boards)):
        if intervals_x[bi]:
            m.AddNoOverlap2D(intervals_x[bi], intervals_y[bi])

    # 板启用变量 + 目标: 旧料成本 0, 新板成本=面积
    used: list[object] = []
    obj_terms = []
    for bi, (bw, bh, b) in enumerate(boards):
        u = m.NewBoolVar(f"used{bi}")
        used.append(u)
        for pi in placeable:
            if (pi, bi) in a:
                m.AddImplication(a[(pi, bi)], u)
        if not b.is_scrap:
            obj_terms.append(bw * bh * u)
    if obj_terms:
        m.Minimize(sum(obj_terms))

    # 对称性破除: 同类型零件可互换 → 展开序的板下标单调不减
    # (方向与装箱顺序一致: 先展开的零件落在不大于后展开者的板下标, Go hint 同序可直接受用)
    type_pieces: dict[int, list[int]] = {}
    for pi in placeable:
        type_pieces.setdefault(pieces[pi][0], []).append(pi)
    for plist in type_pieces.values():
        for p1, p2 in zip(plist, plist[1:]):
            for b1 in by_piece[p1]:
                higher = [a[(p2, b2)] for b2 in by_piece[p2] if b2 >= b1]
                if higher:
                    m.Add(sum(higher) >= a[(p1, b1)])

    # Go 启发式解作为 hint (entries 按 items 顺序展开 → 类型内出现序逐件对齐)
    occ = {ti: 0 for ti in type_pieces}
    for hp in req.initial_solution:
        plist = type_pieces.get(hp.item)
        if not plist:
            continue
        k = occ.get(hp.item, 0)
        occ[hp.item] = k + 1
        if k >= len(plist):
            continue
        pi = plist[k]
        if (pi, hp.board) not in a:
            continue
        _, w, h = pieces[pi]
        bw, bh = boards[hp.board][0], boards[hp.board][1]
        ow, oh = (h, w) if hp.rotated else (w, h)
        x = min(max(0, round(hp.x * scale)), max(0, bw - ow))
        y = min(max(0, round(hp.y * scale)), max(0, bh - oh))
        if x + ow > bw + EPS or y + oh > bh + EPS:
            continue  # hint 与板不符, 丢弃该件
        m.AddHint(a[(pi, hp.board)], 1)
        for b2 in by_piece[pi]:
            if b2 != hp.board:
                m.AddHint(a[(pi, b2)], 0)
        if pi in rot:
            m.AddHint(rot[pi], 1 if hp.rotated else 0)
        m.AddHint(px[(pi, hp.board)], x)
        m.AddHint(py[(pi, hp.board)], y)
    hinted_boards = {hp.board for hp in req.initial_solution}
    for bi, u in enumerate(used):
        m.AddHint(u, 1 if bi in hinted_boards else 0)

    solver = cp_model.CpSolver()
    solver.parameters.max_time_in_seconds = max(0.05, (req.time_limit_ms - elapsed_ms()) / 1000)
    solver.parameters.num_search_workers = min(8, os.cpu_count() or 4)
    status = solver.Solve(m)
    if status not in (cp_model.OPTIMAL, cp_model.FEASIBLE):
        return failed()

    out_pieces: list[Placement] = []
    unfit: dict[int, int] = {}
    for pi in placeable:
        ti, _, _ = pieces[pi]
        assigned = None
        for bi in by_piece[pi]:
            if solver.Value(a[(pi, bi)]):
                assigned = bi
                break
        if assigned is None:  # ExactlyOne 下不应发生, 防御
            unfit[ti] = unfit.get(ti, 0) + 1
            continue
        rotated = bool(solver.Value(rot[pi])) if pi in rot else False
        out_pieces.append(Placement(
            item=ti, board=assigned,
            x=round(solver.Value(px[(pi, assigned)]) / scale, 2),
            y=round(solver.Value(py[(pi, assigned)]) / scale, 2),
            rotated=rotated))

    if unfit:
        # 有零件未安置: 解不完备, 交由 Go 回退启发式
        return failed()

    unplaced = [{"item_index": ti, "count": c, "reason": "oversized"} for ti, c in oversized.items()]
    status_str = "optimal" if status == cp_model.OPTIMAL else "feasible"
    return SolveResponse(pieces=out_pieces, unplaced=unplaced, status=status_str, elapsed_ms=elapsed_ms())
