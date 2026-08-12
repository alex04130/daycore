#!/usr/bin/env python3
"""算出 @daycore/core 真正的最低兼容 API 版本。

core 的版本号语义是「**兼容的最低** API 版本」（作者 2026-08-12 拍板）。这个脚本
是那个数字的来源，存在的理由很直接：**一个没人能重算的数字，下次就只能靠抄**，而抄
一个会随端点增加而变化的数字，就是让它慢慢变成谎话。

怎么算：对 endpoints.ts 里每一条路径，找它第一次出现在这个仓库里的 commit，读那个
commit 的 internal/version/version.go。取最大值 —— 最晚出现的那条决定了下界。

⚠️ 它测得出的下界不会低于**仓库历史的起点**。本仓第一个 commit（2026-07-25）时
Version 已经是 2.2.x，所以任何早于它的端点都只会报 2.2。这不是缺陷，是诚实：更老的
后端这个仓库无法作证，声称支持它们就是编的。

⚠️ 它也**不验证** core 真的能对着那个版本的后端跑 —— 那需要一个那么老的后端。
它验证的是「core 用到的端点在那时都已经存在」，这是必要条件不是充分条件。真正验证
「对当前后端能跑」的是 internal/server/core_client_test.go 的路由核对。
"""
import re
import subprocess
import sys

CORE = 'packages/core/src/endpoints.ts'


def sh(*args: str) -> str:
    return subprocess.run(args, capture_output=True, text=True).stdout


def strip_comments(src: str) -> str:
    src = re.sub(r'/\*.*?\*/', '', src, flags=re.S)
    return re.sub(r'(?m)^\s*//.*$', '', src)


def version_at(commit: str) -> tuple[int, int] | None:
    m = re.search(r'Version\s*=\s*"(\d+)\.(\d+)', sh('git', 'show', f'{commit}:internal/version/version.go'))
    return (int(m.group(1)), int(m.group(2))) if m else None


def main() -> int:
    try:
        src = strip_comments(open(CORE).read())
    except FileNotFoundError:
        print(f'{CORE} 不在 —— packages/core 没 checkout？（make submodules）')
        return 1

    paths = sorted({
        re.sub(r'\$\{[^}]*\}', '{x}', p.split('?')[0]).rstrip('/')
        for p in re.findall(r"""['"`](/api/[^'"`]*)['"`]""", src)
    })
    if not paths:
        print('一条路径都没读到 —— 提取器和 endpoints.ts 的形状对不上了')
        return 1

    worst, worst_path, unknown = (0, 0), '', []
    for p in paths:
        # 占位符前面的稳定片段：`git log -S` 找的是字面串，`{x}` 不会出现在源码里。
        needle = p.split('/{x}')[0]
        commits = sh('git', 'log', '--reverse', '--format=%H', '-S', needle,
                     '--', 'api/openapi.yaml', 'internal/server/').split()
        got = version_at(commits[0]) if commits else None
        if got is None:
            unknown.append(p)
            continue
        if got > worst:
            worst, worst_path = got, p

    print(f'core 调用 {len(paths)} 条路径')
    print(f'最晚出现的：{worst_path} → API {worst[0]}.{worst[1]}')
    print(f'最低兼容    = {worst[0]}.{worst[1]}   （packages/core 的 SPEAKS 应该是这个）')
    if unknown:
        print(f'\n⚠️ {len(unknown)} 条查不到出生点，没有计入：')
        for u in unknown:
            print('   ', u)
        print('  查不到通常意味着提取的片段太短或太泛。计入一个错的下界比漏掉它更糟，')
        print('  所以它们被跳过而不是当成 0 —— 但这也意味着这个结果是下界的下界。')
    return 0


if __name__ == '__main__':
    sys.exit(main())
