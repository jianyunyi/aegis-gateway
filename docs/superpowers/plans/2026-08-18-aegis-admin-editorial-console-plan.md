# AEGIS Admin Editorial Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restyle the authenticated AEGIS admin pages into the approved Editorial Console direction without changing routes, API calls, forms, auth, or business behavior.

**Architecture:** Keep Ant Design 5 as the only UI system. Centralize color, radius, table, card, button, input, modal, drawer, tabs, and typography changes in `web/components/AntdProvider.tsx`, then layer app-specific shell and page classes in `web/components/SideLayout.tsx`. Apply small page-level class names only where hierarchy needs to change, especially dashboard metrics and dense eval/report cards.

**Tech Stack:** Next.js 14 app router, React 18 client components, Ant Design 5, `@ant-design/icons`, ECharts, TypeScript.

---

## File Structure

- Modify `web/components/AntdProvider.tsx`: define the Editorial Console theme tokens and component tokens.
- Modify `web/components/SideLayout.tsx`: rebuild shared shell styling, brand mark, top bar, page header, card/table utility classes, and responsive rules.
- Modify `web/app/dashboard/page.tsx`: add metric-card hierarchy classes and chart card class.
- Modify `web/app/keys/page.tsx`: add table-card class and keep Key security alert behavior.
- Modify `web/app/providers/page.tsx`: add table-card class.
- Modify `web/app/logs/page.tsx`: add table-card class and mono numeric/cost classes where useful.
- Modify `web/app/billing/page.tsx`: add table-card class and mono numeric/cost classes where useful.
- Modify `web/app/evals/page.tsx`: add eval-specific card classes for the dataset pane, detail pane, empty state, and drawer stats.
- Verify with `next lint` and `next build` from the existing bundled/runtime-compatible Node command if global npm is unavailable.

## Task 1: Central Theme Tokens

**Files:**
- Modify: `web/components/AntdProvider.tsx`

- [ ] **Step 1: Replace default blue theme tokens with Editorial Console tokens**

Set these values in `ConfigProvider.theme.token`:

```tsx
colorPrimary: '#20242a',
colorInfo: '#ff6b4a',
colorText: '#24211d',
colorTextHeading: '#24211d',
colorTextSecondary: '#6d6257',
colorBorder: '#ded5c7',
colorBorderSecondary: '#e8dfd2',
colorBgLayout: '#f3efe6',
colorBgContainer: '#fffaf1',
colorFillAlter: '#f7f1e8',
borderRadius: 10,
controlHeight: 38,
fontFamily:
  '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
```

- [ ] **Step 2: Update component tokens**

Use warm paper backgrounds for `Layout`, `Card`, `Table`, `Input`, `Modal`, `Drawer`, `Tabs`, `Tag`, and `Alert`. Primary buttons must use dark text/background contrast through `Button.primaryColor: '#fff7ec'`, `Button.defaultBg: '#fffaf1'`, `Button.defaultBorderColor: '#d8cfc3'`, `Button.defaultColor: '#24211d'`.

- [ ] **Step 3: Verify theme compiles**

Run:

```powershell
Set-Location web
& 'C:\Users\asus\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe' node_modules\next\dist\bin\next lint
```

Expected: lint succeeds or unrelated existing lint failures are reported.

## Task 2: Shared Shell And Global UI Classes

**Files:**
- Modify: `web/components/SideLayout.tsx`

- [ ] **Step 1: Update the shared shell**

Change the side navigation to a墨黑 `#20242a` shell with an orange `A` brand tile, AEGIS wordmark, and preserved `MENU_ITEMS`. The top bar should show left context text `Gateway Control` and preserve the right-side `AEGIS 管理后台` plus logout button.

- [ ] **Step 2: Add global app classes**

Add global classes in the existing `<style jsx global>` block:

```css
.aegis-page-shell
.aegis-brand-mark
.aegis-content
.aegis-page-header
.aegis-page-header-kicker
.aegis-page-header-description
.aegis-surface-card
.aegis-table-card
.aegis-metric-card
.aegis-metric-card-feature
.aegis-mono
.aegis-loading-panel
.aegis-toolbar
```

