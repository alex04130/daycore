// Screenshot helper for screen-by-screen comparison against the prototype.
// Usage: node scripts/shot.mjs <url> <out.png> [clickSelector...] [--full]
import { chromium } from 'playwright-core';

const exe = process.env.CHROMIUM ||
  '/home/alex/.cache/ms-playwright/chromium_headless_shell-1223/chrome-headless-shell-linux64/chrome-headless-shell';

const [url, out, ...rest] = process.argv.slice(2);
const full = rest.includes('--full');
const clicks = rest.filter((x) => x !== '--full');

const browser = await chromium.launch({ executablePath: exe });
const page = await browser.newPage({ viewport: { width: 420, height: 900 } });
const errors = [];
page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
page.on('pageerror', (e) => errors.push(String(e)));
await page.goto(url, { waitUntil: 'networkidle', timeout: 30000 });
await page.waitForTimeout(1200);
for (const sel of clicks) {
  try {
    await page.click(sel, { timeout: 4000 });
    await page.waitForTimeout(900);
  } catch (e) {
    errors.push('click failed: ' + sel + ' — ' + e.message);
  }
}
await page.screenshot({ path: out, fullPage: full });
console.log('saved', out);
if (errors.length) {
  console.log('PAGE ERRORS:');
  errors.forEach((e) => console.log(' -', e.slice(0, 300)));
}
await browser.close();
