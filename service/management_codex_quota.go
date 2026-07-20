package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	managementCodexQuotaDefaultBaseURL = "https://api.sigora.top"
	managementCodexQuotaPath           = "/v0/management"
	managementCodexQuotaCacheTTL       = 60 * time.Second
	managementCodexQuotaHTTPTimeout    = 15 * time.Second
	managementCodexQuotaMaxBodyBytes   = 4 << 20
	managementCodexWhamUsageURL        = "https://chatgpt.com/backend-api/wham/usage"
)

type ManagementCodexQuotas struct {
	Configured bool                       `json:"configured"`
	Items      []ManagementCodexQuotaItem `json:"items"`
	UpdatedAt  int64                      `json:"updated_at"`
}

type ManagementCodexQuotaItem struct {
	Name                                string                       `json:"name"`
	AuthIndex                           string                       `json:"auth_index,omitempty"`
	AccountHash                         string                       `json:"-"`
	PlanType                            string                       `json:"plan_type,omitempty"`
	SubscriptionActiveUntil             string                       `json:"subscription_active_until,omitempty"`
	RateLimitResetCreditsAvailableCount *float64                     `json:"rate_limit_reset_credits_available_count,omitempty"`
	Windows                             []ManagementCodexQuotaWindow `json:"windows"`
	Error                               string                       `json:"error,omitempty"`
	ErrorStatus                         int                          `json:"error_status,omitempty"`
	UpdatedAt                           int64                        `json:"updated_at"`
}

type ManagementCodexQuotaWindow struct {
	ID               string   `json:"id"`
	WindowSeconds    *int64   `json:"window_seconds,omitempty"`
	UsedPercent      *float64 `json:"used_percent,omitempty"`
	RemainingPercent *float64 `json:"remaining_percent,omitempty"`
	ResetAt          *int64   `json:"reset_at,omitempty"`
	ResetAtDerived   bool     `json:"-"`
}

type managementCodexQuotaConfig struct {
	baseURL       string
	managementURL string
	key           string
	concurrency   int
}

type managementCodexAuthFile struct {
	Name                    string
	AuthIndex               string
	AccountID               string
	PlanType                string
	SubscriptionActiveUntil string
	Raw                     map[string]any
}

type managementAuthFilesResponse struct {
	Files []map[string]any `json:"files"`
}

type managementAPICallRequest struct {
	AuthIndex string         `json:"authIndex,omitempty"`
	Method    string         `json:"method"`
	URL       string         `json:"url"`
	Header    map[string]any `json:"header,omitempty"`
	Data      any            `json:"data,omitempty"`
}

type managementAPICallResponse struct {
	StatusCode      int             `json:"status_code"`
	StatusCodeCamel int             `json:"statusCode"`
	Body            json.RawMessage `json:"body"`
	BodyText        string          `json:"body_text"`
	BodyTextCamel   string          `json:"bodyText"`
}

var managementCodexQuotaCache = struct {
	sync.Mutex
	key       string
	expiresAt time.Time
	data      *ManagementCodexQuotas
}{}

func GetManagementCodexQuotas(ctx context.Context) (*ManagementCodexQuotas, error) {
	return getManagementCodexQuotas(ctx, false)
}

func RefreshManagementCodexQuotas(ctx context.Context) (*ManagementCodexQuotas, error) {
	return getManagementCodexQuotas(ctx, true)
}

func getManagementCodexQuotas(ctx context.Context, force bool) (*ManagementCodexQuotas, error) {
	cfg := getManagementCodexQuotaConfig()
	if cfg.key == "" {
		return &ManagementCodexQuotas{
			Configured: false,
			Items:      []ManagementCodexQuotaItem{},
			UpdatedAt:  time.Now().Unix(),
		}, nil
	}

	cacheKey := cfg.managementURL + "|" + cfg.key
	if !force {
		if data := getCachedManagementCodexQuotas(cacheKey); data != nil {
			return data, nil
		}
	}

	client := getManagementCodexHTTPClient()
	files, err := fetchManagementCodexAuthFiles(ctx, client, cfg)
	if err != nil {
		return nil, err
	}

	result := &ManagementCodexQuotas{
		Configured: true,
		Items:      fetchManagementCodexQuotaItems(ctx, client, cfg, files),
		UpdatedAt:  time.Now().Unix(),
	}
	setCachedManagementCodexQuotas(cacheKey, result)
	return result, nil
}

