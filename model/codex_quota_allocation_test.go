package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestUpdateCodexQuotaPolicyAllowsSiteWideOversubscription(t *testing.T) {
	DB.Exec("DELETE FROM users")
	DB.Exec("DELETE FROM codex_quota_sync_states")
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users")
		DB.Exec("DELETE FROM codex_quota_sync_states")
	})

	first := User{Username: "codex-share-a", Password: "password", Role: common.RoleCommonUser, AffCode: "codex-share-a"}
	second := User{Username: "codex-share-b", Password: "password", Role: common.RoleCommonUser, AffCode: "codex-share-b"}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)

	require.NoError(t, UpdateCodexQuotaPolicy(first.Id, 6000, 0))
	require.NoError(t, UpdateCodexQuotaPolicy(second.Id, 4001, 0))

	share, bonus, _, err := GetCodexQuotaPolicy(second.Id)
	require.NoError(t, err)
	require.Equal(t, 4001, share)
	require.Zero(t, bonus)
	require.Error(t, UpdateCodexQuotaPolicy(second.Id, 10001, 0))
}

func TestUpdateCodexQuotaPolicyRejectsPrivilegedUsers(t *testing.T) {
	DB.Exec("DELETE FROM users")
	DB.Exec("DELETE FROM codex_quota_sync_states")
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users")
		DB.Exec("DELETE FROM codex_quota_sync_states")
	})

	admin := User{Username: "codex-share-admin", Password: "password", Role: common.RoleAdminUser, AffCode: "codex-share-admin"}
	require.NoError(t, DB.Create(&admin).Error)
	require.Error(t, UpdateCodexQuotaPolicy(admin.Id, 1000, 0))
}
