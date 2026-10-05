"""一维切割精确求解 (OR-Tools 列生成), 挂载在 baostock sidecar 的 /cut1d/solve。

算法 (参考外部 Java 版 CuttingBarService, 修正其定价跨类型 kerf 近似):
  1. 主问题 LP (GLOP): min Σ 新料长度
     约束 = 需求覆盖 (≥) + 旧料来源共享约束 (同长度组的所有列合计 ≤ 根数,
     修掉"每列各自有界但合计超卖"的问题)
  2. 定价 (CP-SAT 有界背包): 件重 w = len+kerf, 容量 C = L+kerf
     (整根浪费 = C − Σ a×w, 对跨类型 kerf 精确, 无需 Java 版的同型捆近似)
     reduced cost = cost − Σ dual_demand×a − dual_source; < −1e-6 加列
  3. 整数化两阶段 (CBC):
     stage1: min Σ 新料长度 (多规格下比"最少根数"更合理)
     stage2: 新料长度预算 ≤ stage1, 需求改精确相等, 最小化偏好成本:
             1000×(1−util^weight) [利用率偏好] − 30×非零类型数 [偏好混合切割]
             − 1000 [偏好用旧料];  列不足导致不可行时回退 stage1 解 (同 Java 版)
"""

import math
import time
from typing import Optional

from fastapi import APIRouter
from pydantic import BaseModel, Field
from ortools.linear_solver import pywraplp
from ortools.sat.python import cp_model

router = APIRouter()

EPS = 1e-6
MAX_ITERATIONS = 300
MAX_SCRAP_GROUPS = 64  # 旧料按长度分组定价, 防御异常库存规模
UNLIMITED = 10**9


class Item(BaseModel):
    length: float = Field(gt=0)
    demand: int = Field(ge=0)


class Material(BaseModel):
    label: str = ""
    length: float = Field(gt=0)


class Scrap(BaseModel):
    length: float = Field(gt=0)


class SolveRequest(BaseModel):
    kerf: float = Field(default=0, ge=0)
    items: list[Item]
    materials: list[Material] = []
    scraps: list[Scrap] = []
    time_limit_ms: int = Field(default=3000, ge=200, le=60000)
    utilization_weight: float = Field(default=5, ge=1, le=8)


class Bar(BaseModel):
    source: str  # "material" | "scrap"
    material_index: Optional[int] = None
    scrap_length: Optional[float] = None
    pattern: list[int]  # 每种零件的切割数量 (按 items 下标)
    count: int = 1      # 同模式同来源的根数


class SolveResponse(BaseModel):
    bars: list[Bar]
    unplaced: list[dict]  # [{item_index, count}]
    iterations: int = 0
    elapsed_ms: int = 0


def _scale_for(kerf: float) -> int:
    """离散化倍率: 使 kerf×scale 为整数 (精度最高 0.01)"""
    for s in (1, 10, 100):
        if kerf * s == float(int(kerf * s)):
            return s
    return 100


