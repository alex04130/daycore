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
# ⚠️ 相对 URL（../daycore-frontendcore.git）是有意的：它跟着 origin 走，所以同一份
# .gitmodules 在 SSH clone 和 HTTPS clone 下都对，换 host 也不用改。写死
# github.com 的话，任何镜像或迁移都会让每个人的 clone 断掉。
#
# 前置：五个空仓已经在远端建好（脚本不建仓 —— 建仓是账号层面的动作，应该由人做）。
#
#   daycore-frontendcore  daycore-ting  daycore-zhiyu  daycore-liuli
#   daycore-liuli-classic
#
# 仓名从 origin 推导（$base/$prefix-<后缀>.git），所以换 org 只要改 origin。
#
# 用法：
#   scripts/split-repos.sh              # 只打印计划，什么都不做
#   scripts/split-repos.sh --go         # 真做
#   scripts/split-repos.sh --go --resume  # 上次中断了，接着做
#
set -euo pipefail

DRY=1
RESUME=0
for arg in "$@"; do
  case "$arg" in
    --go) DRY=0 ;;
    # ⚠️ --resume 跳过「目标仓必须为空」那条前置。
    #
    # 它存在是因为真出过一次：五个仓推完之后，第三步在临时 clone 里 commit 时倒了
    # （那台机器只在仓局部配了 git 身份，临时 clone 继承不到）。于是仓不空了，重跑
    # 被自己的前置挡住 —— 而这时候你正需要重跑。
    #
    # 前两步是幂等的（split 是确定性的，推同样的 commit 就是 up-to-date），tag 已存在
    # 会跳过，第三步没东西可提交也会跳过。所以 --resume 是安全的，前提是那些仓里除了
    # 这个脚本推的东西没有别的。
    --resume) RESUME=1 ;;
  esac
done

# 目录 → 仓名后缀。改这张表就是改这次切分。
#
# ⚠️ 后缀不必等于目录名。core 的仓叫 daycore-frontendcore 而不是 daycore-core ——
# 「core」在一个既有后端又有前端的项目里指代不清，而仓名是那种一旦有人 clone 过就
# 很难改的东西。第一版脚本把仓名硬推成「目录名」，那是把一个巧合当成了规则。
PAIRS=(
  "packages/core:frontendcore"
  "web/ting:ting"
  "web/zhiyu:zhiyu"
  "web/liuli:liuli"
  "web/liuli-classic:liuli-classic"
)

# 哪一个是 core（四个前端要钉它，自己不钉自己）。必须是上表里的一个后缀。
CORE_NAME="${CORE_NAME:-frontendcore}"

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

# ⚠️ 五个目标仓必须存在且为空 —— 在第一个副作用之前全部检完。
#
# 这条不是礼貌，是这个脚本能不能被救回来的关键。它推完 core、又往前端仓补了一个
# commit 之后才失败的话，重跑会在 push 那一步撞非快进 —— 于是你既没切完，也回不去。
# 一次性迁移脚本的正确形状是：**所有前置在动手之前检完**，把「中途卡死」变成
# 「它不肯开工」。
#
# 真的中途断了（网断、某个仓被别人写了）怎么办：把那几个仓删了重建成空的，再跑。
# split 分支是纯派生物，超级仓这边在最后一步之前没有任何改动。
missing=0

# ⚠️ git 身份，检在这里而不是等到第三步。
#
# 这一条是买来的：第一次真跑时五个仓全推完了，才在第三步的临时 clone 里倒下 ——
# 那台机器只在仓局部配了身份（git config user.name），全局是空的，而 `git clone`
# 出来的临时目录继承不到局部配置。
#
# 教训不是「加一条身份检查」，是**前置清单要覆盖后面每一步需要的东西，不只覆盖显眼
# 的那些**。一条在副作用之后才触发的前置，等于没有前置。
AUTHOR_NAME=$(git config user.name || true)
AUTHOR_EMAIL=$(git config user.email || true)
if [ -z "$AUTHOR_NAME" ] || [ -z "$AUTHOR_EMAIL" ]; then
  say "✗ 没有 git 身份（user.name / user.email）—— 第三步要在临时 clone 里提交"
  missing=1
fi

