package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type codexQuotaAllocationRequest struct {
	ShareBps int `json:"share_bps"`
}

type codexQuotaBonusRequest struct {
	Mode     string `json:"mode"`
	ValueBps int    `json:"value_bps"`
}

func UpdateUserCodexQuotaAllocation(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req codexQuotaAllocationRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	_, bonus, _, err := model.GetCodexQuotaPolicy(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateCodexQuotaPolicy(userId, req.ShareBps, bonus); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, userId, "user.codex_quota_share_update", map[string]interface{}{
		"share_bps": req.ShareBps,
	})
	common.ApiSuccess(c, gin.H{"share_bps": req.ShareBps, "bonus_bps": bonus})
}

func UpdateUserCodexQuotaBonus(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req codexQuotaBonusRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ValueBps < 0 || req.ValueBps > model.CodexQuotaMaxBps {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Codex quota bonus must be between 0 and 100%"})
		return
	}
	share, bonus, _, err := model.GetCodexQuotaPolicy(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	newBonus := bonus
	switch req.Mode {
	case "add":
		newBonus += req.ValueBps
	case "subtract":
		newBonus -= req.ValueBps
	case "override":
		newBonus = req.ValueBps
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid bonus adjustment mode"})
		return
	}
	if err := model.UpdateCodexQuotaPolicy(userId, share, newBonus); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, userId, "user.codex_quota_bonus_update", map[string]interface{}{
		"mode": req.Mode, "from_bps": bonus, "to_bps": newBonus,
	})
	common.ApiSuccess(c, gin.H{"share_bps": share, "bonus_bps": newBonus})
}

func GetSelfCodexQuotaAllocation(c *gin.Context) {
	summary, err := service.GetCodexQuotaAllocationSummary(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func GetCodexQuotaPool(c *gin.Context) {
	summary, err := service.GetCodexQuotaPoolSummary()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}
