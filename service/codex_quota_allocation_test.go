package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestCodexQuotaPoolAvailabilityRequiresUsableCycle(t *testing.T) {
	now := time.Now().Unix()
	pool := CodexQuotaPoolSummary{PoolCapacityUnits: model.CodexQuotaFullWindowUnits}
	cycles := []model.CodexQuotaCycle{{Id: 1}}
	state := model.CodexQuotaSyncState{LastSuccessAt: now, IncludedCount: 1}

	assert.True(t, isCodexQuotaPoolAvailable(pool, cycles, state, now))
	assert.False(t, isCodexQuotaPoolAvailable(pool, cycles, model.CodexQuotaSyncState{LastSuccessAt: now}, now))
	assert.False(t, isCodexQuotaPoolAvailable(CodexQuotaPoolSummary{}, cycles, state, now))
}

func TestSyncCodexQuotaAllocationDefersDeltaUntilCurrentMinuteWeightCloses(t *testing.T) {
	resetManagementCodexQuotaCacheForTest()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalNow := codexQuotaNow
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		codexQuotaNow = originalNow
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		common.RedisEnabled = originalRedisEnabled
		resetManagementCodexQuotaCacheForTest()
	})

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.CodexQuotaCycle{},
		&model.CodexUserCycleUsage{},
		&model.CodexUsageBucket{},
		&model.CodexQuotaSyncState{},
	))

	user := model.User{Username: "codex-quota-user", Role: common.RoleCommonUser, CodexQuotaShareBps: 2500}
	require.NoError(t, db.Create(&user).Error)

	fixedNow := time.Unix(1_800_000_030, 0)
	codexQuotaNow = func() time.Time { return fixedNow }
	resetAt := fixedNow.Add(7 * 24 * time.Hour).Unix()
	var usedPercent atomic.Int64
	usedPercent.Store(10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			writeManagementCodexQuotaTestJSON(t, w, map[string]any{
				"files": []any{map[string]any{
					"name":       "codex.json",
					"provider":   "codex",
					"auth_index": "auth-codex",
				}},
			})
		case "/v0/management/api-call":
			writeManagementCodexQuotaTestJSON(t, w, map[string]any{
				"status_code": http.StatusOK,
				"body": map[string]any{
					"rate_limit": map[string]any{
						"secondary_window": map[string]any{
							"limit_window_seconds": 604800,
							"used_percent":         usedPercent.Load(),
							"reset_at":             resetAt,
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("SIGORA_QUOTA_MANAGEMENT_BASE_URL", server.URL)
	t.Setenv("SIGORA_QUOTA_MANAGEMENT_KEY", "test-management-key")

	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))

	currentMinute := fixedNow.Unix() / 60 * 60
	require.NoError(t, db.Create(&model.CodexUsageBucket{
		UserId:       user.Id,
		BucketMinute: currentMinute,
		Weight:       100,
	}).Error)
	closedMinute := currentMinute - 60
	require.NoError(t, db.Model(&model.CodexQuotaSyncState{}).Where("id = ?", 1).
		Update("last_bucket_minute", closedMinute-60).Error)
	usedPercent.Store(11)

	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))

	var cycle model.CodexQuotaCycle
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(100_000), cycle.UpstreamUsedUnits, "current-minute delta must remain pending until its weight closes")

	require.NoError(t, db.Model(&model.CodexUsageBucket{}).Where("user_id = ?", user.Id).
		Update("bucket_minute", closedMinute).Error)

	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))

	var usage model.CodexUserCycleUsage
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&usage).Error)
	assert.Equal(t, int64(10_000), usage.UsedUnits)

	require.NoError(t, db.Create(&model.CodexUsageBucket{
		UserId:       user.Id,
		BucketMinute: currentMinute,
		Weight:       100,
	}).Error)
	require.NoError(t, db.Model(&model.CodexQuotaSyncState{}).Where("id = ?", 1).
		Update("last_bucket_minute", closedMinute-60).Error)
	usedPercent.Store(12)

	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(120_000), cycle.UpstreamUsedUnits, "open-minute traffic must not block attribution for closed weights")
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&usage).Error)
	assert.Equal(t, int64(20_000), usage.UsedUnits)
}

func TestSyncCodexQuotaAllocationAttributesDerivedResetAtDelta(t *testing.T) {
	resetManagementCodexQuotaCacheForTest()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalNow := codexQuotaNow
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		codexQuotaNow = originalNow
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		common.RedisEnabled = originalRedisEnabled
		resetManagementCodexQuotaCacheForTest()
	})

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.CodexQuotaCycle{},
		&model.CodexUserCycleUsage{},
		&model.CodexUsageBucket{},
		&model.CodexQuotaSyncState{},
	))

	user := model.User{Username: "codex-derived-reset-user", Role: common.RoleCommonUser, CodexQuotaShareBps: 2500}
	require.NoError(t, db.Create(&user).Error)

	fixedNow := time.Now().Truncate(time.Minute).Add(30 * time.Second)
	codexQuotaNow = func() time.Time { return fixedNow }
	var usedPercent atomic.Int64
	usedPercent.Store(10)
	var resetAfterSeconds atomic.Int64
	resetAfterSeconds.Store(604800)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			writeManagementCodexQuotaTestJSON(t, w, map[string]any{
				"files": []any{map[string]any{
					"name":       "codex.json",
					"provider":   "codex",
					"auth_index": "auth-codex",
				}},
			})
		case "/v0/management/api-call":
			writeManagementCodexQuotaTestJSON(t, w, map[string]any{
				"status_code": http.StatusOK,
				"body": map[string]any{
					"rate_limit": map[string]any{
						"secondary_window": map[string]any{
							"limit_window_seconds": 604800,
							"used_percent":         usedPercent.Load(),
							"reset_after_seconds":  resetAfterSeconds.Load(),
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("SIGORA_QUOTA_MANAGEMENT_BASE_URL", server.URL)
	t.Setenv("SIGORA_QUOTA_MANAGEMENT_KEY", "test-management-key")

	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))

	closedMinute := fixedNow.Unix()/60*60 - 60
	require.NoError(t, db.Create(&model.CodexUsageBucket{
		UserId:       user.Id,
		BucketMinute: closedMinute,
		Weight:       100,
	}).Error)
	require.NoError(t, db.Model(&model.CodexQuotaSyncState{}).Where("id = ?", 1).
		Update("last_bucket_minute", closedMinute-60).Error)
	usedPercent.Store(11)
	// The relative reset value changes on every upstream poll. It must not
	// create a new baseline instead of attributing the observed delta.
	resetAfterSeconds.Store(604740)

	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))

	var cycleCount int64
	require.NoError(t, db.Model(&model.CodexQuotaCycle{}).Count(&cycleCount).Error)
	assert.Equal(t, int64(1), cycleCount)

	var cycle model.CodexQuotaCycle
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(110_000), cycle.UpstreamUsedUnits)

	var usage model.CodexUserCycleUsage
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&usage).Error)
	assert.Equal(t, int64(10_000), usage.UsedUnits)
}
