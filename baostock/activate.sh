#!/usr/bin/env bash
# activate.sh - 准备并激活 baostock-api 的 Python 虚拟环境 (macOS/Linux, 对应 Windows 的 activate.ps1)
#
# 用法:
#   source activate.sh            # 创建/校验 .venv + 安装依赖 + 激活到当前终端 (推荐, 提示符显示 (.venv))
#   ./activate.sh                 # 完成环境准备后自动进入一个已激活的交互子 shell (退出 exit)
#   ./activate.sh --reinstall     # 忽略缓存标记, 强制重装依赖
# 可选:
#   BAOSTOCK_PYTHON=python3.12 source activate.sh        # 指定建 venv 的解释器 (默认 python3)
#   BAOSTOCK_PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple source activate.sh  # 国内 pip 镜像
#   ./activate.sh --reinstall                            # 忽略缓存标记, 强制重装依赖

# 注意: 本脚本会同时被 source, 不要 set -e/-u (避免污染调用方 shell)
_bs_activate () {
    local self script_dir venv_path req_file marker python_bin index_url new_hash need_install force_install executed

    # 是否被"直接执行" (而非 source): 直接执行必是 bash 子进程/shebang, BASH_SOURCE[0] == $0
    # zsh 场景用 ZSH_ARGZERO 区分: source 时它仍是调用方 argv[0], 执行 ./activate.sh 时等于脚本路径
    executed=0
    if [ -n "${BASH_VERSION:-}" ]; then
        [ "${BASH_SOURCE[0]:-$0}" = "$0" ] && executed=1
    elif [ -n "${ZSH_VERSION:-}" ]; then
        [ "${ZSH_ARGZERO:-}" = "$0" ] && executed=1
    else
        executed=1
    fi

    # 定位脚本目录: bash 用 BASH_SOURCE (source 时不等于 $0), zsh 的 $0 在 source/执行时都是脚本路径
    if [ -n "${BASH_VERSION:-}" ]; then
        self="${BASH_SOURCE[0]:-$0}"
    else
        self="$0"
    fi
    script_dir="$(cd "$(dirname "$self")" 2>/dev/null && pwd -P)"
    venv_path="$script_dir/.venv"
    req_file="$script_dir/requirements.txt"
    marker="$venv_path/.requirements.md5"
    python_bin="${BAOSTOCK_PYTHON:-python3}"
    index_url="${BAOSTOCK_PIP_INDEX_URL:-https://pypi.org/simple}"

    # 0) 参数校验 (最先做, 无效用法不应动 venv)
    force_install=0
    case "${1:-}" in
        --reinstall | -r) force_install=1 ;;
        "") : ;;
        *) echo "activate.sh: 未知参数 $1 (仅支持 --reinstall)" >&2; return 1 ;;
    esac

    if [ ! -f "$req_file" ]; then
        echo "activate.sh: 未找到 requirements.txt, 请在 baostock 目录内使用本脚本" >&2
        return 1
    fi

    # 1) Python 解释器须为 3.12+ (requirements 锁定的 pandas/ortools 需要较新 wheel)
    if ! command -v "$python_bin" >/dev/null 2>&1; then
        echo "activate.sh: 未找到 $python_bin, 请先安装 Python 3.12+ (brew install python 或 python.org 安装包)" >&2
        return 1
    fi
    if ! "$python_bin" -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 12) else 1)' >/dev/null 2>&1; then
        echo "activate.sh: 需要 Python >= 3.12, 当前是 $( "$python_bin" -V 2>&1 )" >&2
        return 1
    fi

    # 2) venv 不存在或损坏 (bin/python 不可执行) 时重建
    if [ ! -x "$venv_path/bin/python" ]; then
        echo "创建虚拟环境: $venv_path"
        "$python_bin" -m venv "$venv_path" || {
            echo "activate.sh: 创建虚拟环境失败" >&2
            return 1
        }
    fi

    # 3) 激活 (source 本脚本时才作用于当前终端; 直接执行时只对本进程生效)
    if [ -r "$venv_path/bin/activate" ]; then
        . "$venv_path/bin/activate"
    fi

    # 4) 依赖: requirements 内容变化 (md5 标记) 或关键包导入失败才重装, 已一致则跳过
    need_install=0
    new_hash="$(_bs_req_hash "$req_file")"
    if [ "$force_install" = "1" ]; then
        need_install=1
    elif [ ! -f "$marker" ] || { [ -n "$new_hash" ] && [ "$new_hash" != "$(cat "$marker" 2>/dev/null)" ]; }; then
        need_install=1
    elif ! python -c 'import fastapi, uvicorn, pydantic, pandas, ortools, akshare, baostock' >/dev/null 2>&1; then
        need_install=1
    fi

    if [ "$need_install" = "1" ]; then
        echo "安装依赖 (pip 源: $index_url) ..."
        python -m pip --version >/dev/null 2>&1 || python -m ensurepip --upgrade >/dev/null 2>&1 || true
        python -m pip install -r "$req_file" -i "$index_url" || {
            echo "activate.sh: 依赖安装失败, 可换源重试 (BAOSTOCK_PIP_INDEX_URL) 或 --reinstall" >&2
            return 1
        }
        if [ -n "$new_hash" ]; then
            printf '%s\n' "$new_hash" > "$marker"
        fi
    else
        echo "依赖齐备, 跳过安装"
    fi

    echo "baostock 虚拟环境就绪: $(command -v python) [$(python -V 2>&1)]"

    if [ "$executed" = "1" ]; then
        # 直接执行: 本脚本已对子进程生效, 想立刻看到 (.venv) 提示符前缀就进入激活过的交互子 shell
        if [ -t 0 ] && [ -t 1 ] && _bs_spawn_shell "$script_dir" "$venv_path"; then
            :  # exec 成功后进程被替换, 函数不会执行到这里
        else
            echo "提示: 未进入子 shell (非交互终端或 $SHELL 不可用). 让当前终端激活请用: source activate.sh"
        fi
    else
        echo "提示: 已激活到当前终端 (提示符前缀 (.venv)), 退出 venv 用 deactivate"
    fi
    return 0
}

