import type { KindSpec, Manifest, TokenSpec } from '@daycore/core';

// 琉璃 · 长卷 —— 时间是一块连续的空间画布。
//
// ⚠️ Its own family, like the other two. A theme is a set of values for ONE
// token space, and 长卷's space is a canvas's: it has a grid line, a now line,
// a ghost outline. 汀 has water; 纸屿 has paper. None of those words mean
// anything in the other two.

export const FAMILY_ID = 'liuli';
export const DISPLAY_NAME = '琉璃 · 长卷';
export const MIN_API = 1;

/**
 * ⚠️ THE THIRD frontend to need a kind the embedded six cannot express, and the
 * first to need one that is not about a value's syntax at all: a dashed outline
 * is a LIST of lengths (`4px 3px`), and `list-of<length>` is the one combinator
 * that does express it — so 长卷 proposes nothing for that.
 *
 * What it does need is `ratio-or-length`: the ghost's inset is either a
 * proportion of the lane or a fixed offset, and the two are genuinely
 * interchangeable there. `nullable<…>` and `one-of[…]` cannot union two
 * primitives, which is the gap.
 */
const RATIO_OR_LENGTH: KindSpec = {
  name: 'ratio-or-length',
  pattern: '0|0?\\.[0-9]+|1(\\.0+)?|[0-9.]+(px|rem)',
  description: '比例（0–1）或长度（px/rem）。虚影内缩可以按比例也可以按固定值',
};

export const PROPOSED_KINDS: KindSpec[] = [RATIO_OR_LENGTH];

export const TOKENS: TokenSpec[] = [
  { name: '--color-bg-start', kind: 'color', description: '画布背景渐变起点' },
  { name: '--color-bg-end', kind: 'color', description: '画布背景渐变终点' },
  { name: '--color-surface', kind: 'color', description: '块的表面' },
  { name: '--color-surface-hover', kind: 'color', description: '块 hover 时的表面' },
  { name: '--color-primary', kind: 'color', description: '主色：当前、主按钮' },
  { name: '--color-accent', kind: 'color', description: '强调色' },
  { name: '--color-text-primary', kind: 'color', description: '块标题' },
  { name: '--color-text-secondary', kind: 'color', description: '时长、标签' },
  { name: '--color-text-muted', kind: 'color', description: '小时刻度、已过去的块' },
  { name: '--color-border-custom', kind: 'color', description: '块描边与刻度线' },
  { name: '--color-states-success', kind: 'color', description: '完成' },
  { name: '--color-states-warning', kind: 'color', description: '提醒' },
  { name: '--color-states-error', kind: 'color', description: '冲突' },
  // ── 画布自己的东西 ──
  { name: '--cj-hour-line', kind: 'color', description: '整点刻度线。⚠️ 要淡到不和块抢' },
  { name: '--cj-now-line', kind: 'color', description: '「现在」那条线' },
  { name: '--cj-ghost-dash', kind: 'list-of<length>', description: '虚影的虚线节奏，如 4px 3px' },
  { name: '--cj-ghost-inset', kind: 'ratio-or-length', description: '虚影相对块的内缩' },
  { name: '--radius-card', kind: 'length', description: '块圆角' },
  { name: '--radius-sheet', kind: 'length', description: '抽屉圆角' },
];

export const THEME_RULES = [
  '长卷 是一张地图：整屏是连续的时间，块是贴在上面的东西。所以背景要能承托很多块而不喧宾夺主。',
  '⚠️ --cj-hour-line 必须非常淡。刻度线一旦和块争夺注意力，地图就退化成表格。',
  '--cj-now-line 是唯一应该被一眼看到的线，但它是 1.5px 均匀实线，不是渐变 —— 渐变会让它左粗右消、视觉偏心。',
  '过去的块饱和度更低、未来的更淡，这个梯度靠 --color-text-muted 与表面色的关系撑起来。',
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
