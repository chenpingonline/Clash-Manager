# 前端设计评审与优化建议

- 评审对象：`web/`（Vue 3 + TypeScript + Vite，8 个页面 / 41 个组件 / 20 个 scoped style 块）
- 评审方式：源码通读 + 构建产物测量 + 既有渲染截图比对（`.cache/ui-design-qa/`）+ 颜色对比度实算 + 编译产物核验
- 本次为**只读评审**，未修改任何代码，未涉及版本号 / CHANGELOG / 打包。

## 结论摘要

信息架构与视觉方向已经成熟：8 页统一头部、侧栏工作区、明暗双主题、全量 i18n、成熟的无障碍范式（`AppSelect` 键盘模型、原生 `<dialog>` 弹窗、`CollapseTransition`）。**主要问题不是"缺设计"，而是"设计系统没有收敛"** —— token 与历史硬编码长期并存，弹窗/开关/焦点管理等基座存在多套实现，导致浅色主题出现**实际不可读的文字**、深色主题出现**看不见的控件**，以及 55% 的样式声明无法安全修改。

按影响排序的 7 个关键数字：

| # | 问题 | 实测 |
| --- | --- | --- |
| 1 | 浅色主题下深色文字色未覆盖 | 26 处深色文字色中 **18 处无浅色覆盖**，最差 `.quota-line strong` **1.49:1**（截图已复现） |
| 2 | 深色主题开关关闭态 | 轨道与卡片背景 **1.01:1**，描边 1.36:1（需 3:1） |
| 3 | 弹窗焦点管理 | `BaseModal` 支撑 **12 个弹窗**，均无初始焦点/Tab 陷阱/焦点归还，6 个无法用 Esc 关闭 |
| 4 | 表单标签未关联控件 | 101 个 `<label>` 中 **25 个**既无 `for` 也不包裹控件 |
| 5 | 设计 token 漂移 | `styles.css` **568 处裸色值**（374 hex + 194 rgb/rgba），其中 **55 处**仍是旧强调色 `rgba(124,156,255,·)` |
| 6 | 尺度无阶梯 | 字号 **14 种**（`<12px` 占 188/367 = **51%**）、圆角 **51 种**、断点 **20 个** |
| 7 | 基座重复实现 | 弹窗 **2 套**（div + 原生 dialog）、焦点陷阱 **5 份**、遮罩 **3 套**、开关 **2 套 4 尺寸** |

---

## 一、P1：建议本迭代修复

### P1-1 浅色主题有 18 处深色文字色没有覆盖 —— 部分文字实际不可读

