# AEGIS Admin UI Layout Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变登录页、路由、接口和业务交互的前提下，将 AEGIS 核心管理后台统一为稳定专业的深蓝导航 + 浅色工作区视觉系统。

**Architecture:** 保留 Next.js 14 + Ant Design 5 的现有结构。通过 `AntdProvider` 的 `ConfigProvider` 统一主题 token，通过 `SideLayout` 统一壳层和内容容器，再在六个业务页中只做页面标题区、工具栏、卡片和表格间距的定向调整。页面数据加载、表单、弹窗、表格列和 API 调用保持原实现。

**Tech Stack:** Next.js 14 App Router, React 18, TypeScript, Ant Design 5, `@ant-design/icons`, ECharts。

---

## 文件结构与职责

- Modify: `web/components/AntdProvider.tsx`：集中配置 Ant Design 主题 token、控件尺寸和组件样式。
- Modify: `web/components/SideLayout.tsx`：实现深蓝侧栏、品牌区、顶栏、响应式内容容器和统一页面背景。
- Modify: `web/app/dashboard/page.tsx`：统一页面标题区和指标/趋势卡片层级。
- Modify: `web/app/keys/page.tsx`：统一标题、创建操作、成功提示和表格卡片布局。
- Modify: `web/app/providers/page.tsx`：统一标题、创建操作和提供商表格布局。
- Modify: `web/app/logs/page.tsx`：将筛选工具栏与日志表格整理为响应式页面头部和卡片。
- Modify: `web/app/billing/page.tsx`：统一账单标题区、汇总表格和空数据呈现。
- Modify: `web/app/evals/page.tsx`：整理评测页数据集区、主要操作区和 Tabs 容器间距，不改变交互。
- Verify: `web/app/login/page.tsx`：确认文件无改动且登录视觉与行为保持不变。

### Task 1: Establish the global Ant Design visual tokens

**Files:**
- Modify: `web/components/AntdProvider.tsx`

- [ ] **Step 1: Add the approved theme tokens to `ConfigProvider`**

Keep `zhCN`, `dayjs` and `App` unchanged. Add a `theme` prop with a light algorithm and these stable tokens: primary blue `#2f6fed`, dark text `#10253f`, body background `#f3f6fa`, border radius `10`, control height `38`. Add component tokens for `Layout`, `Menu`, `Card`, `Table`, `Button`, `Input` and `Modal` so all pages share the same surface, selected state, header padding and focus treatment.

- [ ] **Step 2: Run the existing lint command**

Run: `npm --prefix web run lint`

Expected: lint completes without new errors. If the repository's Next lint command reports only pre-existing warnings, record them and continue.

- [ ] **Step 3: Commit the theme-only change**

Run:

```bash
git add web/components/AntdProvider.tsx
git commit -m "style(web): define AEGIS admin theme tokens"
```

### Task 2: Rebuild the shared admin shell

**Files:**
- Modify: `web/components/SideLayout.tsx`

- [ ] **Step 1: Preserve auth and route selection logic**

Do not change `TOKEN_KEY`, the `useEffect` redirect, logout behavior, `MENU_ITEMS`, selected-key prefix matching or `router.push` calls.

- [ ] **Step 2: Apply the approved shell layout**

Keep `Layout`, `Sider`, `Header` and `Content`, but use the theme tokens instead of one-off inline colors. Add a compact brand block with `AEGIS` and a small `AI GATEWAY` label, keep the sidebar width at a readable desktop size, use the current selected item with a blue-tinted active background, and keep `collapsedWidth={64}` for narrow screens. Give the header a light surface, a subtle bottom border, and a compact right-side workspace/logout group. Give `Content` a responsive container with `maxWidth: 1440`, centered margins, and mobile padding no smaller than `16px`.

- [ ] **Step 3: Verify all menu routes remain unchanged**

Run: `Select-String -Path web/components/SideLayout.tsx -Pattern "'/dashboard'|'/keys'|'/providers'|'/logs'|'/billing'|'/evals'`

Expected: all six existing route keys are present.

- [ ] **Step 4: Commit the shared shell**

Run:

```bash
git add web/components/SideLayout.tsx
git commit -m "style(web): refresh admin navigation shell"
```

### Task 3: Normalize page headers and dashboard hierarchy

**Files:**
- Modify: `web/app/dashboard/page.tsx`

- [ ] **Step 1: Replace the standalone title with the shared page-header pattern**

Keep the title text `仪表盘`. Add a short secondary description such as `监控今日调用、成本与模型稳定性` below it. Keep the existing data loading and error branches unchanged.

- [ ] **Step 2: Tune dashboard surfaces without changing metrics**

Keep the four existing statistics and the trend chart. Use the page content width supplied by `SideLayout`, add consistent grid gaps, and use the theme's card padding. Keep `ReactECharts`, `chartOption`, data labels, endpoint text and empty/error handling unchanged.

