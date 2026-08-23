package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/domain"
	"github.com/local/trade-discipline-desktop/backend/internal/market"
	"github.com/local/trade-discipline-desktop/backend/internal/rules"
	"github.com/local/trade-discipline-desktop/backend/internal/service"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type Router struct {
	service *service.Service
}

func NewRouter(svc *service.Service, token string) http.Handler {
	router := &Router{service: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", router.health)
	mux.HandleFunc("GET /api/dashboard", router.dashboard)
	mux.HandleFunc("GET /api/instruments", router.instruments)
	mux.HandleFunc("GET /api/plans", router.listPlans)
	mux.HandleFunc("POST /api/plans", router.createPlan)
	mux.HandleFunc("PUT /api/plans/{id}", router.revisePlan)
	mux.HandleFunc("POST /api/plans/{id}/pre-trade-confirmations", router.createPreTradeConfirmation)
	mux.HandleFunc("GET /api/plans/{id}/pre-trade-confirmations/latest", router.latestPreTradeConfirmation)
	mux.HandleFunc("POST /api/executions", router.createExecution)
	mux.HandleFunc("POST /api/executions/quick", router.createQuickExecution)
	mux.HandleFunc("POST /api/executions/{id}/reverse", router.reverseExecution)
	mux.HandleFunc("GET /api/post-trade-reviews", router.pendingPostTradeReviews)
	mux.HandleFunc("POST /api/post-trade-reviews/{executionID}/complete", router.completePostTradeReview)
	mux.HandleFunc("GET /api/portfolio", router.portfolio)
	mux.HandleFunc("GET /api/allocation", router.allocation)
	mux.HandleFunc("PUT /api/allocation", router.reviseAllocation)
	mux.HandleFunc("GET /api/allocation/versions", router.allocationVersions)
	mux.HandleFunc("GET /api/allocation/items/{key}/value-events", router.allocationValueEvents)
	mux.HandleFunc("POST /api/allocation/items/{key}/value-events", router.recordAllocationValue)
	mux.HandleFunc("GET /api/monitor/status", router.monitorStatus)
	mux.HandleFunc("GET /api/monitor/settings", router.monitorSettings)
	mux.HandleFunc("PUT /api/monitor/settings", router.saveMonitorSettings)
	mux.HandleFunc("GET /api/monitor/alerts", router.pendingMonitorAlerts)
	mux.HandleFunc("GET /api/monitor/alerts/unnotified", router.unnotifiedMonitorAlerts)
	mux.HandleFunc("POST /api/monitor/alerts/{id}/notified", router.markMonitorAlertNotified)
	mux.HandleFunc("POST /api/monitor/alerts/{id}/reviews", router.createPositionReview)
	mux.HandleFunc("GET /api/rules", router.listRules)
	mux.HandleFunc("POST /api/rules/versions", router.createRuleVersion)
	mux.HandleFunc("GET /api/audit", router.audit)
	mux.HandleFunc("POST /api/market/refresh", router.refreshMarket)
	mux.HandleFunc("GET /api/market/snapshots/latest", router.latestMarket)
	mux.HandleFunc("POST /api/market/live/refresh", router.refreshLiveMarket)
	mux.HandleFunc("GET /api/market/live/latest", router.latestLiveMarket)
	mux.HandleFunc("GET /api/market/refresh-status", router.marketRefreshStatus)
	mux.HandleFunc("GET /api/market/overview", router.marketOverview)
	mux.HandleFunc("GET /api/market/history", router.marketHistory)
	mux.HandleFunc("POST /api/market/history/refresh", router.refreshMarketHistory)
	mux.HandleFunc("POST /api/market/csv/preview", router.previewMarketCSV)
	mux.HandleFunc("POST /api/market/csv/confirm", router.confirmMarketCSV)
	mux.HandleFunc("GET /api/reviews/current", router.latestReview)
	mux.HandleFunc("POST /api/reviews", router.createReview)
	mux.HandleFunc("GET /api/watchlist", router.listWatchlist)
	mux.HandleFunc("POST /api/watchlist", router.createWatchlist)
	mux.HandleFunc("POST /api/backup/export", router.exportBackup)
	mux.HandleFunc("POST /api/backup/validate", router.validateBackup)
	return LocalCORS(RequireToken(token, requestLimits(mux)))
}

func requestLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		next.ServeHTTP(w, r)
	})
}

func (rt *Router) health(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, map[string]any{"status": "ok", "schemaVersion": store.CurrentSchemaVersion})
}

