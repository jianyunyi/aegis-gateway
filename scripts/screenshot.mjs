/**
 * 管理后台演示截图脚本（Playwright 无头浏览器）。
 *
 * 用法：
 *   1. 确保全栈已启动（docker compose up -d），后台在 http://localhost:3000
 *   2. 安装依赖（独立目录，避免污染 web/）：
 *        npm install playwright --prefix .cache/screenshot
 *        npx --prefix .cache/screenshot playwright install chromium
 *      （国内网络可加 --registry=https://registry.npmmirror.com）
 *   3. 运行：
 *        node scripts/screenshot.mjs
 *
 * 产物：docs/images/{login,dashboard,evals,eval-report,logs}.png
 */
import { fileURLToPath } from 'node:url';
import { chromium } from '../.cache/screenshot/node_modules/playwright/index.mjs';

const BASE = process.env.AEGIS_URL || 'http://localhost:3000';
// fileURLToPath 会正确解码 %20 等转义（pathname 不会）
const OUT = fileURLToPath(new URL('../docs/images/', import.meta.url));

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2 });

const shot = async (name) => {
  await page.screenshot({ path: `${OUT}${name}.png` });
  console.log(`saved: docs/images/${name}.png`);
};

// 1. 登录页
await page.goto(`${BASE}/login`);
await page.waitForTimeout(800);
await shot('login');

// 2. 登录 → 仪表盘（AntD Form id 格式：{formName}_{fieldName}）
await page.fill('#login_username', 'admin');
await page.fill('#login_password', 'admin123');
await page.getByRole('button', { name: '登 录' }).click();
await page.waitForURL('**/dashboard');
await page.waitForTimeout(2000);
await shot('dashboard');

// 3. 评测页
await page.goto(`${BASE}/evals`);
await page.waitForTimeout(1500);
await shot('evals');

// 3.1 选中第一个数据集（表格行点击）
const firstRow = page.locator('.ant-table-tbody > tr').first();
if (await firstRow.isVisible().catch(() => false)) {
  await firstRow.click();
  await page.waitForTimeout(1200);
  // 切到「评测运行」Tab
  await page.getByRole('tab', { name: /评测运行/ }).click();
  await page.waitForTimeout(1200);
  await shot('evals-runs');

  // 4. 评测报告（打开第一个已完成运行的报告抽屉）
  const reportBtn = page.getByRole('button', { name: '查看报告' }).first();
  if (await reportBtn.isEnabled().catch(() => false)) {
    await reportBtn.click();
    await page.waitForTimeout(1500);
    await shot('eval-report');
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
  }
}

// 5. 调用日志页
await page.goto(`${BASE}/logs`);
await page.waitForTimeout(1500);
await shot('logs');

await browser.close();
console.log('done.');
