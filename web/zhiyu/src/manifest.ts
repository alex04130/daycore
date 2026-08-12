import type { KindSpec, Manifest, TokenSpec } from '@daycore/core';

// 纸屿's self-introduction.
//
// ⚠️ A DIFFERENT family from 汀, and that is the whole point of families: 纸屿
// is a narrative ledger on paper, 汀 is a single-piece flow on water. A theme
// made for one is meaningless in the other — different tokens, different
// number of them, different idea of what a surface is. Sharing a family would
// mean every 汀 theme showing up in 纸屿's picker, applying half of itself.

export const FAMILY_ID = 'zhiyu';
export const DISPLAY_NAME = '纸屿 · 顺流';
export const MIN_API = 1;

/**
 * The kinds 纸屿 needs and the deployment does not ship.
 *
 * ⚠️ Two of them, and neither is expressible with the embedded six — which is
 * the second independent frontend to land in that position on day one. 汀
 * needed `shadow`; 纸屿 needs these. The third tier is not an edge case.
 */
const EASING: KindSpec = {
  // ⚠️ Only cubic-bezier() and the CSS keywords. `steps()` is deliberately out:
  // it produces a visibly different KIND of motion, and a theme that could
  // silently turn every transition into a stutter is a theme that can break the
  // product rather than restyle it.
  name: 'easing',
  pattern: 'cubic-bezier\\([0-9.,\\s-]+\\)|linear|ease|ease-in|ease-out|ease-in-out',
  description: '缓动曲线：cubic-bezier(…) 或 linear/ease/ease-in/ease-out/ease-in-out',
};

const RADIUS: KindSpec = {
  // A radius is a length OR a percentage. `length` rejects the percentage and
  // 纸屿's pin is `--pin-shape: 50%` — a круг. So this is length ∪ percentage,
  // which no combinator over the embedded six expresses.
  name: 'radius',
  pattern: '0|[0-9.]+(px|rem)|[0-9.]+%',
  description: '圆角：0、长度（px/rem）或百分比（50% 就是圆）',
};

export const PROPOSED_KINDS: KindSpec[] = [EASING, RADIUS];

/**
 * 纸屿's token space.
 *
 * ⚠️ Deliberately NOT every --dc-* variable in the design system. The stylesheet
 * has around forty; most are internal plumbing that a person theming a paper
 * would never want to touch, and offering them all makes the theme editor a
 * wall of names nobody can act on. What is here is what the handoff calls
 * themeable — including the three pin variables, which it names explicitly as
 * "AI 生成主题时可覆盖".
 */
export const TOKENS: TokenSpec[] = [
  // ── the paper itself ──
  { name: '--dc-bg', kind: 'color', description: '桌面：纸张下面那一层' },
  { name: '--dc-surface', kind: 'color', description: '纸色。卡片就是纸，所以这是最主要的一个' },
  { name: '--dc-surface-2', kind: 'color', description: '纸上再压一层的小块' },
  { name: '--dc-line', kind: 'color', description: '格线／行线／分隔线。⚠️ 极淡才像纸' },
  // ── ink ──
  { name: '--dc-ink', kind: 'color', description: '正文墨色' },
  { name: '--dc-ink-2', kind: 'color', description: '次级墨色：时间、时长' },
  { name: '--dc-ink-3', kind: 'color', description: '最弱：注释、已过去的条目' },
  // ── accent ──
  { name: '--dc-accent', kind: 'color', description: '主色：按钮、当前位置' },
  { name: '--dc-accent-2', kind: 'color', description: '副强调色，图钉默认取它' },
  { name: '--dc-accent-ink', kind: 'color', description: '主色上面的文字' },
  { name: '--dc-accent-soft', kind: 'color', description: '主色的极淡版本，做底' },
  // ── status ──
  { name: '--dc-ok', kind: 'color', description: '完成' },
  { name: '--dc-warn', kind: 'color', description: '提醒' },
  { name: '--dc-err', kind: 'color', description: '冲突／拒绝' },
  // ── shape ──
  { name: '--dc-r-card', kind: 'radius', description: '卡片圆角' },
  { name: '--dc-r-sm', kind: 'radius', description: '小元件圆角' },
  { name: '--dc-r-sheet', kind: 'radius', description: '抽屉／面板圆角' },
  // ── motion ──
  { name: '--dc-t1', kind: 'duration', description: '快：hover、按下' },
  { name: '--dc-t2', kind: 'duration', description: '中：卡片进出' },
  { name: '--dc-t3', kind: 'duration', description: '慢：整屏切换' },
  { name: '--dc-ease', kind: 'easing', description: '默认缓动' },
  // ── the pin ──
  { name: '--pin-c', kind: 'color', description: '图钉颜色' },
  { name: '--pin-sz', kind: 'length', description: '图钉直径' },
  { name: '--pin-shape', kind: 'radius', description: '图钉形状：50% 是圆，小值是方钉' },
];

export const THEME_RULES = [
  '纸屿 的视觉语法是「贴在纸上的东西」，所以配色要先像一张真的纸。',
  '⚠️ 真实的纸在屏幕上是干净的纯色加极淡的规则线条 —— --dc-line 要非常淡（墨色的 4%~9% 那种淡），加重它会立刻变成表格而不是纸。',
  '--dc-surface 是纸色，它和 --dc-bg（桌面）要能分出前后，但不要拉开太多：纸不发光。',
  '三级墨色（--dc-ink / -2 / -3）拉开层次，最弱那级要真的退到纸里去。',
  '--dc-accent 克制使用，它是钢笔的颜色不是荧光笔。',
].join('\n');

export function manifest(buildHash: string): Manifest {
  return {
    familyId: FAMILY_ID,
    buildHash,
    displayName: DISPLAY_NAME,
    version: __APP_VERSION__,
    minApi: MIN_API,
    theme: { tokens: TOKENS, kinds: PROPOSED_KINDS, rules: THEME_RULES },
  };
}
