"""cutopt 自验脚本 (部署侧运行): baostock/.venv/Scripts/python.exe verify_cut1d.py

构造已知最优解的实例直调 cut_api.solver_1d.solve, 校验需求覆盖 / kerf 语义 / 旧料数量约束 /
最少根数; 全部通过打印 ALL CHECKS PASSED。HTTP 冒烟: 启动 uvicorn 后
POST /cut1d/solve (Go 后端「精确模式」走同一端点)。
"""

import sys

from cut_api.models_1d import Item, Material, Scrap, SolveRequest
from cut_api.solver_1d import solve


def check_invariants(req: SolveRequest, label: str, expect_new_bars: int | None = None) -> None:
    resp = solve(req)
    covered = [0] * len(req.items)
    scrap_used: dict[float, int] = {}
    mat_len_total = 0.0
    cut_total = 0.0
    bar_count = 0
    for bar in resp.bars:
        assert bar.count >= 1, f"[{label}] count 应 ≥1"
        cuts = sum(bar.pattern)
        assert cuts > 0, f"[{label}] 空模式"
        if bar.source == "material":
            assert bar.material_index is not None and bar.material_index < len(req.materials)
            length = req.materials[bar.material_index].length
            mat_len_total += length * bar.count
        else:
            assert bar.scrap_length is not None
            length = bar.scrap_length
            scrap_used[length] = scrap_used.get(length, 0) + bar.count
        used = sum(req.items[i].length * bar.pattern[i] for i in range(len(req.items)))
        used += req.kerf * max(0, cuts - 1)
        assert used <= length + 1e-6, f"[{label}] 超容量: used={used} L={length}"
        for i in range(len(req.items)):
            covered[i] += bar.pattern[i] * bar.count
        cut_total += used * bar.count
        bar_count += bar.count
    for i, it in enumerate(req.items):
        assert covered[i] >= it.demand, f"[{label}] 零件 {i} 未覆盖: {covered[i]}/{it.demand}"
    avail: dict[float, int] = {}
    for sc in req.scraps:
        avail[sc.length] = avail.get(sc.length, 0) + 1
    for length, used_count in scrap_used.items():
        assert used_count <= avail.get(length, 0), f"[{label}] 旧料 {length} 超用: {used_count}/{avail.get(length, 0)}"
    if expect_new_bars is not None:
        assert bar_count == expect_new_bars, f"[{label}] 根数应为 {expect_new_bars}, 实际 {bar_count}"
    util = cut_total / mat_len_total * 100 if mat_len_total else 0
    print(f"[{label}] PASS bars={bar_count} 迭代={resp.iterations} 耗时={resp.elapsed_ms}ms 利用率={util:.1f}%")


def main() -> int:
    # 1. 最少根数: {65,60,41,39} 各1, L=100 → 3 根 (65 单独, 60+39, 41 单独)
    check_invariants(SolveRequest(
        kerf=0, items=[Item(length=65, demand=1), Item(length=60, demand=1),
                       Item(length=41, demand=1), Item(length=39, demand=1)],
        materials=[Material(label="L100", length=100)]), "最少根数", expect_new_bars=3)

    # 2. kerf 语义: L=10 kerf=2 len=4×5 → 每根最多 2 件 (4+4+2=10), 共 3 根
    check_invariants(SolveRequest(
        kerf=2, items=[Item(length=4, demand=5)],
        materials=[Material(length=10)]), "kerf语义", expect_new_bars=3)

    # 3. 旧料消化 + 逐根上限: 两根 25 旧料最多用两根 (曾被超卖成 6 根的回归用例)
    check_invariants(SolveRequest(
        kerf=1, items=[Item(length=10, demand=3), Item(length=7, demand=2)],
        materials=[Material(label="L60", length=60)],
        scraps=[Scrap(length=25), Scrap(length=25)]), "旧料消化")

    # 4. 中等规模: 12 种类型 × 50 件
    check_invariants(SolveRequest(
        kerf=3, items=[Item(length=1700 - i * 137, demand=50) for i in range(12)],
        materials=[Material(label="L6000", length=6000)],
        time_limit_ms=5000), "中等规模")

    # 5. 材料保护: 余料不允许落在 [40, 50] — 6×185 的 [185×3] (余料 44.6) 被禁, 应改走其他合规模式
    req = SolveRequest(kerf=0.2, items=[Item(length=185, demand=6)],
                       materials=[Material(label="L600", length=600)],
                       protect_enabled=True, protect_min=40, protect_max=50)
    resp = solve(req)
    covered = 0
    for bar in resp.bars:
        used = sum(req.items[i].length * bar.pattern[i] for i in range(len(req.items)))
        used += req.kerf * max(0, sum(bar.pattern) - 1)
        rem = req.materials[bar.material_index].length - used
        assert not (40 - 1e-6 <= rem <= 50 + 1e-6), f"[保护] 余料 {rem} 落在 [40,50]: pattern={bar.pattern}"
        covered += sum(bar.pattern) * bar.count
    assert covered >= 6, f"[保护] 零件未覆盖: {covered}/6"
    print(f"[保护] PASS bars={sum(b.count for b in resp.bars)} 迭代={resp.iterations} 余料全部避开 [40,50]")

    # 6. 材料保护不可行: 单件 300 / L=600 / 保护 [250,350] — 任何切法余料都是 300, 应返回 unplaced
    req = SolveRequest(kerf=0, items=[Item(length=300, demand=1)],
                       materials=[Material(label="L600", length=600)],
                       protect_enabled=True, protect_min=250, protect_max=350)
    resp = solve(req)
    assert resp.unplaced, "[保护不可行] 应返回 unplaced 而非硬切"
    print(f"[保护不可行] PASS unplaced={resp.unplaced}")

    print("ALL CHECKS PASSED")
    return 0


if __name__ == "__main__":
    sys.exit(main())
