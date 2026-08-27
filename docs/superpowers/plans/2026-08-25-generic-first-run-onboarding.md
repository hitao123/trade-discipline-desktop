# 通用首次启动向导与空白账户 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让全新安装的 Plain Rule 先完成通用首次启动向导并得到完全空白的本地工作区，同时让已有数据库无损保留当前个人规则和全部历史数据。

**Architecture:** 在 schema 11 中加入单例用户资料和明确的 `legacy/generic` 模式，迁移开始前识别新库与旧库；新库只留下 `generic + pending`，向导通过一次数据库事务创建账户、通用规则和审计。渲染层由 `App.vue` 统一门禁，并把用户资料注入主界面；规则、资产配置和个性化文案依据模式分流，旧规则 JSON 缺少模式时一律解释为 `legacy`。

**Tech Stack:** Go 1.25、SQLite（modernc.org/sqlite）、`net/http`、Vue 3 Composition API、Vue Router、TypeScript、Vitest、Testing Library、Electron 37、electron-builder。

**Spec:** `docs/superpowers/specs/2026-08-25-generic-first-run-onboarding-design.md`

## Global Constraints

- 新数据库不得写入腾讯、阿里、20 万元、8 万元中概上限、20 日观察、年末 10% 或七项默认资产配置。
- 既有数据库升级后必须保持账户、规则版本、证券、计划、成交、持仓、配置和审计不变。
- 初始化和资料修改必须使用数据库事务；失败时不留下部分数据。
- 通用用户只启用最大损失、同标的浮亏禁加、冷静期、计划、证据、情绪、违规与复盘等共性纪律。
- 安装包不得包含 `discipline.db`、用户截图或导出的个人文件。
- 遵循用户要求，只增加少量覆盖高风险边界的测试，不扩散重复测试文件。
- 当前工作区已有大量未提交改动；只修改本计划列出的相关文件，先检查重叠差异，不覆盖其他功能。
- 本计划执行期间不提交、不合并、不清空真实数据库，也不覆盖现有 DMG；每个任务以差异检查和测试作为检查点。
- 使用现有隔离工作区 `.worktrees/health-quick-record`，不再创建第二个 worktree。

---

## 文件结构与职责

### 后端新增

- `backend/internal/domain/onboarding.go`：用户模式、向导状态、期限、市场范围及输入输出类型。
- `backend/internal/store/onboarding.go`：读取资料、原子完成初始化、原子修改通用资料与规则。
- `backend/internal/service/onboarding.go`：输入校验、通用规则生成、资料更新策略。
- `backend/internal/service/test_helpers_test.go`：显式构造历史个人模式测试夹具，替代“迁移自动播种”的隐式依赖。

### 前端新增

- `src/renderer/composables/useUserProfile.ts`：注入和读取当前用户资料，供 App、设置页、今日页和市场页共用。
- `src/renderer/views/OnboardingView.vue`：三步首次启动向导。
- `src/renderer/views/OnboardingView.spec.ts`：向导最小关键交互测试。

### 主要修改

- `backend/internal/store/store.go`、`migrations.go`：schema 11、新旧库识别、移除无条件个人种子。
- `backend/internal/rules/snapshot.go`、`engine.go`：通用规则模式和动态金额提示。
- `backend/internal/service/executions.go`、`dashboard.go`、`plans.go`：通用模式不执行个人规则，账户初始现金与规则参考资金解耦。
- `backend/internal/domain/allocation.go`、`store/allocation.go`、`service/allocation.go`：未配置状态、任意项目、现金项目角色和旧配置兼容。
- `backend/internal/api/router.go`：向导、资料修改和新的资产配置响应契约。
- `src/renderer/App.vue`、`types.ts`：首次启动门禁与类型。
- `src/renderer/views/TodayView.vue`、`SettingsView.vue`、`MarketView.vue`、`WeeklyReviewView.vue`、`PositionsView.vue`：按模式隐藏个人化信息。
- `src/renderer/views/AllocationView.vue`、`components/allocation/AllocationEditor.vue`：空白开始和动态项目编辑。
- `scripts/smoke-macos.sh`、`README.md`、`docs/验收清单.md`、`package.json`：全新空库冒烟、分发说明、版本和验收步骤。

---

### Task 1: schema 11 与新旧数据库识别

**Files:**
- Create: `backend/internal/domain/onboarding.go`
- Create: `backend/internal/store/onboarding.go`
- Modify: `backend/internal/store/migrations.go`
- Modify: `backend/internal/store/store.go`
- Modify: `backend/internal/store/store_test.go`

**Interfaces:**
- Produces: `domain.UserProfile`, `domain.UserMode`, `domain.OnboardingStatus`, `domain.HoldingHorizon`, `domain.MarketScope`。
- Produces: `Store.UserProfile(ctx) (domain.UserProfile, error)`。
- Produces: schema 11 表 `user_profiles`，固定主键 `local-user`。
- Consumes: 现有 `schema_migrations`、`accounts`、`rule_versions`，但不得改变其历史行。

- [ ] **Step 1: 写新库为空、旧库保留的失败测试**

在 `backend/internal/store/store_test.go` 把原 `TestMigrateSeedsOneAccountAndInitialRule` 替换为以下边界，并保留现有迁移结构测试：