func getManagementCodexQuotaConfig() managementCodexQuotaConfig {
	baseURL := strings.TrimRight(strings.TrimSpace(common.GetEnvOrDefaultString("SIGORA_QUOTA_MANAGEMENT_BASE_URL", managementCodexQuotaDefaultBaseURL)), "/")
	key := strings.TrimSpace(os.Getenv("SIGORA_QUOTA_MANAGEMENT_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("CLI_PROXY_MANAGEMENT_KEY"))
	}
	if key == "" {
		key = strings.TrimSpace(os.Getenv("MANAGEMENT_KEY"))
	}

	concurrency := common.GetEnvOrDefault("SIGORA_QUOTA_CONCURRENCY", 2)
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 4 {
		concurrency = 4
	}

	return managementCodexQuotaConfig{
		baseURL:       baseURL,
		managementURL: normalizeManagementURL(baseURL),
		key:           key,
		concurrency:   concurrency,
	}
}

func normalizeManagementURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = managementCodexQuotaDefaultBaseURL
	}
	if strings.HasSuffix(trimmed, managementCodexQuotaPath) {
		return trimmed
	}
	return trimmed + managementCodexQuotaPath
}

func getManagementCodexHTTPClient() *http.Client {
	client := GetHttpClient()
	if client == nil {
		return &http.Client{Timeout: managementCodexQuotaHTTPTimeout}
	}
	clientCopy := *client
	clientCopy.Timeout = managementCodexQuotaHTTPTimeout
	return &clientCopy
}

func getCachedManagementCodexQuotas(cacheKey string) *ManagementCodexQuotas {
	managementCodexQuotaCache.Lock()
	defer managementCodexQuotaCache.Unlock()

	if managementCodexQuotaCache.key == cacheKey &&
		managementCodexQuotaCache.data != nil &&
		time.Now().Before(managementCodexQuotaCache.expiresAt) {
		return managementCodexQuotaCache.data
	}
	return nil
}

func setCachedManagementCodexQuotas(cacheKey string, data *ManagementCodexQuotas) {
	managementCodexQuotaCache.Lock()
	defer managementCodexQuotaCache.Unlock()

	managementCodexQuotaCache.key = cacheKey
	managementCodexQuotaCache.data = data
	managementCodexQuotaCache.expiresAt = time.Now().Add(managementCodexQuotaCacheTTL)
}

func fetchManagementCodexAuthFiles(ctx context.Context, client *http.Client, cfg managementCodexQuotaConfig) ([]managementCodexAuthFile, error) {
	var payload managementAuthFilesResponse
	statusCode, err := managementCodexJSON(ctx, client, http.MethodGet, cfg.managementURL+"/auth-files", cfg.key, nil, &payload, managementCodexQuotaMaxBodyBytes)
	if err != nil {
		if statusCode > 0 {
			return nil, fmt.Errorf("management auth-files status %d: %w", statusCode, err)
		}
		return nil, err
	}

	files := make([]managementCodexAuthFile, 0)
	for _, raw := range payload.Files {
		provider := normalizeManagementProvider(firstString(raw, "provider", "type"))
		if provider != "codex" || firstBool(raw, "disabled") {
			continue
		}
		name := firstString(raw, "name", "path")
		if name == "" {
			name = firstString(raw, "filename", "file")
		}
		authIndex := firstString(raw, "auth_index", "authIndex")
		files = append(files, managementCodexAuthFile{
			Name:                    name,
			AuthIndex:               authIndex,
			AccountID:               findCodexAccountID(raw),
			PlanType:                findCodexPlanType(raw),
			SubscriptionActiveUntil: findCodexSubscriptionActiveUntil(raw),
			Raw:                     raw,
		})
	}

	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})
	return files, nil
}

func fetchManagementCodexQuotaItems(ctx context.Context, client *http.Client, cfg managementCodexQuotaConfig, files []managementCodexAuthFile) []ManagementCodexQuotaItem {
	items := make([]ManagementCodexQuotaItem, len(files))
	workerCount := cfg.concurrency
	if workerCount > len(files) {
		workerCount = len(files)
	}
	if workerCount < 1 {
		return items
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				items[index] = fetchManagementCodexQuotaItem(ctx, client, cfg, files[index])
			}
		}()
	}

	for index := range files {
		select {
		case <-ctx.Done():
			items[index] = failedManagementCodexQuotaItem(files[index], ctx.Err(), 0)
		case jobs <- index:
		}
	}
	close(jobs)
	wg.Wait()
	return items
}

