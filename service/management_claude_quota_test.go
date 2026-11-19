package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetManagementClaudeQuotas(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-management-key", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v0/management/auth-files":
			assert.Equal(t, http.MethodGet, r.Method)
			fmt.Fprint(w, `{"files":[
				{"name":"b.json","type":"claude","authIndex":"expired"},
				{"name":"a.json","provider":"claude","auth_index":"working"},
				{"name":"disabled.json","provider":"claude","auth_index":"disabled","disabled":true},
				{"name":"codex.json","provider":"codex","auth_index":"codex"}
			]}`)
		case "/v0/management/api-call":
			calls.Add(1)
			assert.Equal(t, http.MethodPost, r.Method)
			var req managementAPICallRequest
			if !assert.NoError(t, common.DecodeJson(r.Body, &req)) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			assert.Equal(t, http.MethodGet, req.Method)
			assert.Equal(t, "https://api.anthropic.com/api/oauth/usage", req.URL)
			assert.Equal(t, "Bearer $TOKEN$", req.Header["Authorization"])
			assert.Equal(t, "oauth-2025-04-20", req.Header["anthropic-beta"])
			switch req.AuthIndex {
			case "working":
				fmt.Fprint(w, `{"status_code":200,"body":{
					"five_hour":{"utilization":42.5,"resets_at":"2026-10-09T18:00:00.123Z"},
					"seven_day":{"utilization":0,"resets_at":null},
					"seven_day_opus":null,
					"seven_day_sonnet":{"utilization":100,"resets_at":null}
				}}`)
			case "expired":
				fmt.Fprint(w, `{"status_code":401,"body":"upstream error with auth metadata"}`)
			default:
				t.Errorf("unexpected auth index: %q", req.AuthIndex)
				w.WriteHeader(http.StatusBadRequest)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("SIGORA_QUOTA_MANAGEMENT_BASE_URL", server.URL)
	t.Setenv("SIGORA_QUOTA_MANAGEMENT_KEY", "test-management-key")
	ctx := context.Background()

	result, err := GetManagementClaudeQuotas(ctx, false)
	require.NoError(t, err)
	require.True(t, result.Configured)
	require.Len(t, result.Items, 2)
	first := result.Items[0]
	assert.Equal(t, "a.json", first.Name)
	require.Len(t, first.Windows, 3)
	assertQuotaWindow(t, first.Windows[0], "five-hour", 42.5, 57.5)
	assert.Equal(t, ManagementQuotaWindow{
		ID: "weekly", UsedPercent: common.GetPointer(0.0), RemainingPercent: common.GetPointer(100.0),
	}, first.Windows[1])
	assert.Equal(t, ManagementQuotaWindow{
		ID: "weekly-sonnet", UsedPercent: common.GetPointer(100.0), RemainingPercent: common.GetPointer(0.0),
	}, first.Windows[2])
	require.NotNil(t, first.Windows[0].ResetAt)
	assert.Equal(t, time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC).Unix(), *first.Windows[0].ResetAt)
	assert.Equal(t, http.StatusUnauthorized, result.Items[1].ErrorStatus)
	assert.NotContains(t, result.Items[1].Error, "auth metadata")

	cached, err := GetManagementClaudeQuotas(ctx, false)
	require.NoError(t, err)
	assert.Same(t, result, cached)
	assert.Equal(t, int32(2), calls.Load())

	_, err = GetManagementClaudeQuotas(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, int32(4), calls.Load())
}