```go
func TestFreshMigrationCreatesPendingGenericProfileWithoutPersonalSeeds(t *testing.T) {
    db := openTestStore(t)
    profile, err := db.UserProfile(context.Background())
    if err != nil { t.Fatal(err) }
    if profile.Mode != domain.UserModeGeneric || profile.OnboardingStatus != domain.OnboardingPending {
        t.Fatalf("profile=%#v", profile)
    }
    for _, table := range []string{"accounts", "rule_versions", "instruments", "allocation_profiles"} {
        var count int
        if err := db.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil { t.Fatal(err) }
        if count != 0 { t.Fatalf("%s count=%d", table, count) }
    }
}

func TestMigrationMarksExistingDatabaseLegacyWithoutChangingFacts(t *testing.T) {
    ctx := context.Background()
    path := filepath.Join(t.TempDir(), "legacy.db")
    db, err := Open(path)
    if err != nil { t.Fatal(err) }
    if _, err := db.DB().Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil { t.Fatal(err) }
    if _, err := db.DB().Exec(`CREATE TABLE accounts(id TEXT PRIMARY KEY, name TEXT NOT NULL, base_currency TEXT NOT NULL, initial_capital_fen INTEGER NOT NULL, enabled_at TEXT NOT NULL, status TEXT NOT NULL)`); err != nil { t.Fatal(err) }
    if _, err := db.DB().Exec(`CREATE TABLE rule_versions(id TEXT PRIMARY KEY, version INTEGER NOT NULL UNIQUE, snapshot_json TEXT NOT NULL, change_reason TEXT NOT NULL, created_at TEXT NOT NULL, previous_id TEXT)`); err != nil { t.Fatal(err) }
    raw, err := json.Marshal(rules.InitialSnapshot())
    if err != nil { t.Fatal(err) }
    if _, err := db.DB().Exec(`INSERT INTO schema_migrations VALUES(10,'2026-08-25T00:00:00Z')`); err != nil { t.Fatal(err) }
    if _, err := db.DB().Exec(`INSERT INTO accounts VALUES('account-main','我的纪律账户','CNY',20000000,'2026-08-12T00:00:00Z','active')`); err != nil { t.Fatal(err) }
    if _, err := db.DB().Exec(`INSERT INTO rule_versions VALUES('rule-1',1,?,'初始交易纪律规则','2026-08-12T00:00:00Z',NULL)`, string(raw)); err != nil { t.Fatal(err) }
    if err := db.Migrate(ctx); err != nil { t.Fatal(err) }
    profile, err := db.UserProfile(ctx)
    if err != nil { t.Fatal(err) }
    if profile.Mode != domain.UserModeLegacy || profile.OnboardingStatus != domain.OnboardingCompleted { t.Fatalf("profile=%#v", profile) }
    var accounts, versions int
    var after string
    if err := db.DB().QueryRow(`SELECT count(*) FROM accounts`).Scan(&accounts); err != nil { t.Fatal(err) }
    if err := db.DB().QueryRow(`SELECT count(*), snapshot_json FROM rule_versions`).Scan(&versions, &after); err != nil { t.Fatal(err) }
    if accounts != 1 || versions != 1 || after != string(raw) { t.Fatalf("accounts=%d versions=%d snapshotChanged=%v", accounts, versions, after != string(raw)) }
}
```

在幂等测试中改为断言 `user_profiles` 始终只有一行、业务种子始终为零。

- [ ] **Step 2: 运行存储测试确认失败**

Run: `cd backend && go test ./internal/store -run 'TestFreshMigration|TestMigrationMarksExisting|TestMigrateIsIdempotent' -count=1`

Expected: FAIL，原因是 `UserProfile` 尚不存在，且当前迁移仍会写入个人种子。

- [ ] **Step 3: 定义资料领域类型**

在 `backend/internal/domain/onboarding.go` 定义稳定常量和 JSON 契约：

```go
package domain

import "time"

type UserMode string
const (
    UserModeLegacy  UserMode = "legacy"
    UserModeGeneric UserMode = "generic"
)

type OnboardingStatus string
const (
    OnboardingPending   OnboardingStatus = "pending"
    OnboardingCompleted OnboardingStatus = "completed"
)

type HoldingHorizon string
const (
    HorizonUnder6M  HoldingHorizon = "under_6m"
    Horizon6To12M   HoldingHorizon = "6_to_12m"
    Horizon1To3Y    HoldingHorizon = "1_to_3y"
    HorizonOver3Y   HoldingHorizon = "over_3y"
    HorizonLegacy   HoldingHorizon = "legacy_unspecified"
)

type MarketScope string
const (
    MarketAShareStock MarketScope = "ashare_stock"
    MarketAShareETF   MarketScope = "ashare_etf"
    MarketHK          MarketScope = "hk"
)

type UserProfile struct {
    ID                   string           `json:"id"`
    Mode                 UserMode         `json:"mode"`
    OnboardingStatus     OnboardingStatus `json:"onboardingStatus"`
    InvestableCapitalFen int64            `json:"investableCapitalFen"`
    MaxLossFen           int64            `json:"maxLossFen"`
    HoldingHorizon       HoldingHorizon   `json:"holdingHorizon"`
    EnabledMarkets       []MarketScope    `json:"enabledMarkets"`
    CompletedAt          *time.Time       `json:"completedAt,omitempty"`
    UpdatedAt            time.Time        `json:"updatedAt"`
}
```

- [ ] **Step 4: 在建表前捕获旧库状态并添加 schema 11**

在 `Store.Migrate` 开始事务后、执行 `migrationStatements` 前调用私有函数：

```go
legacyDatabase, err := hasExistingApplicationState(ctx, tx)
```

`hasExistingApplicationState` 先从 `sqlite_master` 判断 `schema_migrations`、`accounts`、`rule_versions` 或任一事实表是否存在；只有表存在时才查询行数。空 SQLite 文件必须返回 `false`。

在 `migrationStatements` 加入：

```sql
CREATE TABLE IF NOT EXISTS user_profiles (
    id TEXT PRIMARY KEY CHECK(id='local-user'),
    mode TEXT NOT NULL CHECK(mode IN ('legacy','generic')),
    onboarding_status TEXT NOT NULL CHECK(onboarding_status IN ('pending','completed')),
    investable_capital_fen INTEGER,
    max_loss_fen INTEGER,
    holding_horizon TEXT,
    enabled_markets_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(enabled_markets_json)),
    completed_at TEXT,
    updated_at TEXT NOT NULL
)
```

迁移末尾：

- `legacyDatabase=true`：从最新规则和账户安全回填金额，插入 `legacy/completed`；市场范围写入三项，期限写 `legacy_unspecified`。
- `legacyDatabase=false`：插入 `generic/pending`，金额和期限保持 `NULL`，市场写 `[]`。
- 插入 schema 版本 11。
- 删除 `Migrate` 中账户、规则、腾讯和阿里的无条件 `INSERT OR IGNORE`。
- 删除 `store.go` 对 `rules.InitialSnapshot()` 的初始化依赖；不要删除规则包中的历史快照函数。

- [ ] **Step 5: 实现资料读取和兼容解析**

`Store.UserProfile` 读取单例行，使用 `sql.NullInt64`/`sql.NullString` 处理 pending 空值，解析 `enabled_markets_json`，并把数据库损坏包装成可诊断错误。它不创建、修复或覆盖资料。

- [ ] **Step 6: 运行存储迁移测试**

Run: `cd backend && go test ./internal/store -run 'TestFreshMigration|TestMigrationMarksExisting|TestMigrateIsIdempotent|TestMigration' -count=1`