- [ ] **Step 3: Commit the dashboard visual pass**

Run:

```bash
git add web/app/dashboard/page.tsx
git commit -m "style(web): improve dashboard hierarchy"
```

### Task 4: Normalize operational table pages

**Files:**
- Modify: `web/app/keys/page.tsx`
- Modify: `web/app/providers/page.tsx`
- Modify: `web/app/logs/page.tsx`
- Modify: `web/app/billing/page.tsx`

- [ ] **Step 1: Apply the same page-header and toolbar rhythm**

For each page, retain the existing title and all handlers. Add only a concise description below the title, then place the primary action or filter in the card header/toolbar. Keep the existing Chinese labels, request behavior and form field names.

- [ ] **Step 2: Keep data tables behaviorally identical**

Do not change `columns`, `rowKey`, pagination callbacks, `scroll` widths, status tags, `Popconfirm`, `Modal`, `Alert`, or summary calculations. Only adjust card class/style props, spacing, title typography and responsive toolbar layout.

- [ ] **Step 3: Verify route-specific controls are still present**

Run:

```powershell
Select-String -Path web/app/keys/page.tsx,web/app/providers/page.tsx,web/app/logs/page.tsx,web/app/billing/page.tsx -Pattern '新建|Input.Search|pagination|Table|Modal|Popconfirm'
```

Expected: Key retains create modal and confirmation, providers retains its create controls, logs retains search and pagination, billing retains table summary.

- [ ] **Step 4: Commit the table-page pass**

Run:

```bash
git add web/app/keys/page.tsx web/app/providers/page.tsx web/app/logs/page.tsx web/app/billing/page.tsx
git commit -m "style(web): unify operational page layouts"
```

### Task 5: Refine the evaluation workspace layout

**Files:**
- Modify: `web/app/evals/page.tsx`

- [ ] **Step 1: Preserve the evaluation state and API flow**

Do not rename state variables, form names, endpoint paths, Tabs keys, modal handlers, report parsing or table column definitions.

- [ ] **Step 2: Recompose only the visual grouping**

Add the same page header pattern. Place dataset selection and dataset actions into a distinct surface, keep the existing sample/run actions grouped beside the selected dataset, and give the Tabs and report content a consistent card/container treatment. Use responsive `Space`/`Flex` props or CSS class names so actions wrap on narrow screens.

- [ ] **Step 3: Verify evaluation actions remain available**

Run:

```powershell
Select-String -Path web/app/evals/page.tsx -Pattern '新建数据集|从日志采样|手动添加样本|运行 A/B|查看报告|Tabs'
```

Expected: all five existing action intents and the Tabs component remain present.

- [ ] **Step 4: Commit the evaluation visual pass**

Run:

```bash
git add web/app/evals/page.tsx
git commit -m "style(web): clarify evaluation workspace layout"
```

### Task 6: Build and perform visual/regression verification

**Files:**
- Verify: `web/app/login/page.tsx`
- Verify: all modified files under `web/components` and `web/app`

- [ ] **Step 1: Confirm login page is unchanged**

Run: `git diff HEAD~5 -- web/app/login/page.tsx`

Expected: no diff caused by this work. If the commit count differs because commits were squashed, use `git diff <pre-ui-commit>..HEAD -- web/app/login/page.tsx`.

- [ ] **Step 2: Run lint and production build**

Run:

```bash
npm --prefix web run lint
npm --prefix web run build
```

Expected: both commands complete successfully.

- [ ] **Step 3: Check the routes and responsive layout manually**

Start the app with `npm --prefix web run dev`. After authentication, inspect `/dashboard`, `/keys`, `/providers`, `/logs`, `/billing` and `/evals` at desktop width and a narrow mobile width. Confirm the sidebar collapse, page titles, primary actions, table horizontal scrolling, loading/error/empty states, modals and report drawer.

- [ ] **Step 4: Review the final diff for scope control**

Run:

```bash
git status --short
git diff --stat HEAD~6..HEAD
```

Expected: only the approved theme, shared shell and six core business pages changed; `web/app/login/page.tsx` and backend files remain untouched.

- [ ] **Step 5: Commit any final verification-only corrections**

If verification reveals a real visual regression, patch the smallest affected file, rerun lint/build, then commit with `style(web): polish admin layout verification`. Do not alter routes, API behavior or login page.

## Plan self-review

- Spec coverage: global theme, shell, six core pages, responsive behavior, motion restraint, accessibility and verification are covered by Tasks 1 through 6.
- Placeholder scan: no placeholder markers, vague future steps or unspecified files are used.
- Type consistency: the plan keeps existing Ant Design components, handlers, endpoint strings and state names; no new shared types or dependencies are introduced.
- Scope check: the plan stays within the approved UI-only redesign and explicitly excludes login, routes, APIs and business refactoring.