func fetchManagementCodexQuotaItem(ctx context.Context, client *http.Client, cfg managementCodexQuotaConfig, file managementCodexAuthFile) ManagementCodexQuotaItem {
	item := ManagementCodexQuotaItem{
		Name:                    file.Name,
		AuthIndex:               file.AuthIndex,
		AccountHash:             hashCodexAccountID(file.AccountID),
		PlanType:                file.PlanType,
		SubscriptionActiveUntil: file.SubscriptionActiveUntil,
		UpdatedAt:               time.Now().Unix(),
	}
	if file.AuthIndex == "" {
		item.Error = "codex auth file is missing auth_index"
		return item
	}

	headers := map[string]any{
		"Authorization": "Bearer $TOKEN$",
		"Content-Type":  "application/json",
		"User-Agent":    "codex_cli_rs/0.76.0",
	}
	if file.AccountID != "" {
		headers["Chatgpt-Account-Id"] = file.AccountID
	}

	reqPayload := managementAPICallRequest{
		AuthIndex: file.AuthIndex,
		Method:    http.MethodGet,
		URL:       managementCodexWhamUsageURL,
		Header:    headers,
	}

	var apiPayload managementAPICallResponse
	statusCode, err := managementCodexJSON(ctx, client, http.MethodPost, cfg.managementURL+"/api-call", cfg.key, reqPayload, &apiPayload, managementCodexQuotaMaxBodyBytes)
	if err != nil {
		return failedManagementCodexQuotaItem(file, err, statusCode)
	}

	upstreamStatus := apiPayload.statusCode()
	if upstreamStatus < http.StatusOK || upstreamStatus >= http.StatusMultipleChoices {
		return failedManagementCodexQuotaItem(file, fmt.Errorf("upstream status %d", upstreamStatus), upstreamStatus)
	}

	var usage map[string]any
	if err := apiPayload.decodeBody(&usage); err != nil {
		return failedManagementCodexQuotaItem(file, err, upstreamStatus)
	}

	if planType := normalizePlanType(firstString(usage, "plan_type", "planType")); planType != "" {
		item.PlanType = planType
	}
	if availableCount, ok := rateLimitResetCreditsAvailableCount(usage); ok {
		item.RateLimitResetCreditsAvailableCount = &availableCount
	}
	item.Windows = buildManagementCodexQuotaWindows(usage, time.Now())
	return item
}

func failedManagementCodexQuotaItem(file managementCodexAuthFile, err error, statusCode int) ManagementCodexQuotaItem {
	item := ManagementCodexQuotaItem{
		Name:                    file.Name,
		AuthIndex:               file.AuthIndex,
		AccountHash:             hashCodexAccountID(file.AccountID),
		PlanType:                file.PlanType,
		SubscriptionActiveUntil: file.SubscriptionActiveUntil,
		ErrorStatus:             statusCode,
		UpdatedAt:               time.Now().Unix(),
	}
	if err != nil {
		item.Error = err.Error()
	}
	return item
}

func managementCodexJSON(ctx context.Context, client *http.Client, method string, url string, managementKey string, payload any, dest any, maxBytes int64) (int, error) {
	var body io.Reader
	if payload != nil {
		data, err := common.Marshal(payload)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+managementKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	data, err := readLimitedBody(resp.Body, maxBytes)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp.StatusCode, fmt.Errorf("%s", strings.TrimSpace(string(data)))
	}
	if dest == nil {
		return resp.StatusCode, nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, common.Unmarshal(data, dest)
}

func readLimitedBody(reader io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func decodeManagementAPICallBody(raw json.RawMessage, dest any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return errors.New("empty quota response")
	}
	if trimmed[0] != '"' {
		return common.Unmarshal(trimmed, dest)
	}

	var text string
	if err := common.Unmarshal(trimmed, &text); err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("empty quota response")
	}
	return common.Unmarshal([]byte(text), dest)
}

func (resp managementAPICallResponse) decodeBody(dest any) error {
	if len(bytes.TrimSpace(resp.Body)) > 0 {
		return decodeManagementAPICallBody(resp.Body, dest)
	}
	bodyText := strings.TrimSpace(resp.BodyText)
	if bodyText == "" {
		bodyText = strings.TrimSpace(resp.BodyTextCamel)
	}
	if bodyText == "" {
		return errors.New("empty quota response")
	}
	return common.Unmarshal([]byte(bodyText), dest)
}