Expected: PASS；schema 最大版本为 11，新库业务表为空，旧库内容不变。

- [ ] **Step 7: 检查差异，不提交**

Run: `git diff --check -- backend/internal/domain/onboarding.go backend/internal/store/onboarding.go backend/internal/store/migrations.go backend/internal/store/store.go backend/internal/store/store_test.go`

Expected: 无输出。保留差异供下一任务使用，不执行 `git commit`。

---

### Task 2: 原子完成向导与显式历史测试夹具

**Files:**
- Create: `backend/internal/service/onboarding.go`
- Create: `backend/internal/service/test_helpers_test.go`
- Modify: `backend/internal/store/onboarding.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/router_test.go`
- Modify: `backend/internal/service/executions_test.go`
- Modify: `backend/internal/store/store_test.go`

**Interfaces:**
- Consumes: `Store.UserProfile(ctx)` 和 Task 1 的领域类型。
- Produces: `service.CompleteOnboardingInput`。
- Produces: `Service.Onboarding(ctx)`、`Service.CompleteOnboarding(ctx, input)`。
- Produces: `Store.CompleteGenericOnboarding(ctx, profile domain.UserProfile, snapshot rules.Snapshot, now time.Time) (domain.UserProfile, error)`。
- Produces: `GET /api/onboarding`、`POST /api/onboarding/complete`。
- Produces: 测试专用 `seedLegacyTestData(t, db)`、`openGenericService(t, capitalFen, maxLossFen)`；现有 `openExecutionService(t)` 调用 legacy helper。

- [ ] **Step 1: 写原子初始化和 API 的失败测试**

在 `backend/internal/store/store_test.go` 新增：

```go
func TestCompleteOnboardingCreatesAccountRuleAndAuditOnce(t *testing.T) {
    db := openTestStore(t)
    ctx := context.Background()
    now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
    pending, err := db.UserProfile(ctx)
    if err != nil { t.Fatal(err) }
    pending.InvestableCapitalFen = 20_000_000
    pending.MaxLossFen = 2_000_000
    pending.HoldingHorizon = domain.Horizon6To12M
    pending.EnabledMarkets = []domain.MarketScope{domain.MarketAShareETF, domain.MarketHK}
    completed, err := db.CompleteGenericOnboarding(ctx, pending, rules.GenericSnapshot(20_000_000, 2_000_000), now)
    if err != nil { t.Fatal(err) }
    if completed.OnboardingStatus != domain.OnboardingCompleted { t.Fatalf("profile=%#v", completed) }
    for table, want := range map[string]int{"accounts": 1, "rule_versions": 1, "audit_events": 1} {
        var got int
        if err := db.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil { t.Fatal(err) }
        if got != want { t.Fatalf("%s=%d want=%d", table, got, want) }
    }
    if _, err := db.CompleteGenericOnboarding(ctx, pending, rules.GenericSnapshot(20_000_000, 2_000_000), now); !errors.Is(err, ErrOnboardingAlreadyCompleted) {
        t.Fatalf("second completion err=%v", err)
    }
}
```

在 `backend/internal/api/router_test.go` 新增一个不播种的服务器帮助函数，并覆盖：

- GET 返回 `generic/pending`；
- 非法金额返回 422；
- 正常 POST 返回 201 和 `generic/completed`；
- 重复 POST 返回 409；
- 随后 GET dashboard 可读取与向导一致的资金。

- [ ] **Step 2: 运行目标测试确认失败**

Run: `cd backend && go test ./internal/store ./internal/api -run 'TestCompleteOnboarding|TestOnboardingAPI' -count=1`

Expected: FAIL，接口与初始化方法尚不存在。

- [ ] **Step 3: 实现服务输入校验与通用快照创建入口**

`backend/internal/service/onboarding.go` 定义：

```go
type CompleteOnboardingInput struct {
    InvestableCapitalFen int64                   `json:"investableCapitalFen"`
    MaxLossFen           int64                   `json:"maxLossFen"`
    HoldingHorizon       domain.HoldingHorizon   `json:"holdingHorizon"`
    EnabledMarkets       []domain.MarketScope    `json:"enabledMarkets"`
}

func (s *Service) Onboarding(ctx context.Context) (domain.UserProfile, error)
func (s *Service) CompleteOnboarding(ctx context.Context, input CompleteOnboardingInput) (domain.UserProfile, error)
```

校验金额、期限和市场集合；去重市场并保持固定顺序。最大损失必须严格小于总资金。错误使用可被 API 映射的 sentinel/type：`ErrOnboardingAlreadyCompleted` 对应 409，其余字段错误对应 422。

- [ ] **Step 4: 实现单事务初始化**

`Store.CompleteGenericOnboarding` 在一个事务内：

1. 用条件查询确认 `generic/pending`；
2. 插入 `accounts`，`initial_capital_fen` 等于向导总资金；
3. 插入第一版通用 `rule_versions`；
4. 插入 `audit_events`，实体 `user_profile/local-user`、动作 `onboarding_completed`；
5. 最后更新 profile 为 completed、写金额/期限/市场/时间；
6. 提交后重新读取 profile。

初始可用现金的事实来源是 `accounts.initial_capital_fen`；不要额外插入 `cash_events`，避免未来重放时重复计算。

- [ ] **Step 5: 注册 API 并给冲突使用稳定状态码**

在 `NewRouter` 注册：

```go
mux.HandleFunc("GET /api/onboarding", router.onboarding)
mux.HandleFunc("POST /api/onboarding/complete", router.completeOnboarding)
```

不要让通用 `writeResult` 把所有错误都变成同一种状态；handler 显式把已完成映射为 `409 ONBOARDING_ALREADY_COMPLETED`，字段问题映射为 `422 INVALID_ONBOARDING`。

- [ ] **Step 6: 把历史测试依赖改成显式夹具**

当前 `openExecutionService`、`newAPIServer` 和若干 store 测试依赖迁移自动生成腾讯/阿里。新增测试帮助函数，显式写入：

- `legacy/completed` profile；
- 20 万账户；
- `rules.InitialSnapshot()`；
- `hk-0700` 和 `hk-9988`。

