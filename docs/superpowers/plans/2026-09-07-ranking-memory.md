---
status: planned
planner_model: GPT-6 Astra
executor_model: GPT-5.6 Terra (high); GPT-5.6 Luna (medium) for UI after contract freeze
updated: 2026-09-07
---

# 成交额榜单变化记录：实现任务与模型交接

## Goal

按 `docs/superpowers/specs/2026-09-07-ranking-memory-design.md` 实现可信收盘榜单历史与比较。规划任务只写文档；本计划未执行，也未创建或派发编码任务。

## Acceptance criteria

- 用户可切换股票／ETF 的历史收盘日期，不触发公开行情请求。
- 只有完整、同口径、确认收盘的记录参与上一交易日排名、新进榜及连续在榜计算。
- 同日多次刷新只计一天；修正版本重算；失败保留缓存。
- 节假日跨越正确；缺交易日、旧未验证、CSV、日历未知、口径变化均按设计降级。
- 实时页不显示收盘价文案、不计算历史倍数或排名变化。
- 首屏以精简榜单为主，概览可折叠，走势按需展开；ETF 仅展示核验过的标签。
- 加入观察后显示已观察，理由记住当时的模式、日期、排名与可确认的变化。
- 原有交易计划、成交补录、纪律、价格提醒等不被改动；原有数据无损保留。

## Scope and constraints

当前工作区存在大量用户未提交修改，执行前必须阅读 git status/diff，不能 reset、覆盖或替用户提交这些改动。不要在当前规划任务编码，也不要同时启动多个修改同一文件的执行者。

新增文件优先承载比较模块；已有文件只接入。遵循适用的 AGENTS.md 与 Vue 技能。最终生产构建前按仓库约定验证；不部署、不发布、不生成安装包。测试只用临时数据库和固定 provider，不能打开用户真实数据库试迁移。

P0收盘校验→历史契约→比较接口→UI→验收，不能先画出有数值的比较列再补数据规则。ETF 标签可单独交付但不能阻塞核心历史功能。

## Repository context

- Vue/Electron + Go/SQLite；脚本见根 `package.json`，当前 `store.go` schema=12，迁移时先重读实际版本。
- `backend/internal/market/provider.go`：Quote、RankingKind、SnapshotMode 与 provider 接口。
- `backend/internal/market/eastmoney.go`、`sina.go`：抓取、过滤、排序、截断；`etf_filter.go` 为现有排除规则。
- `backend/internal/service/market.go`：RefreshMarket、RefreshLiveMarket、LatestMarket、CSV入口；新逻辑建议 `ranking_history.go`。
- `backend/internal/store/market.go`：快照及条目保存、最新查询；新查询建议 `ranking_history.go`。
- `backend/internal/store/migrations.go`、`store.go`：迁移；快照 version 的 UNIQUE(trade_date,ranking_kind,version) 不包含 mode，必须保持现有递增约束。
- `backend/internal/api/router.go`、`router_test.go`：路由及集成测试；`src/renderer/types.ts`：前端契约。
- `src/renderer/composables/useMarketHistory.ts`、对应 spec：加载与选择；新增 `useRankingHistory.ts` 独立管理日期及比较请求。
- `src/renderer/views/MarketView.vue`、`MarketView.spec.ts`；`components/market/MarketTable.vue`、`MarketPanels.spec.ts`：主要界面。
- `backend/internal/service/watchlist.go`、`store/watchlist.go` 已提供列表与重复防护，本版优先复用，不改表。
- `backend/internal/store/market_history_test.go`、`service/market_test.go`、`market/market_test.go`、`sina_test.go`：现有验证入口。

## Implementation tasks

### T0 — Terra / high：建立安全基线与收盘资格

- [ ] 阅读设计、当前diff和迁移实现，记录实际基线及已有失败，不修无关问题。
- [ ] 为新快照增加元信息：`comparison_quality`（verified_close/legacy_unverified/manual_unverified/incomplete）、`universe_version`、`quality_reason`。旧行默认 legacy_unverified；保留全部旧行。建议独立 `market_snapshot_quality` 表，以 snapshot_id 为主键，缺元信息按 legacy_unverified；避免改写旧观察事实。
- [ ] 新增 `backend/internal/market/ranking_quality.go`，校验设计规定的时间、完整性、重复、数值、排序、范围与口径。必要时增加 provider 返回的榜单元信息，不能将“返回20行”等同全市场完整。
- [ ] 主源返回成功但数据资格不合格时尝试备用源；两者不合格不存为 verified_close，不更新最后合格数据。保留失败原因与旧缓存。live 模式继续独立。
- [ ] CSV新记录标记 manual_unverified，旧 CSV 不做自动认证。元信息与快照入库同事务。

