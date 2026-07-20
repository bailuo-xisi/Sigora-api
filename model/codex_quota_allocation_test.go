package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestMigrateCodexQuotaUsageBucketIndexPreservesNewConflictTarget(t *testing.T) {
	originalDB := DB
	originalLogDB := LOG_DB
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	DB = db
	LOG_DB = db

	require.NoError(t, db.AutoMigrate(&CodexUsageBucket{}))
	require.NoError(t, db.Migrator().DropIndex(&CodexUsageBucket{}, "idx_codex_usage_bucket_account_minute"))
	require.NoError(t, db.Exec(`
		CREATE UNIQUE INDEX idx_codex_usage_bucket
		ON codex_usage_buckets(user_id, bucket_minute)
	`).Error)
	require.NoError(t, MigrateCodexQuotaUsageBucketIndex())

	migrator := db.Migrator()
	require.True(t, migrator.HasIndex(&CodexUsageBucket{}, "idx_codex_usage_bucket_account_minute"))
	require.False(t, migrator.HasIndex(&CodexUsageBucket{}, "idx_codex_usage_bucket"))

	minute := int64(1_800_000_000)
	require.NoError(t, db.Create(&CodexUsageBucket{
		UserId:       7,
		AccountHash:  "account-a",
		CycleId:      1,
		BucketMinute: minute,
		Weight:       1,
	}).Error)
	require.NoError(t, db.Create(&CodexUsageBucket{
		UserId:       7,
		AccountHash:  "account-b",
		CycleId:      2,
		BucketMinute: minute,
		Weight:       1,
	}).Error)
}
