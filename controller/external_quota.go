package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetExternalCodexQuotas(c *gin.Context) {
	getExternalQuotas(c, false)
}

func GetExternalClaudeQuotas(c *gin.Context) {
	getExternalQuotas(c, true)
}

func getExternalQuotas(c *gin.Context, claude bool) {
	forceRefresh := c.Query("refresh") == "1"
	if forceRefresh {
		// A manual dashboard refresh must not be served the 60-second snapshot.
		// Also prevent an intermediary from retaining this explicitly fresh response.
		c.Header("Cache-Control", "no-store")
	}

	var (
		summary *service.ManagementQuotas
		err     error
	)
	if claude {
		summary, err = service.GetManagementClaudeQuotas(c.Request.Context(), forceRefresh)
	} else if forceRefresh {
		summary, err = service.RefreshManagementCodexQuotas(c.Request.Context())
	} else {
		summary, err = service.GetManagementCodexQuotas(c.Request.Context())
	}
	if err != nil {
		if claude {
			common.ApiErrorMsg(c, "failed to fetch Claude quotas")
		} else {
			common.ApiError(c, err)
		}
		return
	}
	if c.GetInt("role") < common.RoleAdminUser {
		summary = sanitizeExternalQuotas(summary)
	}
	common.ApiSuccess(c, summary)
}

func sanitizeExternalQuotas(summary *service.ManagementQuotas) *service.ManagementQuotas {
	sanitized := *summary
	sanitized.Items = make([]service.ManagementQuotaItem, len(summary.Items))
	for index, item := range summary.Items {
		item.Name = ""
		item.AuthIndex = ""
		sanitized.Items[index] = item
	}
	return &sanitized
}
