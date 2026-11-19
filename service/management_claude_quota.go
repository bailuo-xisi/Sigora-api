package service

import (
	"context"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type claudeUsageWindow struct {
	Utilization *float64   `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at"`
}

func GetManagementClaudeQuotas(ctx context.Context, force bool) (*ManagementQuotas, error) {
	return getManagementQuotas(ctx, "claude", force)
}

func fetchManagementClaudeQuotaItem(ctx context.Context, client *http.Client, cfg managementQuotaConfig, file managementQuotaAuthFile) ManagementQuotaItem {
	item := ManagementQuotaItem{
		Name:      file.Name,
		AuthIndex: file.AuthIndex,
		Windows:   []ManagementQuotaWindow{},
		UpdatedAt: time.Now().Unix(),
	}
	if file.AuthIndex == "" {
		item.Error = "claude auth file is missing auth_index"
		return item
	}

	payload := managementAPICallRequest{
		AuthIndex: file.AuthIndex,
		Method:    http.MethodGet,
		URL:       "https://api.anthropic.com/api/oauth/usage",
		Header: map[string]any{
			"Authorization":  "Bearer $TOKEN$",
			"Accept":         "application/json",
			"anthropic-beta": "oauth-2025-04-20",
		},
	}
	var usage struct {
		FiveHour       *claudeUsageWindow `json:"five_hour"`
		SevenDay       *claudeUsageWindow `json:"seven_day"`
		SevenDayOpus   *claudeUsageWindow `json:"seven_day_opus"`
		SevenDaySonnet *claudeUsageWindow `json:"seven_day_sonnet"`
	}
	if status, err := fetchManagementQuotaUsage(ctx, client, cfg, payload, &usage); err != nil {
		item.Error = "failed to fetch Claude quota"
		item.ErrorStatus = status
		return item
	}

	for _, entry := range []struct {
		id     string
		window *claudeUsageWindow
	}{
		{"five-hour", usage.FiveHour},
		{"weekly", usage.SevenDay},
		{"weekly-opus", usage.SevenDayOpus},
		{"weekly-sonnet", usage.SevenDaySonnet},
	} {
		if entry.window == nil || entry.window.Utilization == nil {
			continue
		}
		used := clampPercent(*entry.window.Utilization)
		window := ManagementQuotaWindow{
			ID:               entry.id,
			UsedPercent:      &used,
			RemainingPercent: common.GetPointer(100 - used),
		}
		if entry.window.ResetsAt != nil {
			window.ResetAt = common.GetPointer(entry.window.ResetsAt.Unix())
		}
		item.Windows = append(item.Windows, window)
	}
	return item
}
