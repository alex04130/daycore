// Full-app drive: walks every screen against the real backend (real AI calls),
// uploading the date-relative fixtures through the actual file inputs, and
// saves one screenshot per screen for prototype comparison.
// Usage: node scripts/drive.mjs <baseURL> <fixtureDir> <outDir>
import { chromium } from 'playwright-core';

const exe = process.env.CHROMIUM ||
  '/home/alex/.cache/ms-playwright/chromium_headless_shell-1223/chrome-headless-shell-linux64/chrome-headless-shell';
const [base, fixDir, outDir] = process.argv.slice(2);

const browser = await chromium.launch({ executablePath: exe });
const page = await browser.newPage({ viewport: { width: 420, height: 900 } });
const errors = [];
page.on('console', (m) => { if (m.type() === 'error' && !m.text().includes('Warning:')) errors.push(m.text()); });
page.on('pageerror', (e) => errors.push(String(e)));
const shot = (name) => page.screenshot({ path: `${outDir}/${name}.png` });
const step = async (label, fn) => {
  try { await fn(); console.log('OK ', label); }
  catch (e) { console.log('FAIL', label, '—', e.message.slice(0, 200)); await shot('FAIL-' + label.replace(/\W+/g, '_')); }
};

await page.goto(base, { waitUntil: 'networkidle' });
await page.waitForTimeout(1000);

await step('onboarding-welcome', async () => {
  await page.click('text=开始');
  await page.waitForTimeout(600);
  await shot('01-ob-import');
});

await step('onboarding-import-canvas', async () => {
  const inputs = page.locator('.dc-ob input[type=file]');
  await inputs.nth(0).setInputFiles(fixDir + '/canvas-now.json');
  await page.waitForSelector('text=已导入', { timeout: 15000 });
});

await step('onboarding-import-ics', async () => {
  const inputs = page.locator('.dc-ob input[type=file]');
  await inputs.nth(1).setInputFiles(fixDir + '/ics-now.ics');
  await page.waitForFunction(() => document.querySelectorAll('.dc-ob .dc-set-main + *').length >= 0, null, { timeout: 15000 });
  await page.waitForTimeout(2500);
  await shot('02-ob-imported');
});

await step('onboarding-finish', async () => {
  await page.click('text=下一步');
  await page.waitForTimeout(700);
  await shot('03-ob-ready');
  await page.click('text=先自己看看'); // AI auto-plan tested later from Today
  await page.waitForTimeout(1200);
});

await step('today-rules-visible', async () => {
  await page.waitForSelector('.dc-hero-cta', { timeout: 8000 });
  await page.waitForTimeout(1200);
  await shot('04-today-rules');
});

await step('today-autoplan-REAL-AI', async () => {
  await page.click('.dc-hero-cta');
  await page.waitForTimeout(900);
  await shot('05-autoplan-sheet');
  await page.click('text=开始规划');
  await page.waitForSelector('.dc-gen-overlay', { timeout: 5000 });
  await page.waitForSelector('.dc-plan-list', { timeout: 90000 });
  await page.waitForTimeout(1500);
  await shot('06-today-planned');
});

await step('block-detail', async () => {
  await page.locator('.dc-block-wrap').first().click();
  await page.waitForTimeout(800);
  await shot('07-block-detail');
  await page.keyboard.press('Escape');
  await page.locator('.dc-app-sheet button', { hasText: '取消' }).first().click({ timeout: 2000 }).catch(() => {});
  await page.mouse.click(210, 60); // close sheet by tapping backdrop area
  await page.waitForTimeout(600);
});

await step('materials', async () => {
  await page.click('nav >> text=资料');
  await page.waitForTimeout(1000);
  await shot('08-materials');
});

await step('rules-subpage', async () => {
  await page.locator('.dc-set-row', { hasText: '重复规则' }).first().click();
  await page.waitForSelector('text=新建规则', { timeout: 8000 });
  await page.waitForTimeout(600);
  await shot('09-rules');
  await page.locator('button[aria-label="返回"]').first().click();
  await page.waitForTimeout(500);
});

await step('companion-REAL-AI-rule-update', async () => {
  await page.click('nav >> text=陪伴');
  await page.waitForTimeout(600);
  await page.fill('.dc-composer textarea', '以后每3天提醒我给绿萝浇水，早上八点');
  await page.click('.dc-send-btn');
  await page.waitForSelector('.dc-chat-action-card', { timeout: 90000 });
  await page.waitForTimeout(800);
  await shot('10-companion-rule');
});

await step('companion-REAL-AI-memory-update', async () => {
  await page.fill('.dc-composer textarea', '记住：我晚上11点以后就不想干活了');
  await page.click('.dc-send-btn');
  await page.waitForSelector('text=已记住', { timeout: 90000 });
  await page.waitForTimeout(800);
  await shot('11-companion-memory');
});

await step('mood-REAL-AI', async () => {
  await page.click('nav >> text=心情');
  await page.waitForTimeout(600);
  await page.click('text=压力大');
  await page.waitForSelector('.dc-mood-reply', { timeout: 60000 });
  await page.waitForTimeout(800);
  await shot('12-mood-reply');
});

await step('settings-memory-version', async () => {
  await page.click('nav >> text=设置');
  await page.waitForTimeout(900);
  await shot('13-settings-top');
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await page.waitForTimeout(600);
  await shot('14-settings-bottom');
});

await step('language-switch-EN', async () => {
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.click('.dc-seg-item >> text=EN');
  await page.waitForTimeout(900);
  await shot('15-settings-en');
  await page.click('nav >> text=Today');
  await page.waitForTimeout(900);
  await shot('16-today-en');
});

console.log(errors.length ? 'PAGE ERRORS:\n' + errors.map((e) => ' - ' + e.slice(0, 250)).join('\n') : 'no page errors');
await browser.close();