def _price_knapsack(duals: list[float], weights: list[int], caps: list[int],
                    capacity: int, time_limit_s: float) -> tuple[Optional[list[int]], float]:
    """CP-SAT 有界背包定价: max Σ dual_i×a_i s.t. Σ w_i×a_i ≤ capacity, 0≤a_i≤caps_i。
    CP-SAT 仅支持整数: weights/capacity 必须已离散化为整数, 对偶系数放大 1e6 取整。"""
    m = cp_model.CpModel()
    xs = []
    for i, w in enumerate(weights):
        if w <= 0 or caps[i] <= 0 or duals[i] <= EPS:
            xs.append(None)
            continue
        upper = min(caps[i], int(capacity // w))
        if upper <= 0:
            xs.append(None)
            continue
        xs.append(m.NewIntVar(0, upper, f"a{i}"))
    terms = [xs[i] * int(round(duals[i] * 1e6)) for i in range(len(xs)) if xs[i] is not None]
    if not terms:
        return None, 0.0
    m.Add(sum(xs[i] * weights[i] for i in range(len(xs)) if xs[i] is not None) <= capacity)
    m.Maximize(sum(terms))
    solver = cp_model.CpSolver()
    solver.parameters.max_time_in_seconds = max(0.05, time_limit_s)
    solver.parameters.num_search_workers = 2
    status = solver.Solve(m)
    if status not in (cp_model.OPTIMAL, cp_model.FEASIBLE):
        return None, 0.0
    pattern = [solver.Value(x) if x is not None else 0 for x in xs]
    value = sum(duals[i] * pattern[i] for i in range(len(pattern)))
    return pattern, value


def _build_master(columns: list[tuple[int, list[int]]], sources: list[dict], demand: list[int],
                  integer: bool, time_limit_ms: int = 0, demand_exact: bool = False,
                  new_length_budget: Optional[float] = None, util_weight: float = 5.0):
    """构建主问题 (LP=GLOP / IP=CBC)。
    返回 (solver, var_rows, demand_cons, source_cons); 不可用返回 (None, ...)。
    - 需求约束: ≥ demand (demand_exact=True 时 == demand, 供 stage2 防超切)
    - 来源共享约束: 有限容量来源 (旧料长度组) 的所有列合计 ≤ 根数
    - new_length_budget: 给定时加 Σ_新料列 长度×x ≤ 预算 (stage2), 并把目标替换为
      利用率偏好成本: 1000×(1−util^weight) − 30×非零类型数 − 1000×是否旧料"""
    kind = "CBC" if integer else "GLOP"
    solver = pywraplp.Solver.CreateSolver(kind)
    if solver is None:
        return None, None, None, None
    if integer and time_limit_ms > 0:
        solver.SetTimeLimit(max(200, time_limit_ms))

    var_rows = []
    for s_i, pat in columns:
        if integer:
            v = solver.IntVar(0, int(sources[s_i]["cap"]), "")
        else:
            v = solver.NumVar(0.0, sources[s_i]["cap"], "")
        var_rows.append((v, s_i, pat))

    demand_cons = []
    for i, d in enumerate(demand):
        if d <= 0:
            demand_cons.append(None)
            continue
        expr = sum(v * pat[i] for v, _, pat in var_rows)
        demand_cons.append(solver.Add(expr == d) if demand_exact else solver.Add(expr >= d))

    source_cons: dict[int, object] = {}
    by_source: dict[int, list[int]] = {}
    for j, (_, s_i, _) in enumerate(var_rows):
        by_source.setdefault(s_i, []).append(j)
    for s_i, js in by_source.items():
        if sources[s_i]["cap"] < UNLIMITED:
            source_cons[s_i] = solver.Add(sum(var_rows[j][0] for j in js) <= sources[s_i]["cap"])

    objective = solver.Objective()
    if new_length_budget is None:
        for v, s_i, _ in var_rows:
            objective.SetCoefficient(v, sources[s_i]["cost"])
        objective.SetMinimization()
    else:
        if new_length_budget > 0:
            budget_terms = [v * sources[s_i]["cost"] for v, s_i, _ in var_rows if sources[s_i]["kind"] == "material"]
            if budget_terms:
                solver.Add(sum(budget_terms) <= new_length_budget)
        for v, s_i, pat in var_rows:
            s = sources[s_i]
            cuts = sum(pat)
            used = sum(pat[i] * s["ref_len"][i] for i in range(len(pat))) + s["ref_kerf"] * max(0, cuts - 1)
            util = min(1.0, used / s["C_len"]) if s["C_len"] > 0 else 0.0
            nonzero = sum(1 for q in pat if q > 0)
            cost = 1000.0 * (1.0 - util ** util_weight)
            if nonzero >= 2:
                cost -= 30.0 * nonzero
            if s["kind"] == "scrap":
                cost -= 1000.0
            objective.SetCoefficient(v, cost)
        objective.SetMinimization()
    return solver, var_rows, demand_cons, source_cons


def _read_solution(var_rows) -> list[tuple[int, int, list[int]]]:
    """在 solver 存活期内读出整数解 (SWIG 变量代理随 solver 销毁, 返回后读取是悬空指针)"""
    return [(int(round(v.solution_value())), s_i, pat) for v, s_i, pat in var_rows]


@router.post("/cut1d/solve", response_model=SolveResponse)
def solve(req: SolveRequest) -> SolveResponse:
    t0 = time.time()
    deadline_ms = req.time_limit_ms
    elapsed = lambda: (time.time() - t0) * 1000  # noqa: E731

    n = len(req.items)
    demand = [it.demand for it in req.items]
    scale = _scale_for(req.kerf)
    weights = [int((it.length + req.kerf) * scale + 1e-9) for it in req.items]

    def all_unplaced(iters: int) -> SolveResponse:
        return SolveResponse(
            bars=[],
            unplaced=[{"item_index": i, "count": d} for i, d in enumerate(demand) if d > 0],
            iterations=iters, elapsed_ms=int(elapsed()))

    if n == 0:
        return SolveResponse(bars=[], unplaced=[], iterations=0, elapsed_ms=int(elapsed()))

    # 求解来源: 新料规格 (不限根数, 成本=长度) + 旧料长度分组 (限根数, 成本 0)
    sources: list[dict] = []
    for mi, mat in enumerate(req.materials):
        sources.append({
            "kind": "material", "material_index": mi, "cost": mat.length,
            "C": int((mat.length + req.kerf) * scale + 1e-9),
            "C_len": mat.length, "ref_len": [it.length for it in req.items],
            "ref_kerf": req.kerf, "cap": UNLIMITED,
        })
    scrap_groups: dict[float, list[int]] = {}
    for _si, sc in enumerate(req.scraps):
        scrap_groups.setdefault(sc.length, []).append(_si)
    for length in sorted(scrap_groups, reverse=True)[:MAX_SCRAP_GROUPS]:
        idxs = scrap_groups[length]
        sources.append({
            "kind": "scrap", "scrap_length": length, "scrap_idxs": idxs, "cost": 0.0,
            "C": int((length + req.kerf) * scale + 1e-9),
            "C_len": length, "ref_len": [it.length for it in req.items],
            "ref_kerf": req.kerf, "cap": len(idxs),
        })

    # 初始列: 每来源 × 每零件单件模式 (保证主问题可行)
    columns: list[tuple[int, list[int]]] = []
    seen: set[tuple[int, tuple[int, ...]]] = set()
    for s_i, s in enumerate(sources):
        for i in range(n):
            if demand[i] > 0 and weights[i] <= s["C"]:
                pat = [0] * n
                pat[i] = 1
                key = (s_i, tuple(pat))
                if key not in seen:
                    seen.add(key)
                    columns.append((s_i, pat))

    # 列生成迭代: LP 对偶 → CP-SAT 背包定价 (reduced cost) → 加列
    iters = 0
    for iters in range(1, MAX_ITERATIONS + 1):
        if elapsed() > deadline_ms * 0.6:
            break
        solver, _rows, demand_cons, source_cons = _build_master(columns, sources, demand, integer=False)
        if solver is None or solver.Solve() != pywraplp.Solver.OPTIMAL:
            return all_unplaced(iters)
        duals = [0.0] * n
        for i, c in enumerate(demand_cons):
            if c is not None:
                duals[i] = max(0.0, c.dual_value())
        src_duals = {s_i: c.dual_value() for s_i, c in source_cons.items()}
        improved = False
        budget_slice = max(0.05, (deadline_ms * 0.6 - elapsed()) / 1000.0 / max(1, len(sources)))
        for s_i, s in enumerate(sources):
            if elapsed() > deadline_ms * 0.6:
                break
            pat, val = _price_knapsack(duals, weights, demand, s["C"], budget_slice)
            if pat is None:
                continue
            # reduced cost = cost − Σ dual_demand×a − dual_source (来源共享约束的对偶 ≤ 0)
            rc = s["cost"] - val - src_duals.get(s_i, 0.0)
            if rc >= -1e-6:
                continue
            key = (s_i, tuple(pat))
            if key in seen:
                continue
            seen.add(key)
            columns.append((s_i, pat))
            improved = True
        if not improved:
            break

    # 整数化 stage1: min Σ 新料长度
    remaining = int(max(200, deadline_ms - elapsed()))
    half = remaining // 2
    s1, rows1, _, _ = _build_master(columns, sources, demand, integer=True, time_limit_ms=half)
    if s1 is None:
        return all_unplaced(iters)
    st1 = s1.Solve()
    if st1 not in (pywraplp.Solver.OPTIMAL, pywraplp.Solver.FEASIBLE):
        return all_unplaced(iters)
    solution = _read_solution(rows1)
    new_len_used = sum(sources[s_i]["cost"] * count for count, s_i, _ in solution
                       if sources[s_i]["kind"] == "material")

    # 整数化 stage2: 新料长度预算内, 精确覆盖需求, 优化利用率/混合度/旧料偏好
    try:
        s2, rows2, _, _ = _build_master(columns, sources, demand, integer=True,
                                        time_limit_ms=max(200, remaining - half),
                                        demand_exact=True,
                                        new_length_budget=new_len_used + 0.01,
                                        util_weight=req.utilization_weight)
        st2 = s2.Solve() if s2 is not None else None
        if st2 in (pywraplp.Solver.OPTIMAL, pywraplp.Solver.FEASIBLE):
            solution = _read_solution(rows2)
    except Exception:  # noqa: BLE001 — stage2 失败一律回退 stage1 解
        pass

    bars: list[Bar] = []
    covered = [0] * n
    for count, s_i, pat in solution:
        if count <= 0:
            continue
        s = sources[s_i]
        covered = [covered[i] + pat[i] * count for i in range(n)]
        if s["kind"] == "material":
            bars.append(Bar(source="material", material_index=s["material_index"], pattern=pat, count=count))
        else:
            bars.append(Bar(source="scrap", scrap_length=s["scrap_length"], pattern=pat, count=count))
    unplaced = [{"item_index": i, "count": demand[i] - covered[i]}
                for i in range(n) if demand[i] - covered[i] > 0]
    return SolveResponse(bars=bars, unplaced=unplaced, iterations=iters, elapsed_ms=int(elapsed()))
