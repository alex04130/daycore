import type { KindSpec, Manifest, TokenSpec } from '@daycore/core';

// 琉璃初版's self-introduction.
//
// ⚠️ Its own family, and the tempting alternative is worth naming. 初版 renders
// the same design-system vocabulary the shipping web/frontend does — the twelve
// `--primary` / `--surface` / … names below are exactly the backend's
// `themeVarWhitelist` — so joining family `default` would let every theme
// already made in the old frontend keep working when this replaces it.
//
// It still declares its own, because `default` is not a design system: it is
// "the family a request that names no build is judged against"
// (internal/domain/frontend.go), the bucket for anonymous and unknown clients.
// A build that deliberately moved in would be indistinguishable from one that
// forgot to introduce itself, and the family would stop meaning anything.
//
// Theme continuity has a supported path that does not cost that:
// PUT /api/admin/frontends/builds/{hash}/family, an OPERATOR deciding at the
// moment of the switchover. That is precisely what the two-layer identity
// (build hash + declared family, operator-overridable) was built for, and it
// keeps the decision with the person who knows whether the two really do render
// the same.
export const FAMILY_ID = 'liuli-classic';

export const DISPLAY_NAME = '琉璃初版 · 页面制';

export const MIN_API = 1;

/**
 * ⚠️ EMPTY, deliberately, and the emptiness is the assertion.
 *
 * 汀 needed `shadow` on day one; 纸屿 needed `easing` and `radius`; 长卷 needed
 * `ratio-or-length`. 初版 needs nothing: every value it lets a theme touch is a
 * colour, a length or a duration, and those are three of the embedded six.
 *
 * Proposing one anyway — "for symmetry", or because the other three have some —
 * would put a decision on an operator's desk that buys nobody anything, and the
 * third tier is only cheap while that stays rare. A frontend that needs no new
 * kind should say so by declaring none.
 */
export const PROPOSED_KINDS: KindSpec[] = [];

/**
 * 初版's token space.
 *
 * ⚠️ The first twelve are the backend's built-in whitelist, name for name
 * (internal/server/handlers_themes.go). That is not a coincidence and not
 * laziness: they are the names the four shipped themes carry values for, the
 * names the theme-generation prompt has always used, and the names every theme
 * anybody has already saved is written in. Renaming them to something prettier
 * would strand all of that for no gain a reader could see.
 *
 * The last four are 初版's own, and each exists because a PAGE-BASED app has a
 * surface the other three do not: a navigation rail that is always on screen,
 * and rows of cards that need a hairline between them.
 */
export const TOKENS: TokenSpec[] = [
  // ── the twelve ──
  { name: '--primary', kind: 'color', description: '主色：按钮、选中的那一项、当前位置' },
  { name: '--accent', kind: 'color', description: '强调色，次级高亮用它，不要抢主色' },
  { name: '--bg-start', kind: 'color', description: '背景渐变的起点（页面顶部）' },
  { name: '--bg-end', kind: 'color', description: '背景渐变的终点（页面底部）' },
  { name: '--text-primary', kind: 'color', description: '正文与标题' },
  { name: '--text-secondary', kind: 'color', description: '次级文字：时间、时长、说明' },
  { name: '--text-muted', kind: 'color', description: '最弱的一级：占位、时间戳、已过去的条目' },
  { name: '--surface', kind: 'color', description: '卡片表面。半透明时会透出背景渐变' },
  { name: '--surface-hover', kind: 'color', description: '卡片被指到时的表面色，要比 --surface 明显一点' },
  { name: '--success', kind: 'color', description: '完成' },
  { name: '--warning', kind: 'color', description: '临近截止' },
  { name: '--error', kind: 'color', description: '冲突、拒绝、删除' },
  // ── 页面制自己的 ──
  { name: '--rail', kind: 'color', description: '底部标签栏与宽屏左侧导航栏的底色。它一直在屏幕上，所以不能比卡片更抢眼' },
  { name: '--line', kind: 'color', description: '分隔线。⚠️ 一列卡片之间只需要一根发丝，重了整页就变成表格' },
  { name: '--radius-card', kind: 'length', description: '卡片圆角' },
  { name: '--t-page', kind: 'duration', description: '切换页面的过渡时长' },
];

/**
 * How 初版 wants themes designed for it. Sent, stored, and NOT used until an
 * operator approves it — until then the backend writes a mechanical prompt from
 * the token list instead, so generation works from day one with zero injection
 * surface.
 */
export const THEME_RULES = [
  '琉璃初版 是页面制：底部或侧边一直有一条导航栏，内容是一列一列的卡片。所以配色要经得起「长时间不变的那两条边」+「大量重复的卡片」。',
  '--bg-start 到 --bg-end 是一层很淡的渐变，不是主角。它和 --surface 要能分出前后，但不要拉太开 —— 卡片是浮在纸上，不是发光的窗口。',
  '⚠️ --line 要非常淡（正文色的 8% 上下）。一列卡片之间只需要一根发丝，重一点整页立刻变成表格。',
  '--rail 是导航栏底色，它一直在屏幕上，所以它应该是全屏最安静的一块，不要用主色去填。',
  '三级文字（--text-primary / -secondary / -muted）的对比度要真的拉开：最弱那级用在已经过去的条目上，要能一眼看出「这条不用管了」，但不能看起来像坏了。',
  '--primary 克制使用，它标的是「你现在在这儿」和「按这个」，不是装饰。',
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