func (resp managementAPICallResponse) statusCode() int {
	if resp.StatusCode > 0 {
		return resp.StatusCode
	}
	return resp.StatusCodeCamel
}

func rateLimitResetCreditsAvailableCount(usage map[string]any) (float64, bool) {
	credits := objectField(usage, "rate_limit_reset_credits", "rateLimitResetCredits")
	if credits == nil {
		return 0, false
	}
	return numberField(credits, "available_count", "availableCount")
}

func buildManagementCodexQuotaWindows(usage map[string]any, now time.Time) []ManagementCodexQuotaWindow {
	rateLimit := objectField(usage, "rate_limit", "rateLimit")
	if rateLimit == nil {
		return nil
	}

	primary := objectField(rateLimit, "primary_window", "primaryWindow")
	secondary := objectField(rateLimit, "secondary_window", "secondaryWindow")
	fiveHour, weekly := selectCodexQuotaWindows(primary, secondary)

	windows := make([]ManagementCodexQuotaWindow, 0, 2)
	if fiveHour != nil {
		windows = append(windows, buildManagementCodexQuotaWindow("five-hour", fiveHour, rateLimit, now))
	}
	if weekly != nil {
		id := "weekly"
		if seconds, ok := numberField(weekly, "limit_window_seconds", "limitWindowSeconds"); ok && isMonthlyWindow(seconds) {
			id = "monthly"
		}
		windows = append(windows, buildManagementCodexQuotaWindow(id, weekly, rateLimit, now))
	}
	return windows
}

func selectCodexQuotaWindows(primary map[string]any, secondary map[string]any) (map[string]any, map[string]any) {
	var fiveHour map[string]any
	var weekly map[string]any
	for _, window := range []map[string]any{primary, secondary} {
		if window == nil {
			continue
		}
		seconds, ok := numberField(window, "limit_window_seconds", "limitWindowSeconds")
		if !ok {
			continue
		}
		if seconds == 18000 && fiveHour == nil {
			fiveHour = window
		}
		if (seconds == 604800 || isMonthlyWindow(seconds)) && weekly == nil {
			weekly = window
		}
	}
	if fiveHour == nil && primary != nil && !sameCodexQuotaWindow(primary, weekly) {
		fiveHour = primary
	}
	if weekly == nil && secondary != nil && !sameCodexQuotaWindow(secondary, fiveHour) {
		weekly = secondary
	}
	return fiveHour, weekly
}

func sameCodexQuotaWindow(a map[string]any, b map[string]any) bool {
	if a == nil || b == nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}

func buildManagementCodexQuotaWindow(id string, window map[string]any, rateLimit map[string]any, now time.Time) ManagementCodexQuotaWindow {
	result := ManagementCodexQuotaWindow{ID: id}
	if seconds, ok := numberField(window, "limit_window_seconds", "limitWindowSeconds"); ok {
		rounded := int64(math.Round(seconds))
		result.WindowSeconds = &rounded
	}

	if used, ok := numberField(window, "used_percent", "usedPercent"); ok {
		result.UsedPercent = common.GetPointer(clampPercent(used))
	}
	if remaining, ok := numberField(window, "remaining_percent", "remainingPercent"); ok {
		result.RemainingPercent = common.GetPointer(clampPercent(remaining))
	}

	if resetAt, resetAtDerived, ok := resetAtField(window, now); ok {
		result.ResetAt = &resetAt
		result.ResetAtDerived = resetAtDerived
	}

	if result.UsedPercent == nil && result.ResetAt != nil {
		limitReached, limitReachedOK := boolField(rateLimit, "limit_reached", "limitReached")
		allowed, allowedOK := boolField(rateLimit, "allowed")
		if limitReachedOK && limitReached || allowedOK && !allowed {
			result.UsedPercent = common.GetPointer(float64(100))
		}
	}
	if result.UsedPercent == nil && result.RemainingPercent != nil {
		result.UsedPercent = common.GetPointer(clampPercent(100 - *result.RemainingPercent))
	}
	if result.RemainingPercent == nil && result.UsedPercent != nil {
		result.RemainingPercent = common.GetPointer(clampPercent(100 - *result.UsedPercent))
	}
	return result
}