func (rt *Router) dashboard(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.Dashboard(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) instruments(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.Instruments(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) listPlans(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.ListPlans(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) createPlan(w http.ResponseWriter, r *http.Request) {
	var draft domain.TradePlanDraft
	if err := decodeJSON(r, &draft); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_JSON", "计划内容格式无效", nil)
		return
	}
	data, err := rt.service.CreatePlan(r.Context(), draft)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) revisePlan(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reason string                `json:"reason"`
		Draft  domain.TradePlanDraft `json:"draft"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_PLAN_REVISION", "计划修订格式无效", nil)
		return
	}
	data, err := rt.service.RevisePlan(r.Context(), r.PathValue("id"), input.Reason, input.Draft)
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) createPreTradeConfirmation(w http.ResponseWriter, r *http.Request) {
	var input service.PreTradeConfirmationInput
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_PRE_TRADE_CONFIRMATION", "开仓前确认内容无效", nil)
		return
	}
	data, err := rt.service.ConfirmPreTrade(r.Context(), r.PathValue("id"), input)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) latestPreTradeConfirmation(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.LatestPreTradeConfirmation(r.Context(), r.PathValue("id"))
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) createExecution(w http.ResponseWriter, r *http.Request) {
	var draft service.ExecutionDraft
	if err := decodeJSON(r, &draft); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_JSON", "成交内容格式无效", nil)
		return
	}
	data, err := rt.service.RecordExecution(r.Context(), draft)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) createQuickExecution(w http.ResponseWriter, r *http.Request) {
	var draft service.QuickExecutionDraft
	if err := decodeJSON(r, &draft); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_QUICK_EXECUTION", "极速补录内容格式无效", nil)
		return
	}
	data, err := rt.service.RecordQuickExecution(r.Context(), draft)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) pendingPostTradeReviews(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.PendingPostTradeReviews(r.Context(), r.URL.Query().Get("periodStart"), r.URL.Query().Get("periodEnd"))
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) completePostTradeReview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Note string `json:"note"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_POST_TRADE_REVIEW", "成交复盘内容格式无效", nil)
		return
	}
	data, err := rt.service.CompletePostTradeReview(r.Context(), r.PathValue("executionID"), input.Note)
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) reverseExecution(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_REVERSAL", "冲正内容格式无效", nil)
		return
	}
	data, err := rt.service.ReverseExecution(r.Context(), r.PathValue("id"), input.Reason)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) portfolio(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.Portfolio(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) allocation(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.Allocation(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) reviseAllocation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reason string                 `json:"reason"`
		Draft  domain.AllocationDraft `json:"draft"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_ALLOCATION_REVISION", "资产配置修订格式无效", nil)
		return
	}
	data, err := rt.service.ReviseAllocation(r.Context(), input.Draft, input.Reason)
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) allocationVersions(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.AllocationVersions(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) allocationValueEvents(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.AllocationValueEvents(r.Context(), r.PathValue("key"))
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) recordAllocationValue(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ValueFen   int64     `json:"valueFen"`
		ObservedAt time.Time `json:"observedAt"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_ALLOCATION_VALUE", "资产市值记录格式无效", nil)
		return
	}
	data, err := rt.service.RecordAllocationValue(r.Context(), r.PathValue("key"), input.ValueFen, input.ObservedAt)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) monitorStatus(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.MonitorStatus(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) monitorSettings(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.MonitorSettings(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) saveMonitorSettings(w http.ResponseWriter, r *http.Request) {
	var settings domain.MonitorSettings
	if err := decodeJSON(r, &settings); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_MONITOR_SETTINGS", "监控设置内容无效", nil)
		return
	}
	err := rt.service.SaveMonitorSettings(r.Context(), settings)
	writeResult(w, settings, err, http.StatusOK)
}

func (rt *Router) pendingMonitorAlerts(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.PendingPriceAlerts(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) unnotifiedMonitorAlerts(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.UnnotifiedPriceAlerts(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) markMonitorAlertNotified(w http.ResponseWriter, r *http.Request) {
	err := rt.service.MarkPriceAlertNotified(r.Context(), r.PathValue("id"))
	writeResult(w, map[string]string{"id": r.PathValue("id")}, err, http.StatusOK)
}

func (rt *Router) createPositionReview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Decision domain.ReviewDecision `json:"decision"`
		Reason   string                `json:"reason"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_POSITION_REVIEW", "持仓复核内容无效", nil)
		return
	}
	data, err := rt.service.CompletePositionReview(r.Context(), r.PathValue("id"), input.Decision, input.Reason)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) listRules(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.ListRuleVersions(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) createRuleVersion(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reason   string         `json:"reason"`
		Snapshot rules.Snapshot `json:"snapshot"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_JSON", "规则内容格式无效", nil)
		return
	}
	data, err := rt.service.CreateRuleVersion(r.Context(), input.Reason, input.Snapshot)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) audit(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.Audit(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) refreshMarket(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.RefreshMarket(r.Context())
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) latestMarket(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.LatestMarket(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) refreshLiveMarket(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.RefreshLiveMarket(r.Context())
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) latestLiveMarket(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.LatestLiveMarket(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) marketRefreshStatus(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.MarketRefreshStatuses(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) marketOverview(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "3m"
	}
	if !validMarketRange(rangeName) {
		writeFailure(w, http.StatusBadRequest, "INVALID_MARKET_RANGE", "行情范围必须是 1m 或 3m", FieldError{"range": "请选择近 1 个月或近 3 个月"})
		return
	}
	data, err := rt.service.MarketOverview(r.Context(), rangeName)
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) marketHistory(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "3m"
	}
	key, ok := parseHistoryKey(r.URL.Query().Get("market"), r.URL.Query().Get("code"))
	if !ok || !validMarketRange(rangeName) {
		writeFailure(w, http.StatusBadRequest, "INVALID_MARKET_HISTORY", "历史行情参数无效", FieldError{"market": "市场仅支持 SH、SZ 或 HK", "code": "证券代码不能为空", "range": "范围仅支持 1m 或 3m"})
		return
	}
	data, err := rt.service.MarketHistory(r.Context(), key, rangeName)
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) refreshMarketHistory(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Market string `json:"market"`
		Code   string `json:"code"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_MARKET_HISTORY", "历史行情刷新内容无效", nil)
		return
	}
	key, ok := parseHistoryKey(input.Market, input.Code)
	if !ok {
		writeFailure(w, http.StatusBadRequest, "INVALID_MARKET_HISTORY", "历史行情参数无效", FieldError{"market": "市场仅支持 SH、SZ 或 HK", "code": "证券代码不能为空"})
		return
	}
	data, err := rt.service.RefreshMarketHistory(r.Context(), key)
	writeResult(w, data, err, http.StatusCreated)
}

func validMarketRange(rangeName string) bool {
	return rangeName == "1m" || rangeName == "3m"
}

func parseHistoryKey(marketName, code string) (market.InstrumentKey, bool) {
	marketName = strings.ToUpper(strings.TrimSpace(marketName))
	code = strings.ToUpper(strings.TrimSpace(code))
	if (marketName != "SH" && marketName != "SZ" && marketName != "HK") || code == "" {
		return market.InstrumentKey{}, false
	}
	return market.InstrumentKey{Market: marketName, Code: code}, true
}

func (rt *Router) previewMarketCSV(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &input); err != nil || input.Content == "" {
		writeFailure(w, http.StatusBadRequest, "INVALID_CSV", "请选择有效的 CSV 文件", FieldError{"content": "CSV 内容不能为空"})
		return
	}
	preview, digest := rt.service.PreviewMarketCSV(strings.NewReader(input.Content))
	writeSuccess(w, http.StatusOK, map[string]any{"preview": preview, "digest": digest})
}

func (rt *Router) confirmMarketCSV(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Preview market.CSVPreview `json:"preview"`
		Digest  string            `json:"digest"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_CSV_CONFIRMATION", "CSV 确认内容无效", nil)
		return
	}
	data, err := rt.service.ConfirmMarketCSV(r.Context(), input.Preview, input.Digest)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) latestReview(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.LatestWeeklyReview(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) createReview(w http.ResponseWriter, r *http.Request) {
	var input service.WeeklyReviewDraft
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_REVIEW", "复盘内容格式无效", nil)
		return
	}
	data, err := rt.service.SaveWeeklyReview(r.Context(), input)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) listWatchlist(w http.ResponseWriter, r *http.Request) {
	data, err := rt.service.ListWatchlist(r.Context())
	writeResult(w, data, err, http.StatusOK)
}

func (rt *Router) createWatchlist(w http.ResponseWriter, r *http.Request) {
	var input service.WatchlistDraft
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_WATCHLIST", "观察内容格式无效", nil)
		return
	}
	data, err := rt.service.AddWatchlist(r.Context(), input)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) exportBackup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_BACKUP_PATH", "备份路径无效", nil)
		return
	}
	data, err := rt.service.ExportBackup(r.Context(), input.Path)
	writeResult(w, data, err, http.StatusCreated)
}

func (rt *Router) validateBackup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeFailure(w, http.StatusBadRequest, "INVALID_BACKUP_PATH", "备份路径无效", nil)
		return
	}
	data, err := rt.service.ValidateBackup(input.Path)
	writeResult(w, data, err, http.StatusOK)
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func writeResult[T any](w http.ResponseWriter, data T, err error, status int) {
	if err != nil {
		code := "BUSINESS_RULE"
		if coded, ok := service.ErrorCode(err); ok {
			code = coded
		}
		writeFailure(w, http.StatusUnprocessableEntity, code, err.Error(), nil)
		return
	}
	writeSuccess(w, status, data)
}

func writeSuccess[T any](w http.ResponseWriter, status int, data T) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Success(data))
}

func writeFailure(w http.ResponseWriter, status int, code, message string, fields FieldError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Failure(code, message, fields))
}
