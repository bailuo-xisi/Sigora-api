package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestGetManagementCodexQuotasFetchesAllCodexFiles(t *testing.T) {
	resetManagementCodexQuotaCacheForTest()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-management-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/v0/management/auth-files":
			writeManagementCodexQuotaTestJSON(t, w, map[string]any{
				"files": []any{
					map[string]any{
						"name":       "codex-a.json",
						"provider":   "codex",
						"auth_index": "auth-a",
						"account_id": "acct-a",
						"plan_type":  "plus",
					},
					map[string]any{
						"name":       "codex-disabled.json",
						"provider":   "codex",
						"auth_index": "auth-disabled",
						"disabled":   true,
					},
					map[string]any{
						"name":       "claude.json",
						"provider":   "claude",
						"auth_index": "auth-claude",
					},
					map[string]any{
						"name":      "codex-b.json",
						"type":      "codex",
						"authIndex": "auth-b",
						"metadata": map[string]any{
							"id_token": map[string]any{
								"https://api.openai.com/auth": map[string]any{
									"chatgpt_account_id": "acct-b",
								},
								"plan_type": "team",
								"subscription": map[string]any{
									"active_until": "2026-07-17T17:05:52Z",
								},
							},
						},
					},
				},
			})
		case "/v0/management/api-call":
			var req managementAPICallRequest
			if err := common.DecodeJson(r.Body, &req); err != nil {
				t.Fatalf("decode api-call request: %v", err)
			}
			if req.Method != http.MethodGet || req.URL != managementCodexWhamUsageURL {
				t.Fatalf("unexpected api-call request: %+v", req)
			}
			if req.Header["Authorization"] != "Bearer $TOKEN$" {
				t.Fatalf("missing token placeholder header: %+v", req.Header)
			}
			if req.AuthIndex == "auth-a" && req.Header["Chatgpt-Account-Id"] != "acct-a" {
				t.Fatalf("missing account id header: %+v", req.Header)
			}

			body := map[string]any{
				"plan_type": planTypeForQuotaTest(req.AuthIndex),
				"rate_limit_reset_credits": map[string]any{
					"available_count": 2,
				},
				"rate_limit": map[string]any{
					"primary_window": map[string]any{
						"limit_window_seconds": 18000,
						"used_percent":         usedPercentForQuotaTest(req.AuthIndex, "primary"),
						"reset_at":             1781756100,
					},
					"secondary_window": map[string]any{
						"limitWindowSeconds": 604800,
						"usedPercent":        usedPercentForQuotaTest(req.AuthIndex, "secondary"),
						"resetAfterSeconds":  3600,
					},
				},
			}
			if req.AuthIndex == "auth-b" {
				bodyText, err := common.Marshal(body)
				if err != nil {
					t.Fatalf("marshal body text: %v", err)
				}
				writeManagementCodexQuotaTestJSON(t, w, map[string]any{
					"statusCode": http.StatusOK,
					"bodyText":   string(bodyText),
				})
				return
			}
			writeManagementCodexQuotaTestJSON(t, w, map[string]any{
				"status_code": http.StatusOK,
				"body":        body,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("SIGORA_QUOTA_MANAGEMENT_BASE_URL", server.URL)
	t.Setenv("SIGORA_QUOTA_MANAGEMENT_KEY", "test-management-key")
	t.Setenv("SIGORA_QUOTA_CONCURRENCY", "2")

	result, err := GetManagementCodexQuotas(context.Background())
	if err != nil {
		t.Fatalf("GetManagementCodexQuotas returned error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 codex quota items, got %d", len(result.Items))
	}

	first := result.Items[0]
	if first.Name != "codex-a.json" || first.PlanType != "plus" {
		t.Fatalf("unexpected first item: %+v", first)
	}
	if first.AccountHash != hashCodexAccountID("acct-a") {
		t.Fatalf("expected first account hash to be retained")
	}
	if len(first.Windows) != 2 {
		t.Fatalf("expected 2 windows, got %+v", first.Windows)
	}
	assertQuotaWindow(t, first.Windows[0], "five-hour", 86, 14)
	assertQuotaWindow(t, first.Windows[1], "weekly", 13, 87)

	second := result.Items[1]
	if second.Name != "codex-b.json" || second.PlanType != "team" {
		t.Fatalf("unexpected second item: %+v", second)
	}
	if second.AccountHash != hashCodexAccountID("acct-b") {
		t.Fatalf("expected second account hash to be retained")
	}
	if second.RateLimitResetCreditsAvailableCount == nil || *second.RateLimitResetCreditsAvailableCount != 2 {
		t.Fatalf("expected reset credits, got %+v", second)
	}
	if !strings.Contains(second.SubscriptionActiveUntil, "2026-07-17") {
		t.Fatalf("expected subscription timestamp, got %q", second.SubscriptionActiveUntil)
	}
	assertQuotaWindow(t, second.Windows[0], "five-hour", 40, 60)
	assertQuotaWindow(t, second.Windows[1], "weekly", 25, 75)
}

func TestGetManagementCodexQuotasRequiresManagementKey(t *testing.T) {
	resetManagementCodexQuotaCacheForTest()
	t.Setenv("SIGORA_QUOTA_MANAGEMENT_KEY", "")
	t.Setenv("CLI_PROXY_MANAGEMENT_KEY", "")
	t.Setenv("MANAGEMENT_KEY", "")

	result, err := GetManagementCodexQuotas(context.Background())
	if err != nil {
		t.Fatalf("expected missing key to return an unconfigured result, got %v", err)
	}
	if result.Configured || len(result.Items) != 0 {
		t.Fatalf("expected unconfigured empty result, got %+v", result)
	}
}

func writeManagementCodexQuotaTestJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	data, err := common.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func planTypeForQuotaTest(authIndex string) string {
	if authIndex == "auth-b" {
		return "team"
	}
	return "plus"
}

func usedPercentForQuotaTest(authIndex string, window string) float64 {
	if authIndex == "auth-b" {
		if window == "primary" {
			return 40
		}
		return 25
	}
	if window == "primary" {
		return 86
	}
	return 13
}

func assertQuotaWindow(t *testing.T, window ManagementCodexQuotaWindow, id string, used float64, remaining float64) {
	t.Helper()
	if window.ID != id {
		t.Fatalf("expected window id %q, got %+v", id, window)
	}
	if window.UsedPercent == nil || *window.UsedPercent != used {
		t.Fatalf("expected used %.0f, got %+v", used, window)
	}
	if window.RemainingPercent == nil || *window.RemainingPercent != remaining {
		t.Fatalf("expected remaining %.0f, got %+v", remaining, window)
	}
	if window.ResetAt == nil || *window.ResetAt <= 0 {
		t.Fatalf("expected reset timestamp, got %+v", window)
	}
}

func resetManagementCodexQuotaCacheForTest() {
	managementCodexQuotaCache.Lock()
	defer managementCodexQuotaCache.Unlock()

	managementCodexQuotaCache.key = ""
	managementCodexQuotaCache.expiresAt = managementCodexQuotaCache.expiresAt.Add(-managementCodexQuotaCacheTTL)
	managementCodexQuotaCache.data = nil
}