for pair in "${PAIRS[@]}"; do
  name="${pair##*:}"
  repo="$base/${prefix}-${name}.git"
  if ! refs=$(git ls-remote "$repo" 2>/dev/null); then
    say "✗ 连不上 $repo —— 先在远端建这个空仓"
    missing=1
  elif [ -n "$refs" ] && [ "$RESUME" = 0 ]; then
    say "✗ $repo 不是空的（已有 $(printf '%s\n' "$refs" | wc -l) 个 ref）"
    say "  这个脚本只往空仓里切。上次中断了的话用 --resume；要重来就把它删掉重建。"
    missing=1
  fi
done
if [ "$missing" = 1 ]; then
  [ "$DRY" = 1 ] && say "" && say "（空跑：上面这些在 --go 时会直接拒绝）" || exit 1
fi

# ── 1+2. 切分支并推出去 ─────────────────────────────────────────────────────
for pair in "${PAIRS[@]}"; do
  path="${pair%%:*}"; name="${pair##*:}"
  repo="$base/${prefix}-${name}.git"
  say "── $path → ${prefix}-${name}"
  # ⚠️ 先删同名分支。`subtree split -b` 遇到已存在的分支直接报错，于是任何一次
  # 中途失败（网络断了、某个仓还没建）都会让重跑在第一步就死掉 —— 而那时候你
  # 正需要重跑。split 分支是纯派生物，重算它没有代价。
  run "git branch -qD 'split/$name' 2>/dev/null || true"
  run "git subtree split -P '$path' -b 'split/$name'"
  run "git push '$repo' 'split/$name:refs/heads/main'"
done

say ""
core_repo="$base/${prefix}-${CORE_NAME}.git"
if [ "$DRY" = 0 ] && git ls-remote --tags "$core_repo" 2>/dev/null | grep -q "refs/tags/$CORE_TAG\$"; then
  say "── tag $CORE_TAG 已经在 ${prefix}-${CORE_NAME} 上了，跳过"
else
  say "── ${prefix}-${CORE_NAME} 打 tag $CORE_TAG"
  run "git push '$core_repo' 'split/${CORE_NAME}:refs/tags/$CORE_TAG'"
fi

# ── 3. 前端仓改依赖 ─────────────────────────────────────────────────────────
#
# ⚠️ 在切出来的仓里改，不在超级仓里改。超级仓的 file:../../packages/core 在
# 树内是对的（workspace 会覆盖它），而独立 clone 需要一个能解析的 spec ——
# 两种世界，两个位置，脚本是知道这个映射的唯一地方。
say ""
say "── 四个前端仓：@daycore/core → git 依赖 + 生成 lockfile"
for pair in "${PAIRS[@]}"; do
  path="${pair%%:*}"; name="${pair##*:}"
  [ "$name" = "$CORE_NAME" ] && continue
  repo="$base/${prefix}-${name}.git"
  # SSH 形式的 origin（git@host:owner/repo.git）转成 npm 认识的 URL。
  case "$origin" in
    git@*) core_dep="git+ssh://${base/://}/${prefix}-${CORE_NAME}.git#${CORE_TAG}" ;;
    *)     core_dep="git+${base}/${prefix}-${CORE_NAME}.git#${CORE_TAG}" ;;
  esac
  # ⚠️ 身份显式传给临时 clone。它继承不到超级仓的局部 config，而这台机器就只有
  # 局部的 —— 这一行是一次五个仓推完之后才炸的失败换来的。
  # ⚠️ 没东西可提交时跳过，这样中断之后 --resume 不会在这里死掉。
  run "tmp=\$(mktemp -d) && git clone -q '$repo' \"\$tmp\" && \
    node -e \"const f=process.argv[1]+'/package.json',p=require(f);p.dependencies['@daycore/core']='$core_dep';require('fs').writeFileSync(f,JSON.stringify(p,null,2)+'\\n')\" \"\$tmp\" && \
    ( cd \"\$tmp\" && npm install --silent && git add -A && \
      if git diff --cached --quiet; then echo '  （已经是 git 依赖了，跳过）'; else \
        git -c user.name='$AUTHOR_NAME' -c user.email='$AUTHOR_EMAIL' \
          commit -qm '依赖 @daycore/core 改成 git 依赖：独立 clone 也能构建' && git push -q origin main; fi ) && \
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