# 直接执行时进入已激活的交互子 shell: 通过 ZDOTDIR/--rcfile 注入
# "先加载用户 rc, 再 source 本脚本" 保证 venv 前缀与 PATH 在用户配置之后生效
_bs_spawn_shell () {
    local script_dir="$1" venv_path="$2" rc_dir sh_bin sh_name
    rc_dir="$venv_path/.shell-rc"
    sh_bin="${SHELL:-/bin/zsh}"
    if ! command -v "$sh_bin" >/dev/null 2>&1; then
        return 1
    fi
    sh_name="$(basename "$sh_bin")"
    mkdir -p "$rc_dir" || return 1
    case "$sh_name" in
        zsh)
            printf '%s\n' \
                '# 自动生成 by activate.sh (每次执行 ./activate.sh 时重写): 保留用户配置并激活 baostock venv' \
                '[ -r "$HOME/.zshrc" ] && source "$HOME/.zshrc"' \
                "source \"$script_dir/activate.sh\"" \
                > "$rc_dir/.zshrc"
            ZDOTDIR="$rc_dir" exec "$sh_bin" ;;
        bash)
            printf '%s\n' \
                '# 自动生成 by activate.sh (每次执行 ./activate.sh 时重写): 保留用户配置并激活 baostock venv' \
                '[ -r "$HOME/.bashrc" ] && . "$HOME/.bashrc"' \
                ". \"$script_dir/activate.sh\"" \
                > "$rc_dir/.bashrc"
            exec "$sh_bin" -i --rcfile "$rc_dir/.bashrc" ;;
        *)
            # 其他 shell: 只能继承已导出的 VIRTUAL_ENV/PATH, 无法再注入提示符前缀
            echo "(shell $sh_name 不支持注入 venv 前缀, venv 环境已随继承生效)"
            exec "$sh_bin" ;;
    esac
}

# requirements.txt 的 md5 (macOS 用 md5 -q, Linux 退回 md5sum)
_bs_req_hash () {
    local h=""
    h="$(md5 -q "$1" 2>/dev/null)"
    if [ -z "$h" ]; then
        h="$(md5sum "$1" 2>/dev/null | cut -d ' ' -f1)"
    fi
    printf '%s\n' "$h"
}

_bs_activate "$@"