产物：可以向服务询问快照是否可比及原因；不会再把上午榜单存成已验证收盘。验证：上午、15:10后但源时间上午、未来时间、混日期、19行、重复代码、正常20行、主源不合格备用成功、双失败缓存保护。

### T1 — Terra / high：日历与历史存储查询（依赖 T0）

- [ ] 新建 `market/trading_calendar.go` 及固定数据文件，提供 PrevTradingDay / IsTradingDay / 覆盖范围。核验并记录交易所官方2026年安排来源；超范围 unknown。不要复用仅工作日判断的 IsAShareTradingSession 来推算昨日。
- [ ] 实现 `ListRankingDates(kind)`、`RankingSnapshotAt(kind,date)` 和批量读取连续历史。提供日列表全部旧记录，但同日优先最新合格版本，无合格则展示最新未验证记录。历史榜单各日只展示一个版本，旧版本仍保存。
- [ ] 给查询增加适当复合索引；使用批量读取，避免每行每一天都查询一次。现有 Store 仅1条连接，必须关闭 rows 后再发起后续查询，避免死锁。
- [ ] 日期、kind严格校验；日期不存在返回404，不默默返回最新日期。测试迁移可重复执行、旧数据仍在、close/live不串、修正版本选对。

产物：历史读取不联网且有明确质量状态。核验日历失败时保留 unknown 降级并记录阻碍，不能以周一至周五替代权威日历。

### T2 — Terra / high：比较逻辑与接口（依赖 T1）

- [ ] 新增纯函数比较器，用完整前日排名集合计算 delta/new/unchanged/unknown，再按日历向前计算 streak。证券主键 market+code。
- [ ] 新增 GET `/api/market/rankings/dates?kind=stock`：`{kind, dates:[{tradeDate,quality}], latestDate}`。
- [ ] 新增 GET `/api/market/rankings/compare?kind=stock&date=YYYY-MM-DD`；省略date取最新可展示日；无数据返回200、snapshot=null、entries=[]、reason=no_history。
- [ ] 对比响应约定如下，Go和TS同步定义；旧接口兼容，新增接口只读本地。

```typescript
interface RankingComparison {
  snapshot: MarketSnapshot | null
  baselineDate: string | null // 日历确认的上一交易日，缺快照仍保留日期
  baselineSnapshotId: string | null
  quality: string
  universeVersion: string | null
  reason: string | null // no_history / unverified / missing_baseline / calendar_unknown / universe_changed
  entries: Array<{
    quote: MarketQuote
    rank: number
    previousRank: number | null
    rankDelta: number | null
    changeState: 'up' | 'down' | 'unchanged' | 'new' | 'unknown'
    streakDays: number | null
    streakExact: boolean
    streakReason: string | null
    etfLabel: { trackingIndexId: string | null; trackingIndexName: string | null;
      assetCategory: string; sourceURL: string; verifiedAt: string } | null
  }>
}
```

- [ ] snapshot.entries 为原始快照，外层 entries 为展示增强行，二者同序；前端不要按过滤后的数组索引重新计算排名。reason使用稳定枚举并映射用户文案。
- [ ] 当前有效但基准缺失时仍可返回已确认至少1天；当前未验证时全部比较未知。旧日期与修正版本查询均动态重算。

验证：前日8→今日3为↑5；前日完整但缺证券为new且delta=null；前日缺失为unknown；三日可比入榜且更早已离榜为精确3；三日之后遇缺口为≥3；刷新同日不递增；周末及法定假期正确跳过；日历边界、口径变化与修正版本处理正确。

### T3 — Terra / medium：ETF标签映射（依赖 T2 契约）

- [ ] 新增只读 `market/etf_metadata.json` 与加载器，以market/code查找，不修改instruments的交易语义。
- [ ] 用基金管理人官方产品资料核验少量常见ETF；记录sourceURL及verifiedAt；无法核验保留未知，不伪造样本。测试数据可以虚构且必须标明fixture。
- [ ] 映射接入比较响应，未知返回null，前端显示待补充。相同指数使用相同规范ID，类别相同不能据此合并。

验证：同指数不同代码标签一致；未知代码安全降级；更新标签不影响排序、ETF过滤或历史金额。

### T4 — Luna / medium：页面精简与观察便利（依赖 T2，T3可先为空）