测试帮助函数只放在 `_test.go`，不得成为生产初始化 API。保留 `openExecutionService(t)` 名称但让它调用 `seedLegacyTestData`，避免机械修改约 50 处调用；`newAPIServer` 同样继续表示 legacy，新增 `newFreshAPIServer` 专测向导。增加 `openGenericService(t, capitalFen, maxLossFen)`，它对 fresh store 调用正式 `Service.CompleteOnboarding`，供通用规则和配置测试使用。Store 层的 `openTestStore` 保持真正 fresh，仅把依赖七项配置的测试改用 `openLegacyTestStore`。

- [ ] **Step 7: 运行相关后端测试**

Run: `cd backend && go test ./internal/store ./internal/service ./internal/api -count=1`

Expected: PASS；现有历史行为测试继续通过，新空库不再依赖个人播种。

- [ ] **Step 8: 检查差异，不提交**

Run: `git diff --check -- backend/internal/service/onboarding.go backend/internal/service/test_helpers_test.go backend/internal/store/onboarding.go backend/internal/api/router.go backend/internal/api/router_test.go backend/internal/service/executions_test.go backend/internal/store/store_test.go`

Expected: 无输出。

---

### Task 3: 通用规则模式与账户现金解耦

**Files:**
- Modify: `backend/internal/rules/snapshot.go`
- Modify: `backend/internal/rules/engine.go`
- Modify: `backend/internal/rules/engine_test.go`
- Modify: `backend/internal/store/onboarding.go`
- Modify: `backend/internal/store/queries.go`
- Modify: `backend/internal/service/executions.go`
- Modify: `backend/internal/service/executions_test.go`
- Modify: `backend/internal/service/dashboard.go`
- Modify: `backend/internal/service/plans.go`

**Interfaces:**
- Produces: `rules.Snapshot.ProfileMode string`、`EffectiveProfileMode()`、`GenericSnapshot(capitalFen, maxLossFen)`。
- Produces: `Store.AccountInitialCashFen(ctx) (int64, error)`。
- Consumes: Task 2 原子初始化传入的通用快照。

- [ ] **Step 1: 写通用规则不会触发个人门槛的失败测试**

在 `engine_test.go` 新增一张 `GenericSnapshot(20_000_000, 2_000_000)` 规则，断言：

- 0700.HK 不因股数或阿里观察日产生 `INSTRUMENT_SHARE_LIMIT`/`TENCENT_SEQUENCE_GATE`；
- 中国科技计划不产生 `CHINA_TECH_EXPOSURE_LIMIT`/`NO_CROSS_INSTRUMENT_AVERAGING`；
- 同一标的已有浮亏时仍产生 `NO_ADD_TO_LOSER`；
- 最大计划损失超过实际红线仍产生 `PORTFOLIO_LOSS_RED_LINE`；
- 红线消息显示配置值，不含硬编码 `20,000`。

在 `executions_test.go` 用 generic 服务记录超过 100 股的 0700.HK，断言不会因个人上限违规；再构造达到最大损失的组合，断言共性红线仍执行。

- [ ] **Step 2: 运行规则和成交测试确认失败**

Run: `cd backend && go test ./internal/rules ./internal/service -run 'TestGeneric|TestExecutionUsesGeneric' -count=1`

Expected: FAIL，当前快照没有模式且引擎无条件执行个人规则。

- [ ] **Step 3: 增加模式兼容和通用快照**

`Snapshot` 增加：

```go
ProfileMode string `json:"profileMode,omitempty"`
```

并实现：

```go
func (s Snapshot) EffectiveProfileMode() string {
    if s.ProfileMode == "generic" { return "generic" }
    return "legacy"
}

func GenericSnapshot(capitalFen, maxLossFen int64) Snapshot {
    rule := InitialSnapshot()
    rule.ProfileMode = "generic"
    rule.InitialCapitalFen = capitalFen
    rule.LossRedLineFen = maxLossFen
    rule.LossCautionFen = maxLossFen * 75 / 100
    rule.ChinaTechLimitFen = 0
    rule.TencentMaxShares = 0
    rule.AlibabaMaxShares = 0
    rule.EnforceTencentSequenceGate = false
    rule.NoCrossInstrumentAveraging = false
    return rule
}
```

旧快照没有字段时 `EffectiveProfileMode()` 必须返回 legacy。

- [ ] **Step 4: 给规则引擎和成交事实分类加模式门控**

在 `EvaluatePlan` 和 `prepareExecution` 中只在 legacy 模式执行：

- 腾讯/阿里股数上限；
- 中国科技跨标的浮亏禁加；
- 中国科技敞口上限；
- 腾讯顺序门槛。

共性规则保持不变。把红线提示改为 `fmt.Sprintf` 使用 `LossRedLineFen`；legacy 中概上限提示使用 `ChinaTechLimitFen`，不能继续写死金额。

- [ ] **Step 5: 将组合初始现金改为读取账户**

实现：

```go
func (s *Store) AccountInitialCashFen(ctx context.Context) (int64, error)
```

`portfolioFromEvents` 使用账户初始资金调用 `domain.Replay`，不再使用最新规则的 `InitialCapitalFen`。这样后续修改风险基准不会改写历史现金。

Dashboard 增加 `ProfileMode`，`InitialCapitalFen` 使用 profile 的可投资总资金；legacy 无资料金额时回退规则。`ChinaTechLimitFen` 可以继续返回用于 legacy UI，但 generic UI 不显示。

- [ ] **Step 6: 放宽规则版本的固定 20 万校验**

`CreateRuleVersion`：

- legacy 继续要求初始资金与当前 legacy 规则一致，防止意外改写历史行为；
- generic 只允许通过资料修改事务更新资金和红线，普通规则版本接口不得切换 `ProfileMode`；
- 保留警戒线小于红线的校验。

- [ ] **Step 7: 运行规则、服务和 API 测试**

Run: `cd backend && go test ./internal/rules ./internal/service ./internal/api -count=1`

Expected: PASS。

- [ ] **Step 8: 检查差异，不提交**

Run: `git diff --check -- backend/internal/rules backend/internal/store/onboarding.go backend/internal/store/queries.go backend/internal/service/executions.go backend/internal/service/dashboard.go backend/internal/service/plans.go`

Expected: 无输出。

---

### Task 4: App 首次启动门禁与三步向导

**Files:**
- Create: `src/renderer/composables/useUserProfile.ts`
- Create: `src/renderer/views/OnboardingView.vue`
- Create: `src/renderer/views/OnboardingView.spec.ts`
- Modify: `src/renderer/types.ts`
- Modify: `src/renderer/App.vue`
- Modify: `src/renderer/App.spec.ts`
- Modify: `src/renderer/style.css`