- 机制：`styles.css` 把浅色当作"覆盖层"（181 个 `html[data-theme="light"]` 选择器）。不透明**背景**几乎都覆盖了，但**文字颜色**漏了一批：这些颜色只在深色语境下定义，浅色下直接落在白底上。
- 实测（白底，下为逐条复核过的样例）：`.quota-line strong{#cbd4e4}` [styles.css:47](web/src/styles.css#L47) = **1.49:1**；`.quota-line{#8f9aaf}` = 2.83:1；`.quota-expire{#8290a8}` = 3.23:1；`.quick-label{#aeb6c7}` [:162](web/src/styles.css#L162) = 2.04:1；`.proxy-chevron{#9ea8bc}` [:35](web/src/styles.css#L35) = 2.39:1；`.settings-accordion-chevron{#9ea8bc}` [:194](web/src/styles.css#L194) 及其展开态 `#dce3f2` [:198](web/src/styles.css#L198) = 2.39 / **1.29:1**；`.proxy-env-key{#a9bbff}` [:130](web/src/styles.css#L130) = 1.87:1；`.local-config-icon{#9db5ff}` [:53](web/src/styles.css#L53) = 2.0:1。
- 视觉复现：`.cache/ui-design-qa/final-profiles.png`（浅色订阅配置页）中"剩余 **994.6 GB**"几乎看不见，"已用 5.4 GB"同样极淡 —— 与 1.49:1 的计算一致。
- 为什么一直没被发现：`ui-polish.css:196-213` 把仪表盘的同名元素（`.up/.down`、`.node-count`、`.node-delay.*`）改回了主题 token，看起来"浅色已经做完了"，但 Profile 卡片、设置手风琴、代理筛选这些位置没被覆盖到。
- 修复：删除这些字面色，改用 `var(--muted)` / `var(--accent-ink)` / `var(--text)`；并加一条 CI 守卫（`:root` 之外的裸 hex 直接报错），避免再次回流。

### P1-2 深色主题下"关闭态开关"几乎不可见

- 实测：轨道 `--surface-disabled:#222325` 与卡片 `--panel:#232425` 对比度 **1.01:1**，1px 描边 `--line:#383a3d` 仅 **1.36:1**（[ui-polish.css:198](web/src/ui-polish.css#L198)、[styles.css:2-3](web/src/styles.css#L2-L3)）。截图 `dark-dashboard.png` 中"系统代理 / 虚拟网卡(TUN)模式"只剩一个悬浮白点，看不出是开关、也看不出状态。
- 建议：新增开关语义 token（数值已按 3:1 校准）：

```css
:root { --switch-track:#5c636f; --switch-track-border:#6d7380; }          /* 2.57:1 / 3.27:1 */
html[data-theme="light"] { --switch-track:#cbd2dd; --switch-track-border:#8b96a8; }
.switch > span { background: var(--switch-track); border-color: var(--switch-track-border); }
```

### P1-3 `BaseModal` 缺少焦点管理，12 个弹窗受影响

- [BaseModal.vue:10](web/src/components/BaseModal.vue#L10) 声明了 `role="dialog" aria-modal="true"`，但组件体内没有任何 keydown / 焦点代码：**不设初始焦点、不锁 Tab、不归还焦点、不锁 body 滚动、不响应 Esc**，背景也没有 `inert`（`inert` 目前只用于弹窗叠弹窗，如 [ConfigEditor.vue:93](web/src/components/ConfigEditor.vue#L93)）。
- 无法用 Esc 关闭：[TunSettingsModal.vue](web/src/components/TunSettingsModal.vue)、[SystemProxySettingsModal.vue](web/src/components/SystemProxySettingsModal.vue)、[PortConflictHelp.vue](web/src/components/PortConflictHelp.vue)、[RulesPage.vue](web/src/pages/RulesPage.vue)、[ProxiesPage.vue](web/src/pages/ProxiesPage.vue)、[ProfilesPage.vue](web/src/pages/ProfilesPage.vue)。Tab 会直接走到弹窗背后的页面；关闭后焦点掉到 `<body>`。
- 项目里已有正确范式：[SequenceFormDialog.vue:34](web/src/components/SequenceFormDialog.vue#L34)、[GroupMembersModal.vue:33](web/src/components/GroupMembersModal.vue#L33) 用原生 `<dialog>` + `showModal()`，`@cancel` 关窗，浏览器自带焦点陷阱与惰性化。
- 建议：`BaseModal` 改为原生 `<dialog showModal()>`，或抽 `useModalDialog()`（初始焦点 / Esc / Tab 循环 / 记录并归还 `document.activeElement` / 滚动锁）并在 `BaseModal` 内统一实现，随后删除 5 份手写陷阱（`ChangelogModal`、`RuleImportModal`、`ConfigEditor`、`SequenceFormDialog`、`GroupMembersModal`）。顺带补上弹窗的移动端约束（当前 `max-height:90vh`、`padding:20px` 无 ≤680px 规则，`vh` 在移动浏览器会被地址栏裁切）。

### P1-4 其余对比度不达标（含一处特异性反转）

| 元素 | 主题 | 实测 | 位置 |
| --- | --- | --- | --- |
| `.node-type`（协议，11px 大写） | 浅色 | **3.67:1** | [styles.css:82](web/src/styles.css#L82) |
| `.node-type` | 深色 | 4.27:1 | [styles.css:39](web/src/styles.css#L39) |
| `.log-debug`（12px 等宽日志正文） | 浅色 | **3.61:1** | [styles.css:88](web/src/styles.css#L88) |
| `--warn` 文本（10–11px 警告/待保存） | 浅色 | **3.89:1** | [styles.css:73](web/src/styles.css#L73) |
| `.node-row.active .node-select`（选中节点名） | 浅色 | **4.38:1** | [ui-polish.css:203](web/src/ui-polish.css#L203) |
| 搜索框 `×` 清空按钮 | 浅色 | **2.43:1** | 见下 |

- `.search-clear` 是**特异性反转**：深色规则 `.topbar .proxy-topbar-tools .search-clear{color:#9ca7ba}`（[styles.css:46](web/src/styles.css#L46)，特异性 0,3,0）压过了浅色覆盖 `html[data-theme="light"] .search-clear{color:#667085}`（[:92](web/src/styles.css#L92)，0,2,1），于是浅色主题下"代理节点"页的清空按钮是白底 2.43:1。`.topbar .rule-search .search-clear`（[:713](web/src/styles.css#L713)）同型。
- 建议：浅色把 `.node-type` 提到 `#667085`（4.9:1）、日志 debug 提到 `#6b7686`、`--warn` 由 `#b76b10` 降到 `#8f5309`（≈5.4:1）、`--accent-ink` 由 `#0167ff` 降到 `#0057d8`（≈5.4:1）；`.search-clear` 的浅色选择器补齐层级。

### P1-5 设计 token 漂移：568 处裸色值 + 4 组死规则 + 2 个未定义 token

- **色相漂移**：强调色已从 `#7c9cff` 迁到 `#5b98ff`，但 `styles.css` 仍有 **55 处** `rgba(124,156,255,·)`、3 处 `rgba(143,117,255,·)`（悬停底、选中底、focus 环、图标底），Vue 里另有 4 处，例如 [LogsPage.vue](web/src/pages/LogsPage.vue) 的 `.log-source-switch button.active{color:var(--accent);background:rgba(124,156,255,.14)}` —— 文字是新蓝、底色是旧蓝。Vue 的 scoped 块整体很干净（仅 11 处裸色），**债务集中在两个 CSS 文件**。
- **死规则**（同选择器被 `ui-polish.css` 同权重后置覆盖，改 `styles.css` 不生效）：`#2dbd74` 绿色勾选开关（[styles.css:104](web/src/styles.css#L104)、[:532](web/src/styles.css#L532) → 实际为蓝色）、浅色侧栏 `#F3F3F3`（[:75](web/src/styles.css#L75) → `#fff`）、下载色 `#4b9cff` 4 处（[:844](web/src/styles.css#L844) 等）、`.theme-switch/.theme-choice` 约 **11 条**（见 P2-9）。
- **两套浅色 token**：[styles.css:73](web/src/styles.css#L73) 与 [ui-polish.css:21-44](web/src/ui-polish.css#L21-L44) 各定义一套且取值不同（`--bg #f5f7fb` vs `#f7f9fc`、`--muted #667085` vs `#647286`、`--accent #627fe6` vs `#0167ff`、`--good #239a58` vs `#16834b`）。后者后加载故生效 —— **除 `--warn` 外的颜色 token 全部失效**（这正是 P1-4 里 `--warn` 仍是 3.89:1 的原因）；深色侧 `--accent`/`--shadow`/`--radius` 同样被 [ui-polish.css:2-19](web/src/ui-polish.css#L2-L19) 覆盖（`--radius` 16→14、`--shadow` 18px/50px→2px/8px）。
- **未定义 token**：[SettingsPage.vue](web/src/pages/SettingsPage.vue) 的 `.core-update-status.error{color:var(--danger,#dc4663)}` / `.done{color:var(--success,#21965a)}` 使用了不存在的 `--danger`/`--success`，因此这两处状态色**永远不随主题变化**，也与 `--bad`/`--good` 不一致；`--font-mono` 3 处靠回退，而等宽栈在别处硬写了 5 次。
- 建议（纯机械替换，`npm --prefix web run check` 可兜底）：① 合并为唯一一份主题块；② 55+4 处旧强调色替换为 `color-mix(in srgb, var(--accent) N%, transparent)`；③ 裸色映射 `#0d1016`→`var(--control-bg)`、`#0e1118/#0f1219/#10131a`→`var(--surface-inset)`；④ 删除死规则与 4 处 `#4b9cff`；⑤ 补 `--font-mono`，`--danger/--success` 改用 `--bad/--good`。

### P1-6 101 个 `<label>` 中 25 个没有关联控件

- 实测：带 `for`/`:for` 关联的 16 个、包裹控件的 60 个、**两者都没有 25 个**。示例：[TunSettingsModal.vue:156](web/src/components/TunSettingsModal.vue#L156)（MTU）、[SettingsPage.vue:598](web/src/pages/SettingsPage.vue#L598)（DNS 监听地址 / 增强模式 / Fake IP IPv4 范围，一行内 5 个字段）、[SettingsPage.vue:577](web/src/pages/SettingsPage.vue#L577)、[ProfilesPage.vue:259](web/src/pages/ProfilesPage.vue#L259)、[SystemProxySettingsModal.vue:161](web/src/components/SystemProxySettingsModal.vue#L161)。
- 影响：这些输入框没有可访问名称，读屏只念"编辑框"；点击标签也不会聚焦输入框。
- 建议：统一用"标签包裹控件"的写法（[LogsPage.vue:153](web/src/pages/LogsPage.vue#L153) 已是正确范式），或补 `id`/`for`。

### P1-7 开关与遮罩的"多套实现"

- 开关：`.switch`（42×24）、`.switch.large`（48×27）、日志里 `transform:scale(.86)`（≈36×21，[:663](web/src/styles.css#L663)）、规则启停 `.rule-toggle`（34×20 的 `aria-checked` 按钮，[:1013](web/src/styles.css#L1013)）—— 同一语义 **4 种尺寸 2 套实现**。
- 遮罩：`.modal` 已按主题给色（[ui-polish.css:222-223](web/src/ui-polish.css#L222-L223)），但两个原生 `<dialog>` 硬编码 `rgba(0,0,0,.45)`（[SequenceFormDialog.vue:46](web/src/components/SequenceFormDialog.vue#L46)）与 `rgba(0,0,0,.6)`（[GroupMembersModal.vue:49](web/src/components/GroupMembersModal.vue#L49)）且**无浅色变体** → 浅色主题出现 45–60% 纯黑遮罩，与 BaseModal 的 20% 深蓝遮罩明显不一致。
- 建议：抽 `--scrim` 供 `.modal` 与两处 `::backdrop` 共用；开关统一为 `.switch` + 尺寸变量，`.rule-toggle` 复用 `.switch`。

### P1-8 错误提示 3.2 秒自动消失且不可手动关闭

- [toast.ts:20-22](web/src/services/toast.ts#L20-L22)：非配置类消息 3200ms 后移除，无 hover/focus 暂停；[ToastHost.vue:17](web/src/components/ToastHost.vue#L17)：关闭按钮仅在 `configError` 时渲染；[ToastHost.vue:9](web/src/components/ToastHost.vue#L9)：`#toast` 只有 `aria-live="polite"`，无 `role`，全应用 **0 个 assertive 区域** —— 失败提示排在读屏队列末尾，然后消失。
- 另注意：原生 `<dialog>` 位于 top-layer，`z-index:80` 的 toast 会被弹窗遮挡，`TunSettingsModal` / `SystemProxySettingsModal` 的保存失败提示可能看不见（弹窗内已有 `.local-warning` 模式可用）。
- 建议：错误类不自动消失（或 ≥8s）+ 统一关闭按钮；错误用 `role="alert"`，`#toast` 加 `role="status"`；弹窗内失败改为行内提示。

---

## 二、P2：体验与一致性

| # | 问题 | 证据与建议 |
| --- | --- | --- |
| P2-1 | **字号体系缺失** | 367 处声明 / 14 种字号；`<12px` 占 51%（11px 111 处、10px 63 处、9px 14 处）。建议收敛为 `--text-xs:12 / sm:13 / md:14 / lg:16 / xl:20`，把 9–10px 元信息抬到 11–12px。这是"看起来更专业"最直接的杠杆 |
| P2-2 | **圆角/间距无阶梯** | 51 种 `border-radius`（5/6/7/8/9/10/11/12 并存），`--radius` 只服务 `.card`。建议 `--radius-xs:6 / sm:8 / md:10 / lg:14 / full:999`，间距取 4 的倍数 |
| P2-3 | **顶栏工具过密** | 日志页 46px 顶栏内 8 个控件（搜索 / 800 行 / info / 历史查询 / 日志设置 / 清空历史日志 / 停止 / 刷新），≤1100px 依赖 3px 高的横向滚动条（[ui-polish.css:110-112](web/src/ui-polish.css#L110-L112)）。建议把二级操作收进"更多"菜单，或下沉到 `.log-summary` 工具条 |
| P2-4 | **连接页是 8 页中的异类** | 全局 `table{position:sticky}` 表头（[:17](web/src/styles.css#L17)）在 [:606](web/src/styles.css#L606) 的 `overflow-y:hidden` 容器里**不会粘住**；`min-width:1120px`（[:595](web/src/styles.css#L595)）在 360px 视口需横向平移约 3 屏，规则页 `rule-spacer{min-width:720px}`（[:720](web/src/styles.css#L720)）同样没有 ≤760px 列定义。建议连接页给独立高度 + 自身纵向滚动（与日志/规则页一致），≤760px 改卡片式行布局 |
| P2-5 | **移动端导航与状态** | ≤1000px 隐藏 `.nav-label`（[:27](web/src/styles.css#L27)、[:880](web/src/styles.css#L880)）且 ≤680px 从未恢复（[:28](web/src/styles.css#L28)、[:354](web/src/styles.css#L354)）：手机上 8 个目的地全是纯图标，而 `.nav-icon` 是 `aria-hidden`，可访问名只剩 `title`（触屏不可用）。"有新版本"提示在 ≤1000px 缩成圆点、≤680px 随 `.sidebar-footer` 隐藏（[:882](web/src/styles.css#L882)、[:28](web/src/styles.css#L28)）。桌面 `.sidebar{height:100%}` 无 `overflow:auto`（[:267](web/src/styles.css#L267)），窗口高 <420px 时最后两项导航被裁且无法滚动 |
| P2-6 | **加载/空状态两套语言** | [AsyncState.vue:8-11](web/src/components/AsyncState.vue#L8-L11) 用静态 `.empty` 文本，各页另有带脉冲点的 `.async-placeholder`（[:363-366](web/src/styles.css#L363-L366)）；`AsyncState` 无 `role="status"`/`aria-busy`，加载完成与失败对读屏静默；空状态只有结论、没有下一步入口 |
| P2-7 | **页面切换无上下文更新** | `document.title` 只在启动时设一次（[runtime.ts:16](web/src/services/runtime.ts#L16)），切页不改标题、不移动焦点、不播报（[App.vue:50](web/src/App.vue#L50) 只做动画）。建议 `<h1>` 加 `tabindex="-1"` 并在切页时聚焦，标题改为「页面名 · 应用名」。另：导航用 `history.replaceState`（[App.vue:86](web/src/App.vue#L86)）并主动屏蔽前进/后退，**无法用返回键在页面间后退**（刷新可保持当前页）——嵌入式窗口可接受，但应在文档中写明 |
| P2-8 | **`AppSelect` 的 `aria-label` 全部丢失** | 模板根是 `button + <Teleport>` 片段（编译产物 `_Fragment`），`$attrs` 不落到 trigger；标签被绑到 listbox（[AppSelect.vue:103](web/src/components/AppSelect.vue#L103)），而 ≥8 处调用方传入的标签（`LanguageSelect`、`RuleImportModal`、`ProfileSequenceEditor`、`TunStackFields`、`ProxyBindAddress`）实际被丢弃，并有非 props 属性告警。建议加显式 `label` prop 应用到按钮，`aria-activedescendant` 交给持有焦点的 listbox（或 `aria-owns`） |
| P2-9 | **没有应用内主题切换，但留着约 11 条死 CSS** | `useFnosTheme.ts` 恒写 `themeMode='system'`，跟随 `prefers-color-scheme` 与 fnOS 宿主事件；全仓库 `theme-switch`/`theme-choice` **零引用**，而 [styles.css:70-71](web/src/styles.css#L70-L71)、[:79](web/src/styles.css#L79)、[:98-99](web/src/styles.css#L98-L99)、[:172-174](web/src/styles.css#L172-L174) 仍在提供完整样式。宿主偏好深色的用户无法切浅色，反之亦然。建议：接到"设置 → 行为"分类，或删除死 CSS |
| P2-10 | **断点体系膨胀** | 20 个断点（480/520/600/620/640/680/**681**/700/720/760/820/850/860/900/**901**/980/1000/1040/1100/1200）、66 个媒体块；最初那套 ≤680 规则（[:27-28](web/src/styles.css#L27-L28) 的 `#app{display:block}`、`.topbar{top:49px}`、`.sidebar{position:sticky}`）被 [:257](web/src/styles.css#L257)、[:354-357](web/src/styles.css#L354-L357) 的面板式重写覆盖成死代码。建议收敛到 4 个（1200/1000/760/480） |
| P2-11 | **标题层级有缺口** | `ProxiesPage.vue:235` 用 `h3` 表示代理组但中间没有 `h2`；`ConfigPage`、`LogsPage`、`RulesPage` **完全没有标题元素**，子面板是无标题区域 |
| P2-12 | **登录页无法切换语言** | 语言选择器只在设置页渲染（[App.vue:188](web/src/App.vue#L188)），而 `i18n.ts` 按 `navigator.language` 自动判定 —— 浏览器为英文环境的中文用户会在登录页看到英文且无法切换 |
| P2-13 | **4 处规则删掉了全局焦点环** | [:21](web/src/styles.css#L21)、[:107](web/src/styles.css#L107)、[:757](web/src/styles.css#L757)、[:758](web/src/styles.css#L758) 的 `:focus-visible{…outline:none}` 覆盖了 [ui-polish.css:141](web/src/ui-polish.css#L141) 的全局 2px 描边，其中两处只剩 10% 透明度的底色变化。建议改为 `outline:2px solid var(--accent);outline-offset:2px` |

---

## 三、P3：小修与整洁度

1. **z-index 无层级表**：20 / 30 / 35 / 40 / 60 / 80 / 1100 / 2000 散落各处。建议集中为 `--z-sticky:20 / --z-dropdown:40 / --z-modal:60 / --z-popover:1100 / --z-toast:1200`。
2. **虚拟列表行高双份维护**：[RuleVirtualList.vue:10](web/src/components/RuleVirtualList.vue#L10) 的 `52` 对应 [styles.css:722](web/src/styles.css#L722)，`ProfileSequenceEditor.vue:80` 的 `26` 对应 [styles.css:1070](web/src/styles.css#L1070)，改 CSS 会静默错位；`RuleVirtualList.vue:55` 的 `aria-rowcount` 少算了表头（`items.length` 应为 `items.length + 1`）。
3. **`100vh` 与 `100dvh` 混用**：[:22](web/src/styles.css#L22)/[:26](web/src/styles.css#L26) 用 `vh`，新弹窗用 `dvh`。
4. **图标来源重复**：[NavIcon.vue:6-14](web/src/components/NavIcon.vue#L6-L14) 手抄 lucide 路径，而 `lucide-vue-next` 已是依赖。
5. **`index.html` 不在 i18n 检查范围**：[check-i18n.mjs](web/scripts/check-i18n.mjs) 只遍历 `src/`，而 [index.html:41](web/index.html#L41) 的首屏 `aria-label="正在加载"` 是硬编码中文（英文用户启动时读屏是中文）；[:2](web/index.html#L2) 的 `lang="zh-CN"` 也要等 JS 启动后才被修正。
6. **i18n 结构**：`zh-CN.json` 是 1816 行 `"X": "X"` 自映射，仅作键清单，可由 `en-US.json` 派生；`t()` 依赖 `{arg0}` 模板匹配与中文前缀推断，新增文案若落在 `explicitOnly` 名单外容易静默不翻译。
7. **体积与拆包**：8 个页面在 [App.vue:11-18](web/src/App.vue#L11-L18) 静态引入，主包 gzip 236KB（CSS 34KB）；改 `defineAsyncComponent` 按页懒加载即可，YAML 编辑器已单独出 chunk。
8. **残留细节**：`table{min-width:800px}` 已无实际作用；`.log-storage-error{margin:6px 28px}`（[LogsPage.vue:179](web/src/pages/LogsPage.vue#L179)）在 ≤680px 仍留 28px 边距而 `.content` 已降到 16px；`styles.css` 33 处 `!important` 主要用于对抗通用 `input`/`label` 规则，token 化后可清理。

---

## 四、建议的设计基线（可直接落地）

```css
/* tokens.css —— 唯一一份主题定义 */
:root {
  /* 表面 */
  --bg:#1f2021; --panel:#232425; --panel2:#292a2c; --line:#383a3d;
  --surface-inset:#1f2021; --surface-raised:#272829; --control-bg:#202123; --surface-disabled:#222325;
  /* 文本（深色） */
  --text:#eef2f8; --muted:#9e9fa0;
  /* 语义 */
  --accent:#5b98ff; --accent-ink:#9bbfff; --accent-soft:#1b2d4c; --accent-line:#355685;
  --good:#43d17b; --warn:#ffbf5b; --bad:#ff667a;
  --switch-track:#5c636f; --switch-track-border:#6d7380;
  --scrim:rgb(24 35 51 / 38%);
  /* 尺度 */
  --text-xs:12px; --text-sm:13px; --text-md:14px; --text-lg:16px; --text-xl:20px;
  --radius-xs:6px; --radius-sm:8px; --radius-md:10px; --radius-lg:14px;
  --z-sticky:20; --z-dropdown:40; --z-modal:60; --z-popover:1100; --z-toast:1200;
  --font-mono:"SFMono-Regular",Consolas,monospace;
}
html[data-theme="light"] {
  --bg:#f7f9fc; --panel:#fff; --panel2:#f7f9fc; --line:#e4e9f0;
  --text:#27313e; --muted:#647286;
  --accent:#0167ff; --accent-ink:#0057d8; --accent-soft:#f0f5ff; --accent-line:#d4e2ff;
  --good:#16834b; --warn:#8f5309; --bad:#bd3c52;
  --switch-track:#cbd2dd; --switch-track-border:#8b96a8;
  --scrim:rgb(31 49 78 / 20%);
}
```

回归对照表（验收时按此核对）：

| 用途 | 目标 | 当前最差 |
| --- | --- | --- |
| 正文 / 小字（<18px） | ≥ 4.5:1 | **1.49:1**（浅色 `.quota-line strong`） |
| 大字号、图标、状态色 | ≥ 3:1 | 1.29:1（浅色手风琴箭头展开态） |
| 控件边界（开关/输入/选中态） | ≥ 3:1 | **1.01:1**（深色开关轨道） |

---

## 五、分阶段落地计划

**阶段一（1–2 天，低风险、收益最大）**
1. 新建 `tokens.css`，合并重复主题块、删除死规则与死 CSS（P1-5、P2-9、P2-10 的死块）。
2. 回填浅色主题 18 处文字色（P1-1）+ 其余对比度与 `.search-clear` 特异性（P1-4）+ 开关 token（P1-2）。
3. `BaseModal` 迁到原生 `<dialog>` 或抽 `useModalDialog()`，删除 5 份手写陷阱；统一 `--scrim`（P1-3、P1-7）。
4. `label` 关联 25 处（P1-6）、Toast 时长/关闭/role（P1-8）。
5. 验收：`npm --prefix web run check`（30 文件 / 187 用例 + typecheck + i18n + build）；浏览器核对明暗两主题 × 键盘 Tab/Esc × `prefers-reduced-motion`。

**阶段二（3–5 天，一致性与可用性）**
1. 字号/圆角/间距阶梯替换（P2-1、P2-2），9–10px 全部抬到 ≥11px。
2. 开关合并（P1-7 收尾）、焦点环统一（P2-13）、标题层级补齐（P2-11）。
3. 顶栏溢出策略（P2-3）；`AsyncState` live 语义与空状态入口（P2-6）；页面切换更新标题与焦点（P2-7）。
4. `AppSelect` 的 `label` prop（P2-8）；`HelpPopover` 点击外部关闭 + TUN 弹窗内帮助面板 Teleport（见附录 A-9）。

**阶段三（按需）**
1. 连接页表格重做、规则页列收敛（P2-4）。
2. 移动端导航标签与更新徽标位置、桌面侧栏可滚动（P2-5）；登录页语言切换（P2-12）。
3. 按页懒加载与 CSS 瘦身（P3-7）；`zh-CN.json` 结构简化（P3-6）；`index.html` 纳入 i18n 检查（P3-5）。

---

## 六、已经做得好的部分（重构时不要丢）

- **外壳与信息架构**：8 页统一头部（标题 + 元信息 + 页面动作 + 刷新），`#page-title-meta` / `#page-actions` 的 Teleport 插槽设计干净；`aside`/`nav`/`main`/`header`/`section` 地标完整，每页一个 `h1`，`aria-current="page"`，刷新按钮带 `aria-busy`。
- **主题能力**：首屏内联样式避免白屏闪烁，`data-theme` + `color-scheme` 正确，fnOS 宿主主题事件已接入；浅色覆盖层覆盖了侧栏、顶栏、卡片、弹窗、表格、日志、输入、开关、节点/订阅行。
- **无障碍投入**：全应用 **282 个 `aria-*`**；`AppSelect` 是完整的 combobox/listbox/option 键盘模型（方向键/Home/End/700ms 类型预选/Esc/Tab、跳过禁用项、滚动与 resize 关闭、弹窗内 portal 重挂载、清理彻底）；`TrafficChart` 的 `role="img"` + 键盘读数 + 主题/语言重绘；`RuleVirtualList` 的表格语义；`ChangelogModal` 的陷阱 + 焦点归还；图标按钮基本都有 `aria-label`。
- **动效克制**：`CollapseTransition` 用 Web Animations 做高度过渡并处理打断与 reduced-motion；`ui-polish.css:253-255` 提供全局兜底。
- **i18n 工程化**：以中文原文为 key、`en-US.json` 强制键对齐 + 参数形状 + 未翻译中文检测，`<style>` 内零中文，`document.documentElement.lang` 与语言同步。
- **回归保障**：187 个单测 + `design-qa.md` 的截图对比流程，使上述重构都可验证。

---

## 附录 A：本次额外核验的问题清单（组件层）

1. **`HelpPopover` 点击外部不关闭**：只监听 `mouseenter/mouseleave/focusout/Esc`（[HelpPopover.vue:19-25](web/src/components/HelpPopover.vue#L19-L25)），触屏点开后必须再点一次；面板 `role="dialog"` 但从不获得焦点，也没有 `aria-describedby`。
2. **`TunSettingsModal` 内帮助面板被裁切**：帮助组件放在 `overflow:auto` 的 `.tun-settings-modal-scroll`（[styles.css:806](web/src/styles.css#L806)）内，而面板是 `position:absolute; width:min(360px,…)`（[:107](web/src/styles.css#L107)），MTU / 排除网段两处会被切掉并撑出滚动条。
3. **`AsyncState` 无 live 语义**（已在 P2-6）。
4. **`ToastHost` 图标色硬编码**：`#209b57`/`#e3475c` 而非 `var(--good)`/`var(--bad)`。
5. **5 份互相不一致的焦点陷阱**：候选选择器漏掉 `a[href]`（[SequenceFormDialog.vue:17](web/src/components/SequenceFormDialog.vue#L17)、[RuleImportModal.vue:118](web/src/components/RuleImportModal.vue#L118)），以后加链接会静默破坏循环。
6. **`RuleVirtualList` `aria-rowcount` 少算表头**（见 P3-2）。
7. **`!important` 33 处**（`styles.css`）：多为对抗通用 `input`/`label` 规则（如 `.switch input{width/height:1px!important}`），token 化后可清理。

*本文只做评审，不修改代码；未做版本号、CHANGELOG 或打包相关变更。*

---

# 实施记录（UI v2 已落地）

本节记录评审结论的落地情况，代码为工作区改动（**未提交、未打版本、未打包**）。截图证据见 `.cache/ui-redesign/shipped/index.html`（优化前 / 已实现对比）。

## 已实施

**设计 token 层（P1-4 / P1-5 / P2-1 / P2-2）**
- 新增 `web/src/tokens.css`：唯一一份主题定义（表面、文本、语义色、强调色族、交通色、开关、遮罩、字号、圆角、阴影、层级、动效、布局），并追加 `--text-3`/`--line-strong`/`--switch-track`/`--switch-track-border`/`--scrim`/`--font-mono`/`--rule-row-height` 等此前缺失或硬编码的语义。
- 删除 `styles.css` 与 `ui-polish.css` 中两份重复主题块（原先浅色 token 被定义两次、深色 `--accent`/`--shadow`/`--radius` 被二次覆盖；`styles.css:73` 除 `--warn` 外全部失效）。
- 132 处旧色族（`rgba(124,156,255,·)` 55 处、`rgba(143,117,255,·)`、旧 `--good`/`--warn`/`--bad` 的 rgba 写法）改为 `color-mix(in srgb, var(--token) N%, transparent)`；约 85 处深色字面量（`#0d1016`、`#0e1118`、`#a9bbff`、`#aeb6c7`、`#cbd4e4` 等）映射到 token。
- 删除死规则：绿色勾选开关、浅色侧栏 `#F3F3F3`、`#4b9cff` 4 处、内置主题选择器约 15 条、`table{min-width:800px}`。
- 字号下限：14 处 9px + 60 处 10px 统一为 `var(--text-xs)`（11px）；控件/正文使用 `--text-sm`/`--text-base`。
- 开关统一为 40×22（`.switch`、`.switch.large`、`.quick-switch`、日志缩放 hack、`.rule-toggle`），遮罩统一 `--scrim`。

**对比度（P1-1 / P1-2 / P1-4）**
- 浅色主题回填此前无覆盖的深色文字：`.quota-line strong` 由 1.49:1 提升到 15.7:1，`.quota-line`/`.quota-expire`/`.quick-label`/`.proxy-chevron`/手风琴箭头/`.node-type`/`.log-debug`/`--warn`/`--accent-ink` 全部 ≥4.5:1（数值见下表）。
- 深色开关轨道由 1.01:1 → 1.9:1、描边 1.36:1 → 3.9:1。
- 修复 `.search-clear` 的特异性反转（浅色下 2.43:1）。

**组件与无障碍（P1-3 / P1-6 / P1-8 / P2-6 / P2-7 / P2-8 / P2-9 / P2-13）**
- `BaseModal`：新增初始焦点、Tab 循环、Esc 关闭（尊重 `closable`）、关闭后焦点归还；13 个弹窗统一受益（背景不设 `inert`，以保持提示条可点）。
- 表单标签：23 处孤立 `<label>` 通过 `for`/`id` 或包裹控件完成关联（SettingsPage / TunSettingsModal / SystemProxySettingsModal / ProfilesPage），`TunStackFields` 用 `useId()` 生成唯一 id。
- `AppSelect`：新增 `label` prop 与显式 `v-bind` 转发（修复片段根节点导致的 `aria-label`/`id`/`class` 丢失）。
- `HelpPopover`：面板 Teleport 到 body + 视口内定位（修复 TUN 弹窗内被滚动容器裁切），新增点击外部关闭。
- `AsyncState`：`role="status"`/`role="alert"`；Toast：错误 8s、成功 3.2s、所有提示可手动关闭、`role="alert"`/`role="status"`、图标色改用 token。
- 焦点环：4 处 `outline:none` 改为统一 2px 强调色描边；标题层级/页面标题：`document.title` 变为「页面名 · 应用名」，切页聚焦 `<h1>`。

**页面结构（P2-3 / P2-4 / P2-5）**
- 日志页顶栏由 8 个控件降为 3 个 + 刷新，二级操作下沉到页内 `.log-secondary-actions` 工具条。
- 连接页表格改为独立滚动容器 + 真正生效的粘性表头，字节列右对齐等宽数字。
- 移动端恢复导航文字标签（横向滚动、不再压缩重叠），保留状态/更新提示，顶栏自动换行。

**其他**：`RuleVirtualList` 的 `aria-rowcount` 修正为 `items.length + 1`；行高改由 `--rule-row-height`/`--rule-table-row-height` 单一来源；弹窗尺寸 `100vh` → `100dvh`；`--danger`/`--success` 未定义 token 改为 `--bad`/`--good`。

## 验证

- `npm --prefix web run check`：i18n 检查 + `vue-tsc` + **34 文件 / 223 用例** + 生产构建，全部通过。
- `cd backend && go test -race ./... && go vet ./...`：全部通过（`failed to trim cache` 为沙箱下 Go 构建缓存告警，非测试失败）。
- 真实浏览器（CDP）行为验证：弹窗打开后焦点进入对话框、Esc 关闭、焦点回到触发按钮；帮助浮层在 TUN 弹窗内逃出滚动容器、位于视口内、点击外部关闭。
- 视觉回归：以真实构建重拍 8 页 + 深色 + 移动端，与评审阶段的设计稿逐页比对，见 `.cache/ui-redesign/shipped/index.html`。

## 有意未做（后续）

1. 按页懒加载（主包 gzip 245KB）与 CSS 瘦身（gzip 37.6KB，本次新增外观层略高于删除量）。
2. 连接页/规则页的移动端卡片式降级；空状态补行动入口。
3. `zh-CN.json` 结构简化（1816 行自映射）、`index.html` 启动期中文 `aria-label` 纳入 i18n 检查。
4. `AppSelect` 升级到 ARIA 1.2 combobox（把 `aria-activedescendant` 交给持有焦点的 listbox）。
5. 应用内明暗主题切换（当前仍由宿主 `prefers-color-scheme` 决定）。
6. 删除 `ChangelogModal`/`RuleImportModal`/`ConfigEditor` 中与 `BaseModal` 重复的焦点处理（现为双实现并存，行为正确）。

未提交、未改 `VERSION`/`CHANGELOG.md`、未执行 `scripts/sync-version.sh`、未打包 FPK/Docker/DEB —— 按 AGENTS.md，版本与变更日志在打包前随发布一起更新。
