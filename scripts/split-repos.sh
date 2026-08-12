#!/usr/bin/env bash
#
# 阶段 κ：把 packages/core 与四个前端切成兄弟仓，再作为 submodule 挂回来。
#
# ⚠️ 这个脚本是这次动作唯一可执行的定义。文档会漂，脚本不会 —— 而这是一次
# 会推到远端的、别人看得见的改动，所以「照着文档手工做一遍」正是不该发生的事。
#
# 它做什么：
#   1. git subtree split 每个目录 → 一个只含该子树历史的分支（历史保留）
#   2. 推到对应的空仓；core 打上 tag
#   3. 每个前端仓里把 @daycore/core 从 file: 改成 git 依赖，装一次生成 lock
#   4. 超级仓里把那五个目录换成 submodule（相对 URL）
#
# ⚠️ 相对 URL（../daycore-core.git）是有意的：它跟着 origin 走，所以同一份
# .gitmodules 在 SSH clone 和 HTTPS clone 下都对，换 host 也不用改。写死
# github.com 的话，任何镜像或迁移都会让每个人的 clone 断掉。
#
# 前置：五个空仓已经在远端建好（脚本不建仓 —— 建仓是账号层面的动作，应该由人做）。
#
#   daycore-core  daycore-ting  daycore-zhiyu  daycore-liuli  daycore-liuli-classic
#
# 用法：
#   scripts/split-repos.sh              # 只打印计划，什么都不做
#   scripts/split-repos.sh --go         # 真做
#
set -euo pipefail

DRY=1
[ "${1:-}" = "--go" ] && DRY=0

# 目录 → 仓名。改这张表就是改这次切分。
PAIRS=(
  "packages/core:core"
  "web/ting:ting"
  "web/zhiyu:zhiyu"
  "web/liuli:liuli"
  "web/liuli-classic:liuli-classic"
)

# core 的第一个 tag。四个前端会钉它。
CORE_TAG="${CORE_TAG:-v0.1.0}"

root=$(git rev-parse --show-toplevel)
cd "$root"

origin=$(git remote get-url origin)
# 兄弟仓的基址：origin 去掉最后一段。
base="${origin%/*}"
prefix=$(basename "$origin" .git)   # 期望是 "daycore"

say() { printf '%s\n' "$*"; }
run() {
  if [ "$DRY" = 1 ]; then printf '  $ %s\n' "$*"; else eval "$@"; fi
}

say "origin      : $origin"
say "兄弟仓基址  : $base/${prefix}-<name>.git"
say "core tag    : $CORE_TAG"
say ""

if [ "$DRY" = 1 ]; then
  say "⚠️ 这是空跑。确认下面的计划之后加 --go。"
  say ""
fi

# ── 0. 前置检查 ─────────────────────────────────────────────────────────────
#
# ⚠️ 脏工作树切出来的分支不含未提交的改动，而它们会在 git rm 那一步被删掉。
# 那是这个脚本唯一能造成不可逆数据损失的地方，所以它是第一条检查。
if [ -n "$(git status --porcelain)" ]; then
  if [ "$DRY" = 1 ]; then
    # 空跑要能在脏树上看计划 —— 不然「先看看它要干什么」这件事本身被挡住了。
    say "⚠️ 工作树不干净。--go 会拒绝：未提交的改动会在换 submodule 那一步被删掉。"
    say ""
  else
    say "✗ 工作树不干净。先提交或 stash —— 未提交的改动会在换 submodule 那一步被删掉。"
    exit 1
  fi
fi
if [ -f .gitmodules ]; then
  say "✗ 已经有 .gitmodules 了，看起来切过一次。这个脚本不做增量。"
  exit 1
fi

# ── 1+2. 切分支并推出去 ─────────────────────────────────────────────────────
for pair in "${PAIRS[@]}"; do
  path="${pair%%:*}"; name="${pair##*:}"
  repo="$base/${prefix}-${name}.git"
  say "── $path → ${prefix}-${name}"
  run "git subtree split -P '$path' -b 'split/$name'"
  run "git push '$repo' 'split/$name:refs/heads/main'"
done

say ""
say "── core 打 tag $CORE_TAG"
run "git push '$base/${prefix}-core.git' 'split/core:refs/tags/$CORE_TAG'"

# ── 3. 前端仓改依赖 ─────────────────────────────────────────────────────────
#
# ⚠️ 在切出来的仓里改，不在超级仓里改。超级仓的 file:../../packages/core 在
# 树内是对的（workspace 会覆盖它），而独立 clone 需要一个能解析的 spec ——
# 两种世界，两个位置，脚本是知道这个映射的唯一地方。
say ""
say "── 四个前端仓：@daycore/core → git 依赖 + 生成 lockfile"
for pair in "${PAIRS[@]}"; do
  path="${pair%%:*}"; name="${pair##*:}"
  [ "$name" = "core" ] && continue
  repo="$base/${prefix}-${name}.git"
  # SSH 形式的 origin（git@host:owner/repo.git）转成 npm 认识的 URL。
  case "$origin" in
    git@*) core_dep="git+ssh://${base/://}/${prefix}-core.git#${CORE_TAG}" ;;
    *)     core_dep="git+${base}/${prefix}-core.git#${CORE_TAG}" ;;
  esac
  run "tmp=\$(mktemp -d) && git clone -q '$repo' \"\$tmp\" && \
    node -e \"const f=process.argv[1]+'/package.json',p=require(f);p.dependencies['@daycore/core']='$core_dep';require('fs').writeFileSync(f,JSON.stringify(p,null,2)+'\\n')\" \"\$tmp\" && \
    ( cd \"\$tmp\" && npm install --silent && git add -A && \
      git commit -qm '依赖 @daycore/core 改成 git 依赖：独立 clone 也能构建' && git push -q origin main ) && \
    rm -rf \"\$tmp\""
done

# ── 4. 超级仓换 submodule ───────────────────────────────────────────────────
say ""
say "── 超级仓：五个目录 → submodule（相对 URL）"
for pair in "${PAIRS[@]}"; do
  path="${pair%%:*}"; name="${pair##*:}"
  run "git rm -rq --cached '$path' && rm -rf '$path'"
  run "git submodule add '../${prefix}-${name}.git' '$path'"
done
run "git commit -qm 'κ：core 与四端切成兄弟仓，作为 submodule 挂回来'"

say ""
if [ "$DRY" = 1 ]; then
  say "空跑结束。加 --go 真做。"
else
  say "切完了。接下来："
  say "  make submodules      # 别人 clone 之后的第一步"
  say "  npm install          # 根上一次，workspace 会把 core 解析到 packages/core"
  say "  make test            # submodules-present 会挡住没 init 的情况"
fi