**Interfaces:**
- Consumes: `GET /api/onboarding`、`POST /api/onboarding/complete`。
- Produces: `UserProfile`、`CompleteOnboardingInput` 前端类型。
- Produces: `userProfileKey`、`useUserProfile()`，主界面子组件读取同一份 profile。
- Produces: `OnboardingView` emits `completed: [profile: UserProfile]`。

- [ ] **Step 1: 写 App 门禁和向导交互的失败测试**

在 `App.spec.ts` mock `api.request`：

- pending 响应时断言九个导航链接不存在、向导标题存在；
- completed 响应时断言导航存在；
- 请求失败时显示“重新检查本地服务”。

在 `OnboardingView.spec.ts` 覆盖一条完整路径：

1. 空金额不能进入下一步；
2. 输入总资金 200000、最大损失 20000，显示 10%；
3. 选择 6–12 月和 A 股 ETF；
4. 确认隐私说明后提交；
5. POST payload 使用整数分与 `ashare_etf`；
6. 成功 emit completed。

- [ ] **Step 2: 运行前端目标测试确认失败**

Run: `pnpm test -- src/renderer/App.spec.ts src/renderer/views/OnboardingView.spec.ts`

Expected: FAIL，向导和门禁尚不存在。

- [ ] **Step 3: 增加前端资料类型与注入上下文**

在 `types.ts` 定义与 Go 完全一致的 union：

```ts
export type UserMode = 'legacy' | 'generic'
export type OnboardingStatus = 'pending' | 'completed'
export type HoldingHorizon = 'under_6m' | '6_to_12m' | '1_to_3y' | 'over_3y' | 'legacy_unspecified'
export type MarketScope = 'ashare_stock' | 'ashare_etf' | 'hk'

export interface UserProfile {
  id: string
  mode: UserMode
  onboardingStatus: OnboardingStatus
  investableCapitalFen: number
  maxLossFen: number
  holdingHorizon: HoldingHorizon
  enabledMarkets: MarketScope[]
  completedAt?: string
  updatedAt: string
}
```

`useUserProfile.ts` 提供只读 profile 和 `replaceProfile(next)`；没有注入时抛出清楚的开发错误。

- [ ] **Step 4: 实现 OnboardingView 三步表单**

要求：

- 金额以字符串存储，提交时转换为分；
- 第一步即时计算损失比例；
- 第二步使用可多选 checkbox；
- 第三步列出“本机保存、不连接券商、无预设数据”三条确认；
- 每步只显示当前字段，可返回；
- 提交期间禁用按钮；API 字段错误回到对应步骤；
- `ONBOARDING_ALREADY_COMPLETED` 时重新 GET 并在 completed 后 emit，保证超时重试幂等。

- [ ] **Step 5: 重构 App 为状态机门禁**

App 状态明确为 `loading | error | pending | completed`：

- loading：显示品牌和“正在打开本地工作区”；
- error：显示短错误和重试按钮；
- pending：只显示 `OnboardingView`；
- completed：才显示现有侧边栏和 `RouterView`。

只有 completed 时注册价格提醒点击监听。App provide profile；向导成功后 replace profile、跳转 `/` 并进入主壳。Memory Router 即使预先位于其他路径，也不能在 pending 时渲染页面。

- [ ] **Step 6: 运行门禁与向导测试**

Run: `pnpm test -- src/renderer/App.spec.ts src/renderer/views/OnboardingView.spec.ts && pnpm typecheck`

Expected: PASS。

- [ ] **Step 7: 检查差异，不提交**

Run: `git diff --check -- src/renderer/App.vue src/renderer/App.spec.ts src/renderer/composables/useUserProfile.ts src/renderer/views/OnboardingView.vue src/renderer/views/OnboardingView.spec.ts src/renderer/types.ts src/renderer/style.css`

Expected: 无输出。

---

### Task 5: 通用资料修改、历史保留与模式化页面

**Files:**
- Modify: `backend/internal/service/onboarding.go`
- Modify: `backend/internal/store/onboarding.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/router_test.go`
- Modify: `src/renderer/types.ts`
- Modify: `src/renderer/views/SettingsView.vue`
- Modify: `src/renderer/views/SettingsView.spec.ts`
- Modify: `src/renderer/views/TodayView.vue`
- Modify: `src/renderer/views/PositionsView.vue`
- Modify: `src/renderer/views/WeeklyReviewView.vue`
- Modify: `src/renderer/views/MarketView.vue`
- Modify: `src/renderer/App.vue`

**Interfaces:**
- Produces: `service.UpdateUserProfileInput`。
- Produces: `Service.UpdateUserProfile(ctx, input)`。
- Produces: `PUT /api/profile`。
- Consumes: `useUserProfile()` 和 Dashboard 的 `profileMode`。

- [ ] **Step 1: 写资料修改原子性和通用设置页失败测试**

后端 API 测试：

- generic 用户提交新资金、新红线、期限、市场和修改原因后，profile 更新、规则版本从 1 到 2、审计存在；
- 账户 `initial_capital_fen` 不变，Portfolio 可用现金不变；
- 空原因和 `maxLoss >= capital` 返回 422；
- legacy 用户调用返回 409，不改历史。

前端 Settings 测试：

- generic profile 不出现“中国科技敞口”“腾讯顺序门槛”“阿里观察交易日”；
- 显示四组基础资料和修改原因；
- legacy profile 继续显示现有个人规则，并显示“个人历史模式”。

- [ ] **Step 2: 运行目标测试确认失败**

Run: `cd backend && go test ./internal/api -run 'TestUpdateUserProfile' -count=1`

