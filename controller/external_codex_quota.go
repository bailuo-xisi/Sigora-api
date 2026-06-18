package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetExternalCodexQuotas(c *gin.Context) {
	summary, err := service.GetManagementCodexQuotas(c.Request.Context())
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
