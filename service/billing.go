package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
)

// numericQuotaCharge returns the amount that should be charged to the user's
// numeric wallet/statistics. Codex percentage allocation is accounted for in
// the Codex quota pool instead, so it must not appear as a numeric charge.
func numericQuotaCharge(relayInfo *relaycommon.RelayInfo, quota int) int {
	if relayInfo != nil && relayInfo.UseCodexQuotaAllocation {
		return 0
	}
	return quota
}

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo != nil && relayInfo.UseCodexQuotaAllocation {
		relayInfo.FinalPreConsumedQuota = 0
		return nil
	}
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		return apiErr
	}
	relayInfo.Billing = session
	return nil
}

// RefundAndClearBillingSession releases a numeric reservation before a retry
// switches to percentage-based Codex allocation.
func RefundAndClearBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil {
		return
	}
	if relayInfo.Billing != nil {
		if session, ok := relayInfo.Billing.(*BillingSession); ok {
			if err := session.RefundSync(c); err != nil {
				logger.LogError(c, "同步返还数字额度失败: "+err.Error())
			}
		} else {
			relayInfo.Billing.Refund(c)
		}
	}
	relayInfo.Billing = nil
	relayInfo.FinalPreConsumedQuota = 0
	relayInfo.BillingSource = ""
	relayInfo.SubscriptionId = 0
	relayInfo.SubscriptionPreConsumed = 0
	relayInfo.SubscriptionPostDelta = 0
	relayInfo.SubscriptionPlanId = 0
	relayInfo.SubscriptionPlanTitle = ""
	relayInfo.SubscriptionAmountTotal = 0
	relayInfo.SubscriptionAmountUsedAfterPreConsume = 0
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if relayInfo != nil && relayInfo.UseCodexQuotaAllocation {
		return nil
	}
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		delta := actualQuota - preConsumed

		if delta > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费与实际消耗一致，无需调整：%s（按次计费）",
				logger.FormatQuota(actualQuota),
			))
		}

		if err := relayInfo.Billing.Settle(actualQuota); err != nil {
			return err
		}

		// 发送额度通知（订阅计费使用订阅剩余额度）
		if actualQuota != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			} else {
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}
		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		return PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
	}
	return nil
}