- [ ] 实现 `useRankingHistory.ts`，日期查询与比较请求独立于公开刷新；模式／日期／kind快速切换采用请求序号或AbortController避免旧响应覆盖新选择。
- [ ] 更新 `MarketView.vue`，榜单置顶，概览折叠，走势按需展示；保持现有用户市场开关。股票／ETF分别维护可用日期；历史日期不会因刷新被切走。
- [ ] 更新 `MarketTable.vue`，按设计精简列；显式mode。比较数字从API读取，null绝不显示0；↑↓配文字语义，不只靠颜色。日期选择和展开控件可键盘使用；窄窗口表格可滚动。
- [ ] 通过现有 GET /api/watchlist 建立已观察集合；加入时按钮禁用，成功更新集合，失败保留重试。其他窗口已加入导致冲突时重拉列表，匹配后显示已观察。
- [ ] 原有 AddWatchlist 请求结构不变，reason写入实际模式、日期、原始排名和确认的比较文字；不得把未知说成新进榜。用户已存观察理由不改写。
- [ ] 更新受影响组件测试，包括实时空态文案、隐藏比较列、日期缺失、旧快照、ETF未知标签、加入观察、快速切换竞态及历史选择刷新保持。

验证：固定fixture下按验收清单操作；浏览器检查普通桌面及窄窗口。只显示本地开发页面，不触发真实行情刷新或真实观察写入；用测试工作区/fixture服务。

### T5 — Terra / high：整体验收与文档收尾（依赖 T0–T4）

- [ ] 按下方命令验证；对失败区分新增问题与已有基线问题。修复本次范围内问题。
- [ ] 核对收盘刷新写入→按日回看→比较→加入观察完整链路；检查迁移后旧记录可看但不误标可比。
- [ ] 更新README的数据口径、历史积累限制、日历覆盖及实际schema；更新 `docs/验收清单.md`，不宣称关闭应用会自动积累数据。
- [ ] 在本计划Execution log逐项记录测试证据、变更文件和偏差，全部验收满足才将status改为completed。

## Verification

后端定向：`cd backend && go test ./internal/market ./internal/store ./internal/service ./internal/api`。
前端定向：`pnpm exec vitest run src/renderer/views/MarketView.spec.ts src/renderer/components/market/MarketPanels.spec.ts src/renderer/composables/useMarketHistory.spec.ts`，补充新增比较/历史composable spec路径。
整体验证：`pnpm test:go`、`pnpm test`、`pnpm typecheck`、`pnpm build`。构建可能更新resources/bin/discipline-server，执行者须识别原有二进制修改并在交付中说明，不能撤销用户版本。

所有金融数值和历史对比测试使用确定fixture；日历和ETF事实核验仅用官方来源并记录链接。UI验收不等于行情源真实可用性；不能据mock测试声称公开源已联通。

## Risks and decisions

- 没有后台采集，关闭应用或漏刷新会缺历史；这是第一版明确限制，不通过偷偷增加定时任务弥补。
- 主/备行情源过滤后是否足够20条、时间戳语义是否能证明收盘，需要执行者验证适配器；不满足则降级未知，不放宽合同。
- 旧记录质量未知、历史ETF清理可能删过条目；本版不追溯认证。后续可单独做有证据的历史认证工具。
- 2026以外日历需更新；范围未知时仍可回看，禁止给出虚假精确对比。
- Schema与既有未提交修改可能继续变化；以实际代码递增迁移版本，不硬编码使用13。
- 若执行发现必须改变产品口径或超出范围，记录具体证据交还规划者；常规文件移动或测试修复不需重新规划。

## Execution log

- 2026-09-07：完成当前代码审视及方案/任务文档；尚未编码、运行测试或派发执行模型。

## 可直接发送的执行提示

### Terra：核心实现

使用 $model-handoff 的 Execute 模式。先读 docs/superpowers/plans/2026-09-07-ranking-memory.md 和其引用的设计文档，再读仓库指令与当前diff。仅按依赖顺序实现T0、T1、T2、T3，保留现有未提交修改；执行对应验证并更新Execution log。接口稳定后输出契约与测试证据交给Luna实现T4；遇到产品口径变化记录证据交还规划者。模型使用 GPT-5.6 Terra，high。

### Luna：界面实现

使用 $model-handoff 的 Execute 模式。先读 docs/superpowers/plans/2026-09-07-ranking-memory.md、设计文档和当前diff，确认T0–T2已完成且契约存在。仅实现T4，比较逻辑使用后端结果，ETF标签缺省安全降级。保留用户修改，执行前端定向测试与类型检查并记录Execution log。模型使用 GPT-5.6 Luna，medium。

### Terra：最终验收

使用 $model-handoff 的 Execute 模式。阅读同一计划和执行日志，确认T0–T4完成，执行T5并修复本次范围内问题，记录验证结果与已有基线失败。仅在所有验收条件满足时标记completed。模型使用 GPT-5.6 Terra，high。
