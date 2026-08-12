// 汀's self-introduction: who it is, and what it can theme.
//
// This is the thing POST /api/version wants. See docs/specs/frontend-manifest.md
// — the backend adjudicates and answers with which family it assigned us, which
// tokens were new, and which of our proposed kinds are still waiting on a human.

// ⚠️ The SHAPES come from @daycore/core; the CONTENT below is 汀's own. Its
// token space and family id are its identity — four products do not share those,
// and a shared constant would make adding a frontend an edit to everybody.
import type { KindSpec, Manifest, TokenSpec } from '@daycore/core';

// ⚠️ FAMILY_ID is the theme-compatibility group, not the build.
//
// Every 汀 build — web, a Capacitor shell, whatever comes later — declares
// "ting", so a theme somebody made on their phone is the same theme on their
// desktop. The operator can move a build to another family from the console,
// and that assignment WINS over this constant; the handshake response's
// assignedFamilyId is the answer, not this.
export const FAMILY_ID = 'ting';

export const DISPLAY_NAME = '汀 · 此刻';

// MIN_API is the oldest backend this build knows how to talk to.
//
// Bumped when 汀 starts *requiring* something new — not when it starts using
// something new behind a capability check. The difference matters: the first
// locks out deployments, the second degrades on them.
export const MIN_API = 1;

/**
 * The shadow kind.
 *
 * ⚠️ This is the third tier, and 汀 needs it on day one — which is a decent
 * sign the tier was worth building. `--shadow` is `0 24px 70px rgba(0,0,0,.55)`,
 * and none of the six embedded kinds can express it: `list-of<length>` rejects
 * the colour in the fourth position, and a colour kind rejects the offsets.
 *
 * Until an operator approves it, the backend holds `--shadow` out of the family
 * and says so in `deferredTokens`. 汀 keeps working — its own stylesheet default
 * applies, and the only thing missing is the ability to theme that one value.
 * That degradation is the whole design: propose, keep working, wait for a human.
 *
 * The pattern is deliberately tight. Three lengths and one colour function, in
 * that order, and nothing else — an inset shadow or a spread list would be a
 * different kind rather than a looser version of this one, because a pattern an
 * operator cannot read at a glance is a pattern they cannot meaningfully approve.
 */
const SHADOW_KIND: KindSpec = {
  name: 'shadow',
  // ⚠️ Each offset is `(0|number+unit)`, not `number+unit`. A unitless zero is
  // legal CSS and 汀's own default shadow starts with one — the first draft
  // demanded a unit everywhere and therefore REFUSED THE VALUE IT SHIPS WITH.
  // Caught by running this pattern through the backend's own registry
  // (internal/theme/zz_ting_kind_test.go), which is the only place the two
  // halves meet.
  pattern:
    '(0|-?[0-9.]+(px|rem)) (0|-?[0-9.]+(px|rem)) (0|-?[0-9.]+(px|rem)) rgba?\\([0-9.,%\\s]+\\)',
  description: '投影：X偏移 Y偏移 模糊 颜色，如 0 24px 70px rgba(0,0,0,.55)',
};

export const PROPOSED_KINDS: KindSpec[] = [SHADOW_KIND];

/**
 * 汀's token space.
 *
 * ⚠️ These names and kinds must match what src/theme.css actually reads, or the
 * theme editor offers a variable that changes nothing — the most confusing
 * possible outcome, because it looks like the backend lost the value.
 *
 * The descriptions are not decoration: they go to the model that generates
 * themes (docs/AI.md), so "次级文字" earns its keep and "ink2" would not.
 */
export const TOKENS: TokenSpec[] = [
  { name: '--tg-bg', kind: 'color', description: '底色，整屏最深的一层' },
  { name: '--tg-bg2', kind: 'color', description: '上半部渐变的起点，比底色略亮' },
  { name: '--tg-water', kind: 'color', description: '底部那一汪水光的颜色' },
  { name: '--tg-ink', kind: 'color', description: '正文与大标题' },
  { name: '--tg-ink2', kind: 'color', description: '次级文字：解释、元信息' },
  { name: '--tg-ink3', kind: 'color', description: '最弱的文字：提示、时间戳' },
  { name: '--tg-line', kind: 'color', description: '分隔线与描边，通常是半透明的正文色' },
  { name: '--tg-card', kind: 'color', description: '浮起来的卡片表面' },
  { name: '--tg-card2', kind: 'color', description: '卡片里再深一层的小块' },
  { name: '--tg-accent', kind: 'color', description: '强调色：主按钮、当前状态' },
  { name: '--tg-accent-ink', kind: 'color', description: '强调色上面的文字，要和它有足够对比' },
  { name: '--tg-warm', kind: 'color', description: '暖色点缀，用在进度与少量提示上' },
  { name: '--tg-soft', kind: 'color', description: '强调色的极淡版本，用作 hover 底' },
  { name: '--tg-shadow', kind: 'shadow', description: '卡片投影' },
];

/**
 * How 汀 wants themes designed for it. Sent, stored, and NOT used until an
 * operator approves it — see docs/specs/frontend-manifest.md. Unapproved, the
 * backend writes a mechanical prompt from the token list instead, so theme
 * generation works from day one and the injection surface is zero.
 */
export const THEME_RULES = [
  '汀 是单件流：整屏只回答一个问题，所以配色要能撑住一个很大的标题和大片留白。',
  '底部有一汪水光（--tg-water 的径向渐变），它应该比 --tg-bg 亮一点点、并且偏向 --tg-accent 的色系，像水面反着光。',
  '三级文字（--tg-ink / --tg-ink2 / --tg-ink3）的对比度要拉开：大标题必须清晰，最弱那级要真的退到背景里去。',
  '--tg-accent 是唯一的高饱和色，克制使用；--tg-warm 只是它的对位，不要抢。',
].join('\n');

/** The whole manifest, as POST /api/version wants it. */
export function manifest(buildHash: string): Manifest {
  return {
    familyId: FAMILY_ID,
    buildHash,
    displayName: DISPLAY_NAME,
    version: __TING_VERSION__,
    minApi: MIN_API,
    theme: {
      tokens: TOKENS,
      kinds: PROPOSED_KINDS,
      rules: THEME_RULES,
    },
  };
}
