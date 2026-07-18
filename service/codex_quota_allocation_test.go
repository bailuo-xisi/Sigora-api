package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestPercentToCodexUnits(t *testing.T) {
	assert.Equal(t, int64(0), percentToCodexUnits(-1))
	assert.Equal(t, int64(125_000), percentToCodexUnits(12.5))
	assert.Equal(t, int64(1_000_000), percentToCodexUnits(120))
}

func TestProportionalCodexSharesPreservesTotal(t *testing.T) {
	weights := []codexWeight{
		{UserId: 3, Weight: 1, Role: common.RoleCommonUser},
		{UserId: 1, Weight: 1, Role: common.RoleCommonUser},
		{UserId: 2, Weight: 1, Role: common.RoleCommonUser},
	}
	shares := proportionalCodexShares(10, weights, 3)
	assert.Equal(t, []int64{3, 4, 3}, shares)
	assert.Equal(t, int64(10), shares[0]+shares[1]+shares[2])
}

func TestProportionalCodexSharesUsesWeights(t *testing.T) {
	weights := []codexWeight{
		{UserId: 1, Weight: 3, Role: common.RoleCommonUser},
		{UserId: 2, Weight: 1, Role: common.RoleCommonUser},
	}
	assert.Equal(t, []int64{75, 25}, proportionalCodexShares(100, weights, 4))
}
