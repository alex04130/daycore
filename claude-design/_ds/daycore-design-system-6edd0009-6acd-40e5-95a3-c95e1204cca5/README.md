# DaycoreUI (@daycore/ui@0.1.0)

This design system is the published @daycore/ui React library, bundled as a single
browser global. All 26 components are the real upstream code.

## Where things are

- `_ds_bundle.js` — the whole-DS bundle at the project root; loads every component to `window.DaycoreUI`. First line is a `/* @ds-bundle: … */` metadata header.
- `styles.css` — the single stylesheet entry: it `@import`s the tokens, fonts, and component styles (`_ds_bundle.css`). Link this one file.
- `components/<group>/<Name>/<Name>.prompt.md` (example JSX + variants), `<Name>.d.ts` (types), `<Name>.html` (variant grid).
- `tokens/*.css` — CSS custom properties, names verbatim from upstream.
- `fonts/` — `@font-face` files + `fonts.css` (when the package ships fonts).

For a specific component, `read_file("components/<group>/<Name>/<Name>.prompt.md")`.

## Loading

Add these two lines to your page once (React must be on the page first):

```html
<link rel="stylesheet" href="styles.css">
<script src="_ds_bundle.js"></script>
```

Components are then available at `window.DaycoreUI.*`. Mount into a dedicated child node (e.g. `<div id="ds-root">`), not the host page's own React root, so the two trees don't collide:

```jsx
const { Avatar } = window.DaycoreUI;
ReactDOM.createRoot(document.getElementById('ds-root')).render(<Avatar />);
```

## Tokens

86 CSS custom properties from @daycore/ui. Names are
preserved verbatim from upstream. They are declared inside `_ds_bundle.css` (this DS ships one compiled stylesheet rather than separate token files).

- **color** (33): `--tw-border-style`, `--tw-shadow-color`, `--tw-inset-shadow-color`, …
- **spacing** (4): `--tw-inset-shadow`, `--tw-inset-shadow-alpha`, `--tw-inset-ring-shadow`, …
- **typography** (8): `--tw-font-weight`, `--font-sans`, `--font-mono`, …
- **radius** (6): `--radius-xl`, `--radius-2xl`, `--radius-card`, …
- **shadow** (4): `--tw-shadow`, `--tw-shadow-alpha`, `--tw-ring-shadow`, …
- **other** (31): `--tw-translate-x`, `--tw-translate-y`, `--tw-translate-z`, …

## Components

### general
- `Avatar` — A user avatar that shows the profile image when available and gracefully
- `Badge` — A small non-interactive status pill  fixed, done, coming soon, counts.
- `BottomSheet` — A modal sheet that slides up from the bottom on a frosted surface with a grab
- `Button` — The primary action element. Token-driven so it adopts the active theme's
- `Card` — A structured glass surface with optional header (icon  title  description 
- `ChatBubble` — A single chat message in the companion conversation. User messages are
- `Chip` — A rounded pill for filters, tags, and single-select choices (mood filters,
- `EmptyState` — The friendly nothing here yet placeholder  a haloed icon, a title, a line
- `Fab` — The floating action button  the primary add affordance, a filled circle
- `FeatureCard` — A centered feature tile  icon in a halo, label, description, and an optional
- `GlassCard` — The foundational surface of the Daycore language: a frosted-glass panel with
- `IconButton` — A circular icon-only button  used for top-bar actions, dismiss buttons, and
- `Input` — A single-line text field with the soft tinted fill and focus ring of the
- `Label` — A form field label in the design system's text-primary weight.
- `MoodTile` — A single tile in the mood-check-in grid: a big emoji over a short label on a
- `ProgressRing` — A circular progress indicator  the breathing-exercise timer and any
- `SectionHeader` — A screen / section heading: an optional eyebrow, a bold title, an optional
- `Select` — A styled dropdown built on the native select for reliable behavior on
- `Skeleton` — A shimmering placeholder block for loading states. Compose several to sketch
- `TabBar` — The frosted bottom navigation bar  evenly spaced icon+label tabs with the
- `Tabs` — A segmented tab switcher on a glass track with a sliding brand indicator 
- `Textarea` — A multi-line text field matching
- `ThemeSwitcher` — Switch between the four Daycore themes. The pill variant is a segmented
- `TimeBlockCard` — A single entry in the day timeline: a time column with an accent dot, the
- `TypingDots` — Three bouncing dots  the assistant is typing / thinking indicator used in

### theme
- `ThemeProvider` — Provides the active Daycore theme to descendants and writes the