Run: `pnpm test -- src/renderer/views/SettingsView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 实现 generic 资料与规则同事务修改**

`UpdateUserProfileInput` 复用向导四字段并增加 `Reason string`。Store 事务：

1. 读取 before profile 和最新规则 JSON；
2. 要求模式 generic/completed；
3. 创建 `rules.GenericSnapshot`，保留当前通用规则中与冷静期、退出码、同标的禁加相关的可编辑值；
4. 插入新规则版本，previous_id 指向旧规则；
5. 更新 profile，但不更新 `accounts.initial_capital_fen`；
6. 写 profile 和 rule 两条审计；
7. 提交并返回 profile。

服务调用现有自动备份后再进入事务。API 注册 `PUT /api/profile`。

- [ ] **Step 4: 设置页按模式分支**

Settings 从 App 注入 profile：

- generic：显示资金、最大损失、期限、市场、修改原因；保存 `PUT /api/profile`，成功后 `replaceProfile`；监控设置、备份和审计区域继续显示。
- legacy：保留现有损失线、中国科技、腾讯门槛和规则版本表；顶部增加“个人历史模式，升级未改变你的规则”。

不得让 generic 表单继续调用通用规则版本 POST，以免绕过资料事务。

- [ ] **Step 5: 清理通用页面的个人化文案**

- `TodayView`：generic 显示总资金、现金、累计亏损、违规和损失使用；隐藏中国科技与腾讯顺序门槛。
- `PositionsView`：generic 隐藏“中国科技敞口（市值）”，保留现金和累计亏损。
- `WeeklyReviewView`：generic 最近复盘隐藏中国科技指标；把“只跟踪阿里云”等 placeholder 改为通用例子。
- `MarketView`：根据 `enabledMarkets` 隐藏禁用的沪深股票或 ETF 标签；只启用其中一个时自动选择该标签。只启用港股时显示“当前公开榜单仅覆盖 A 股与 ETF”，不发起榜单刷新。
- `App.vue`：generic 且只有港股时隐藏“市场榜单”导航；legacy 保持九项导航。

公共行情缓存不删除，已有事实不因市场开关隐藏。

- [ ] **Step 6: 运行 API、设置页及受影响页面测试**

Run: `cd backend && go test ./internal/api ./internal/service -count=1`

Run: `pnpm test -- src/renderer/App.spec.ts src/renderer/views/SettingsView.spec.ts src/renderer/views/MarketView.spec.ts src/renderer/views/PositionsView.spec.ts src/renderer/views/WeeklyReviewView.spec.ts && pnpm typecheck`

Expected: PASS。

- [ ] **Step 7: 检查差异，不提交**

Run: `git diff --check -- backend/internal/service/onboarding.go backend/internal/store/onboarding.go backend/internal/api/router.go src/renderer/views/SettingsView.vue src/renderer/views/TodayView.vue src/renderer/views/PositionsView.vue src/renderer/views/WeeklyReviewView.vue src/renderer/views/MarketView.vue src/renderer/App.vue`

Expected: 无输出。

---

### Task 6: 通用资产配置后端的空状态与任意项目

**Files:**
- Modify: `backend/internal/domain/allocation.go`
- Modify: `backend/internal/store/allocation.go`
- Modify: `backend/internal/service/allocation.go`
- Modify: `backend/internal/service/allocation_test.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/router_test.go`

**Interfaces:**
- Produces: `domain.AllocationItemRole` (`holding | cash`)。
- Produces: `domain.AllocationState { Configured, SuggestedCapitalFen, Overview }`。
- Changes: `Service.Allocation(ctx) (domain.AllocationState, error)`。
- Changes: `PUT /api/allocation` 在未配置时创建 v1，已配置时创建下一版本。
- Consumes: profile mode 和 investable capital。

- [ ] **Step 1: 写空白配置和任意项目的失败测试**

在 `allocation_test.go` 新增 generic 服务测试：

```go
func TestGenericAllocationStartsUnconfiguredAndAcceptsArbitraryItems(t *testing.T) {
    svc := openGenericService(t, 20_000_000, 2_000_000)
    state, err := svc.Allocation(context.Background())
    if err != nil { t.Fatal(err) }
    if state.Configured || state.SuggestedCapitalFen != 20_000_000 || state.Overview != nil {
        t.Fatalf("state=%#v", state)
    }
    draft := domain.AllocationDraft{
        InitialCapitalFen: 20_000_000,
        Items: []domain.AllocationItem{
            {Key: "global-equity", Name: "全球股票", Role: domain.AllocationRoleHolding, TargetWeightBP: 5_000},
            {Key: "cash", Name: "现金", Role: domain.AllocationRoleCash, TargetWeightBP: 5_000},
        },
    }
    version, err := svc.ReviseAllocation(context.Background(), draft, "建立第一版通用配置")
    if err != nil { t.Fatal(err) }
    if version.Version != 1 { t.Fatalf("version=%#v", version) }
    state, err = svc.Allocation(context.Background())
    if err != nil { t.Fatal(err) }
    if !state.Configured || state.Overview == nil || len(state.Overview.Items) != 2 { t.Fatalf("state=%#v", state) }
    cash := allocationItemByKey(t, *state.Overview, "cash")
    if cash.LinkedValueFen != 20_000_000 { t.Fatalf("cash=%#v", cash) }
}
```

另覆盖：两个现金项被拒绝、重复绑定代码被拒绝、目标合计不为 100% 被拒绝、空截止日和 0 目标收益允许、legacy 首次读取仍生成原七项配置。

- [ ] **Step 2: 运行配置测试确认失败**

Run: `cd backend && go test ./internal/service -run 'TestGenericAllocation|TestLegacyAllocation' -count=1`

Expected: FAIL，当前读取会无条件生成七项配置且只接受固定 key。

- [ ] **Step 3: 扩展配置领域类型并兼容旧 JSON**

在 `AllocationItem` 增加：

```go
type AllocationItemRole string
const (
    AllocationRoleHolding AllocationItemRole = "holding"
    AllocationRoleCash    AllocationItemRole = "cash"
)