These classes must use warm paper surfaces, 8-12px radii, light warm borders, no decorative gradient blobs, no large shadows, and responsive single-column behavior under `575px`.

- [ ] **Step 3: Verify routes still use the same keys**

Confirm `MENU_ITEMS` keys remain:

```tsx
'/dashboard'
'/keys'
'/providers'
'/logs'
'/billing'
'/evals'
```

## Task 3: Dashboard Hierarchy

**Files:**
- Modify: `web/app/dashboard/page.tsx`

- [ ] **Step 1: Add Editorial Console page header kicker**

Under the existing `Typography.Title`, add:

```tsx
<Typography.Text className="aegis-page-header-kicker">Gateway Control</Typography.Text>
```

Keep the visible title `仪表盘` and description text unchanged.

- [ ] **Step 2: Restyle metric cards**

Apply `className="aegis-metric-card aegis-metric-card-feature"` to the first metric card and `className="aegis-metric-card"` to the other metric cards. Add `valueStyle={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace' }}` to each `Statistic`.

- [ ] **Step 3: Restyle the trend card**

Set the trend `Card` to `className="aegis-table-card"` and keep the ECharts option data unchanged.

## Task 4: Table Pages

**Files:**
- Modify: `web/app/keys/page.tsx`
- Modify: `web/app/providers/page.tsx`
- Modify: `web/app/logs/page.tsx`
- Modify: `web/app/billing/page.tsx`

- [ ] **Step 1: Add consistent page header kicker**

Add `Typography.Text className="aegis-page-header-kicker"` to each page header:

```tsx
Key Registry
Provider Routing
Request Ledger
Daily Costing
```

Keep each Chinese title and description unchanged.

- [ ] **Step 2: Restyle table cards**

Set each main list card to `className="aegis-table-card"`.

- [ ] **Step 3: Add mono numeric styles**

Use `className="aegis-mono"` on request IDs, key prefixes, Base URLs, cost values, token counts, and billing summary numbers where the value is rendered with `Typography.Text` or a string wrapper can be safely introduced without changing data.

## Task 5: Evals Dense Layout

**Files:**
- Modify: `web/app/evals/page.tsx`

- [ ] **Step 1: Add page header kicker**

Add:

```tsx
<Typography.Text className="aegis-page-header-kicker">Evaluation Lab</Typography.Text>
```

Keep the title `评测` and description unchanged.

- [ ] **Step 2: Restyle dataset and detail cards**

Add `className="aegis-table-card"` to the dataset card, selected dataset card, empty-state card, drawer statistic cards, and other major eval cards.

- [ ] **Step 3: Keep all interactions unchanged**

Do not change calls to `/evals/datasets`, `/evals/datasets/:id/samples`, `/evals/datasets/:id/sample`, `/evals/samples`, `/evals/samples/:id/label`, `/evals/runs`, or `/evals/runs/:id/report`.

## Task 6: Verification, Commit, Push

**Files:**
- Verify only; no source changes expected unless verification finds a UI regression or TypeScript issue.

- [ ] **Step 1: Run lint**

Run:

```powershell
Set-Location web
& 'C:\Users\asus\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe' node_modules\next\dist\bin\next lint
```

Expected: `✔ No ESLint warnings or errors`.

- [ ] **Step 2: Run production build**

Run:

```powershell
Set-Location web
& 'C:\Users\asus\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe' node_modules\next\dist\bin\next build
```

Expected: build exits 0 and includes `/dashboard`, `/keys`, `/providers`, `/logs`, `/billing`, and `/evals`.

- [ ] **Step 3: Commit**

Run:

```bash
git add web/components/AntdProvider.tsx web/components/SideLayout.tsx web/app/dashboard/page.tsx web/app/keys/page.tsx web/app/providers/page.tsx web/app/logs/page.tsx web/app/billing/page.tsx web/app/evals/page.tsx docs/superpowers/plans/2026-08-18-aegis-admin-editorial-console-plan.md
git commit -m "style(web): apply editorial admin console theme"
```

- [ ] **Step 4: Push**

Run:

```bash
git push origin main
```

Expected: `main` pushes successfully to `https://github.com/jianyunyi/aegis-gateway`.
