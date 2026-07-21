package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetExternalCodexQuotas(c *gin.Context) {
	forceRefresh := c.Query("refresh") == "1"
	if forceRefresh {
		// A manual dashboard refresh must not be served the 60-second snapshot.
		// Also prevent an intermediary from retaining this explicitly fresh response.
		c.Header("Cache-Control", "no-store")
	}

	var (
		summary *service.ManagementCodexQuotas
		err     error
	)
	if forceRefresh {
		summary, err = service.RefreshManagementCodexQuotas(c.Request.Context())
	} else {
		summary, err = service.GetManagementCodexQuotas(c.Request.Context())
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if c.GetInt("role") < common.RoleAdminUser {
		summary = sanitizeExternalCodexQuotas(summary)
	}
	common.ApiSuccess(c, summary)
}

func sanitizeExternalCodexQuotas(summary *service.ManagementCodexQuotas) *service.ManagementCodexQuotas {
	if summary == nil {
		return nil
	}
	sanitized := *summary
	sanitized.Items = make([]service.ManagementCodexQuotaItem, len(summary.Items))
	for index, item := range summary.Items {
		item.Name = ""
		item.AuthIndex = ""
		sanitized.Items[index] = item
	}
	return &sanitized
}
