// Loads the vendored DaycoreUI design-system bundle. Import order matters:
// globals.js first (window.React), then the bundle side-effect.
import './globals.js';
import '../vendor/ds-bundle.js';
import '../ds/styles.css';

const UI = window.DaycoreUI;
export default UI;