func resetAtField(window map[string]any, now time.Time) (int64, bool, bool) {
	if value, ok := numberField(window, "reset_at", "resetAt"); ok && value > 0 {
		return int64(math.Round(value)), false, true
	}
	if value, ok := numberField(window, "reset_after_seconds", "resetAfterSeconds"); ok && value > 0 {
		return now.Unix() + int64(math.Round(value)), true, true
	}
	return 0, false, false
}

func isMonthlyWindow(seconds float64) bool {
	return seconds >= 2419200 && seconds <= 2678400
}

func normalizeManagementProvider(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if normalized == "chatgpt" || normalized == "chatgpt-subscription" {
		return "codex"
	}
	return normalized
}

func normalizePlanType(planType string) string {
	normalized := strings.ToLower(strings.TrimSpace(planType))
	if normalized == "" || normalized == "0" {
		return ""
	}
	return normalized
}

func findCodexAccountID(raw map[string]any) string {
	if value := firstString(raw, "account_id", "accountId", "chatgpt_account_id", "chatgptAccountId"); value != "" {
		return value
	}
	for _, source := range candidateCodexObjects(raw) {
		if value := firstString(source, "account_id", "accountId", "chatgpt_account_id", "chatgptAccountId"); value != "" {
			return value
		}
	}
	return ""
}

func findCodexPlanType(raw map[string]any) string {
	if value := normalizePlanType(firstString(raw, "plan_type", "planType")); value != "" {
		return value
	}
	for _, source := range candidateCodexObjects(raw) {
		if value := normalizePlanType(firstString(source, "plan_type", "planType")); value != "" {
			return value
		}
	}
	return ""
}

func findCodexSubscriptionActiveUntil(raw map[string]any) string {
	for _, key := range []string{"subscription_active_until", "subscriptionActiveUntil", "chatgpt_subscription_active_until", "expires_at", "expiresAt"} {
		if value := firstString(raw, key); value != "" {
			return value
		}
	}
	for _, source := range candidateCodexObjects(raw) {
		for _, key := range []string{"subscription_active_until", "subscriptionActiveUntil", "chatgpt_subscription_active_until", "chatgptSubscriptionActiveUntil", "expires_at", "expiresAt", "active_until", "activeUntil"} {
			if value := firstString(source, key); value != "" {
				return value
			}
		}
		if subscription := objectFromAny(source["subscription"]); subscription != nil {
			if value := firstString(subscription, "active_until", "activeUntil"); value != "" {
				return value
			}
		}
	}
	return ""
}

func candidateCodexObjects(raw map[string]any) []map[string]any {
	objects := make([]map[string]any, 0, 8)
	addObject := func(value any) {
		if obj := objectFromAny(value); obj != nil {
			objects = append(objects, obj)
			if authClaims := objectFromAny(obj[codexJWTClaimPath]); authClaims != nil {
				objects = append(objects, authClaims)
			}
		}
		if token := stringFromAny(value); token != "" {
			if claims, ok := decodeJWTClaims(token); ok {
				objects = append(objects, claims)
				if authClaims := objectFromAny(claims[codexJWTClaimPath]); authClaims != nil {
					objects = append(objects, authClaims)
				}
			}
		}
	}
	addObject(raw["id_token"])
	if metadata := objectFromAny(raw["metadata"]); metadata != nil {
		objects = append(objects, metadata)
		addObject(metadata["id_token"])
	}
	if attributes := objectFromAny(raw["attributes"]); attributes != nil {
		objects = append(objects, attributes)
		addObject(attributes["id_token"])
	}
	if subscription := objectFromAny(raw["subscription"]); subscription != nil {
		objects = append(objects, subscription)
	}
	return objects
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringFromAny(raw[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstBool(raw map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := boolValue(raw[key])
		if ok {
			return value
		}
	}
	return false
}

func objectField(raw map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value := objectFromAny(raw[key]); value != nil {
			return value
		}
	}
	return nil
}

func numberField(raw map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := numberValue(raw[key]); ok {
			return value, true
		}
	}
	return 0, false
}

func boolField(raw map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if value, ok := boolValue(raw[key]); ok {
			return value, true
		}
	}
	return false, false
}

func objectFromAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return ""
		}
		if math.Trunc(typed) == typed {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case float32:
		value := float64(typed)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}
		return value, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		trimmed := strings.TrimSpace(strings.TrimSuffix(typed, "%"))
		if trimmed == "" {
			return 0, false
		}
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return value, true
	default:
		return 0, false
	}
}

func boolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		}
	case float64:
		return typed != 0, true
	case int:
		return typed != 0, true
	}
	return false, false
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
