import { Screen } from '../ui.jsx';

// Every section starts here and is replaced by a real view. It exists so the
// shell is navigable while the eight screens are written one at a time — a nav
// entry that renders nothing is indistinguishable from a broken one.
export function Placeholder() {
  return (
    <Screen title="还没写" sub="这一屏正在做。" state={{ status: 'ok' }}>
      <div className="placeholder">—</div>
    </Screen>
  );
}