Role AllocationItemRole `json:"role,omitempty"`
```

新增：

```go
type AllocationState struct {
    Configured          bool                `json:"configured"`
    SuggestedCapitalFen int64               `json:"suggestedCapitalFen"`
    Overview            *AllocationOverview `json:"overview,omitempty"`
}
```

读取旧配置时：`role=="" && key=="cash-fund"` 解释为 cash，其余解释为 holding，不改写历史 JSON。

- [ ] **Step 4: 拆分“读取”和“创建默认配置”**

Store 增加：

```go
func (s *Store) HasAllocation(ctx context.Context) (bool, error)
func (s *Store) CreateAllocationProfile(ctx context.Context, draft domain.AllocationDraft, reason string, now time.Time) (domain.AllocationVersion, error)
```

`EnsureInitialAllocation` 仅供 legacy 分支调用。generic `Allocation` 在没有 profile 时返回 unconfigured，不能写库。

- [ ] **Step 5: 泛化验证和进度计算**

规则：

- 项目至少一项，名称非空；买卖规则与备注允许为空；
- role 必须是 holding/cash；最多一个 cash；
- cash 不能绑定证券；holding 可绑定零到多个证券；
- 同一代码不能跨项目重复；
- 权重范围 0–100%，总计必须 100%；
- 目标收益允许 0；截止日期为空时允许，否则必须为 YYYY-MM-DD；
- item key 为空时生成 `allocation-item-<random>`，已存在版本的 key 不改；
- 现金进度按 role 判断，不再只判断 `cash-fund`；
- legacy 分支继续应用 `allocationWithDefaultBindings` 和固定七项验证。

删除 generic 路径对 `allocationItemKeys` 的限制；记录调整和查询历史时改为检查当前版本是否真实包含 key。

- [ ] **Step 6: 调整 API 响应和未配置时的版本列表**

GET `/api/allocation` 返回 `AllocationState`。未配置时：

```json
{"configured":false,"suggestedCapitalFen":20000000}
```

GET versions 返回空数组；调整接口在未配置时返回 409 `ALLOCATION_NOT_CONFIGURED`。PUT 创建或修订后仍返回 `AllocationVersion`。

- [ ] **Step 7: 运行配置和 API 测试**

Run: `cd backend && go test ./internal/service ./internal/api -run 'Allocation' -count=1`

Expected: PASS。

- [ ] **Step 8: 检查差异，不提交**

Run: `git diff --check -- backend/internal/domain/allocation.go backend/internal/store/allocation.go backend/internal/service/allocation.go backend/internal/service/allocation_test.go backend/internal/api/router.go backend/internal/api/router_test.go`

Expected: 无输出。

---

### Task 7: 通用资产配置空白界面与动态编辑

**Files:**
- Modify: `src/renderer/types.ts`
- Modify: `src/renderer/views/AllocationView.vue`
- Modify: `src/renderer/views/AllocationView.spec.ts`
- Modify: `src/renderer/components/allocation/AllocationEditor.vue`
- Modify: `src/renderer/components/allocation/AllocationSummary.vue`
- Modify: `src/renderer/components/allocation/AllocationValueForm.vue`

**Interfaces:**
- Consumes: Task 6 的 `AllocationState`。
- Changes: `AllocationEditor` 接收 `draft`、可选 `versionNumber`、`isNew`，不再强依赖完整 overview。
- Produces: 动态增加、删除、排序配置项目和现金角色选择。

- [ ] **Step 1: 写未配置空状态和动态项目的失败测试**

在 `AllocationView.spec.ts` 增加：

- GET 返回 unconfigured 时显示“尚未建立资产配置”和“开始配置”，不出现腾讯/沪深 300/半导体；
- 点击开始配置后基准资金预填 profile 建议值，目标收益和截止日期为空；
- 添加“全球股票”和“现金”，设置 60/40，提交 payload 的 role 正确；
- 删除项目后总权重重新计算；
- configured 响应继续显示 summary、调整、编辑和版本历史。

- [ ] **Step 2: 运行配置前端测试确认失败**

Run: `pnpm test -- src/renderer/views/AllocationView.spec.ts`

Expected: FAIL，当前页面期待直接返回 overview，编辑器不能增加项目。

- [ ] **Step 3: 更新前端类型和空状态读取**

增加：

```ts
export type AllocationItemRole = 'holding' | 'cash'
export interface AllocationState {
  configured: boolean
  suggestedCapitalFen: number
  overview?: AllocationOverview
}
```

`AllocationItem` 增加 `role`。AllocationView 根据 `state.configured` 分支；只有 configured 时加载版本和调整表单，unconfigured 不请求无意义历史。

- [ ] **Step 4: 泛化 AllocationEditor**

编辑器支持：

- “添加资产”创建 holding 项目；
- “添加现金”只在没有 cash 项时可用；
- 项目名称可编辑；
- 上移、下移和删除；
- holding 显示关联证券多选，cash 显示自动读取现金；
- 目标股数、买入规则、卖出规则、备注均可选；
- 新配置按钮文案“建立资产配置”，修订时为“创建配置新版本”；
- 新配置默认 reason 为用户必须填写的初始化原因，不自动写个人文案；
- 使用 `crypto.randomUUID()` 生成稳定 key；测试环境 stub UUID。

- [ ] **Step 5: 让目标收益和日期真正可选**

输入为空时 payload 发送 `targetReturnBP: 0`、`targetDeadline: ''`；Summary 在没有收益目标时显示“未设置目标收益”，没有截止日时不渲染无效日期。所有 `toFixed` 前继续通过 `finiteNumber`，避免之前字符串数值异常复发。

- [ ] **Step 6: 运行配置前端测试与类型检查**

Run: `pnpm test -- src/renderer/views/AllocationView.spec.ts && pnpm typecheck`

Expected: PASS。

- [ ] **Step 7: 检查差异，不提交**

Run: `git diff --check -- src/renderer/types.ts src/renderer/views/AllocationView.vue src/renderer/views/AllocationView.spec.ts src/renderer/components/allocation/AllocationEditor.vue src/renderer/components/allocation/AllocationSummary.vue src/renderer/components/allocation/AllocationValueForm.vue`

Expected: 无输出。

---

### Task 8: 防止未初始化后台任务和接口误用

**Files:**
- Modify: `backend/internal/service/monitor.go`
- Modify: `backend/internal/service/monitor_test.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/router_test.go`
- Modify: `src/main/index.ts`
- Modify: `src/main/monitor-notifications.spec.ts`

**Interfaces:**
- Consumes: `Store.UserProfile(ctx)`。
- Produces: `service.RequireCompletedOnboarding(ctx)` 或等价守卫。
- Behavior: pending 状态只允许 health、onboarding、备份恢复相关接口；不启动行情提醒计算。

- [ ] **Step 1: 写 pending 状态后台安全测试**

覆盖：

- `RunMonitorOnce` 在 pending 时不读取 CurrentRule、不请求网络，返回 disabled/未初始化状态；
- pending 时 GET dashboard 返回 409 `ONBOARDING_REQUIRED`，而 GET onboarding 和 GET health 正常；
- Electron 通知轮询收到未初始化响应时不弹系统通知且不刷错误屏。

- [ ] **Step 2: 运行目标测试确认失败**

Run: `cd backend && go test ./internal/service ./internal/api -run 'OnboardingRequired|PendingMonitor' -count=1`

Run: `pnpm test -- src/main/monitor-notifications.spec.ts`

Expected: 至少后端测试 FAIL，当前业务接口可直接进入缺少规则的路径。

- [ ] **Step 3: 增加统一后端守卫**

在 Router 中使用中间件或 handler helper：除以下白名单外，generic/pending 请求返回 409：

- `/api/health`
- GET `/api/onboarding`
- POST `/api/onboarding/complete`
- 备份验证和恢复所需接口

不要阻止 legacy/completed。错误 envelope 使用 `ONBOARDING_REQUIRED` 和短中文提示。

- [ ] **Step 4: 让 monitor 在 pending 时安静等待**

`RunMonitorOnce` 最先读取 profile；pending 时返回 `Enabled:false, Interval:"off"`，不调用 Portfolio 或公开行情。`StartMonitor` 保持低频等待，向导完成后下一周期可读取用户设置；首次完成不自动开启提醒。

Electron 的通知轮询对 `ONBOARDING_REQUIRED` 视为正常空状态，不输出每分钟错误；其他错误仍写本地日志。

- [ ] **Step 5: 运行后台与 API 测试**

Run: `cd backend && go test ./internal/service ./internal/api -count=1`

Run: `pnpm test -- src/main/monitor-notifications.spec.ts && pnpm typecheck`

Expected: PASS。

- [ ] **Step 6: 检查差异，不提交**

Run: `git diff --check -- backend/internal/service/monitor.go backend/internal/api/router.go src/main/index.ts`

Expected: 无输出。

---

### Task 9: 全量回归、全新启动冒烟与分发检查

**Files:**
- Modify: `scripts/smoke-macos.sh`
- Modify: `README.md`
- Modify: `docs/验收清单.md`
- Modify: `package.json`
- Generated: `resources/bin/discipline-server`
- Generated: `release/Plain Rule-<next-version>-arm64.dmg`

**Interfaces:**
- Consumes: schema 11、新向导 UI、空白配置和所有现有功能。
- Produces: 可分发 ARM64 DMG、哈希和可复现的首次启动验收结果。

- [ ] **Step 1: 更新 macOS 冒烟脚本的全新空库断言**

把旧的 `account_count=1`、`rule_count=1` 改为：

```zsh
profile_mode="$(sqlite3 "$database" "SELECT mode FROM user_profiles WHERE id='local-user';")"
onboarding_status="$(sqlite3 "$database" "SELECT onboarding_status FROM user_profiles WHERE id='local-user';")"
account_count="$(sqlite3 "$database" 'SELECT count(*) FROM accounts;')"
rule_count="$(sqlite3 "$database" 'SELECT count(*) FROM rule_versions;')"
instrument_count="$(sqlite3 "$database" 'SELECT count(*) FROM instruments;')"
allocation_count="$(sqlite3 "$database" 'SELECT count(*) FROM allocation_profiles;')"
```

断言 `generic/pending`、四个 count 全为 0、schema=11、integrity=ok。脚本仍使用临时 `--user-data-dir`，不得碰真实 Application Support。

- [ ] **Step 2: 更新说明和验收清单**

README 明确：

- 每位用户首次启动自己设置风险边界；
- 应用初始不包含证券、持仓和资产配置；
- 数据只在各自 Mac 用户目录；
- 目前分发包为 Apple Silicon ARM64、无开发依赖、未使用正式开发者签名；
- Intel Mac 不在本次产物支持范围。

验收清单加入 fresh/legacy 两条路径和“任何页面无个人默认文案”的人工检查。

- [ ] **Step 3: 更新版本并执行全量验证**

将 `package.json` patch 版本从 `0.1.17` 提升到 `0.1.18`。

Run: `pnpm verify`

Expected: Go 全量、全部前端测试、TypeScript 和生产构建通过。

- [ ] **Step 4: 使用临时数据库验证 schema 11 两条路径**

Run: `pnpm build:backend && zsh scripts/smoke-macos.sh`

Expected: fresh 数据库为 pending 且无业务种子。

另复制一个测试夹具数据库到临时目录运行后端迁移，比较迁移前后以下值：账户数、规则数、证券数、计划数、成交数、allocation version 数以及最新规则 `snapshot_json`。Expected: 除新增 profile/schema 行外完全一致。

- [ ] **Step 5: 构建 DMG 并检查资源内容**

Run: `pnpm build:mac`

Run: `find release/mac-arm64/Plain\ Rule.app -type f \( -name 'discipline.db' -o -name '*.png' -o -name '*.xlsx' \) -print`

Expected: 不出现 `discipline.db`、用户截图或 Excel；允许的应用图标路径需人工确认不是用户截图。

进一步检查 asar 文件列表中不包含 `discipline.db`、`Downloads`、`codex-clipboard` 或真实用户名路径。

- [ ] **Step 6: 记录产物和哈希**

Run: `shasum -a 256 'release/Plain Rule-0.1.18-arm64.dmg'`

Expected: 输出一个 SHA-256；把实际值写入本轮交付说明，不写死在源码。

- [ ] **Step 7: 最终差异和敏感信息检查，不提交**

Run: `git diff --check`

Run: `rg -n '/Users/henryhua|codex-clipboard|20万资产配置动态跟踪表' --glob '!docs/superpowers/**' --glob '!release/**' .`

Expected: 业务源码和产物配置中无个人绝对路径或剪贴板文件名；文档中的历史产品说明如确需保留，逐项确认不是运行时默认数据。

Run: `git status --short`

Expected: 仅看到现有功能差异、本计划实现文件、生成后端二进制和新版本构建产物；不执行 commit 或 merge。

---

## 最终人工验收路径

1. 用全新临时用户目录打开应用，只看到三步向导。
2. 退出后重开，仍停在向导且没有账户、规则、证券和资产配置。
3. 填写资金、最大损失、期限与市场，完成后进入今日页。
4. 检查观察名单、计划、成交、持仓、复盘和资产配置均为空。
5. 检查今日、持仓、设置和复盘没有腾讯、阿里或中国科技个人门槛。
6. 从资产配置空状态建立两项自定义配置，保存后现金与真实持仓联动。
7. 修改风险资料并填写原因，确认规则版本增加但现金不跳变。
8. 用旧数据库副本启动，确认直接进入主界面且原个人规则、持仓和配置保持不变。
9. 打开 DMG 内容确认不含本地数据库和用户文件。
