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
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
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

func TestSyncCodexQuotaAllocationUsesOutboundWeightBeforeResponseCompletes(t *testing.T) {
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
		&model.CodexQuotaUnattributedUsage{},
		&model.CodexQuotaSyncState{},
	))

	user := model.User{Username: "codex-quota-user", Role: common.RoleCommonUser, CodexQuotaShareBps: 2500}
	require.NoError(t, db.Create(&user).Error)

	fixedNow := time.Unix(1_800_000_030, 0)
	nowValue := fixedNow
	codexQuotaNow = func() time.Time { return nowValue }
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
					"account_id": "acct-codex",
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
	var cycle model.CodexQuotaCycle
	require.NoError(t, db.First(&cycle).Error)
	accountHash := hashCodexAccountID("acct-codex")
	requestContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(requestContext, constant.ContextKeyCodexQuotaAccountHash, accountHash)
	common.SetContextKey(requestContext, constant.ContextKeyCodexQuotaCycleIds, []int64{cycle.Id})
	require.NoError(t, StartCodexQuotaUsageTracking(requestContext, user.Id, 0, 0, 100))
	pending, err := getCodexPendingUsage(user.Id, []model.CodexQuotaCycle{cycle}, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(100), pending.PendingWeight)
	assert.Equal(t, currentMinute, pending.PendingSince)
	usedPercent.Store(11)

	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))

	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(100_000), cycle.UpstreamUsedUnits, "current-minute delta must remain pending until its weight closes")

	nowValue = nowValue.Add(time.Minute)
	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(100_000), cycle.UpstreamUsedUnits, "in-flight traffic must not settle before it succeeds")
	require.NoError(t, FinalizeCodexQuotaUsageTracking(requestContext, user.Id, 0, 0, 100, 100))

	nowValue = nowValue.Add(time.Minute)
	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))
	var usage model.CodexUserCycleUsage
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&usage).Error)
	assert.Equal(t, int64(10_000), usage.UsedUnits)
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(110_000), cycle.UpstreamUsedUnits)

	nextCurrentMinute := nowValue.Unix() / 60 * 60
	require.NoError(t, db.Create(&model.CodexUsageBucket{
		UserId:       user.Id,
		AccountHash:  accountHash,
		CycleId:      cycle.Id,
		BucketMinute: nextCurrentMinute,
		Weight:       100,
	}).Error)
	usedPercent.Store(12)

	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(110_000), cycle.UpstreamUsedUnits, "open-minute traffic must remain pending")

	nowValue = nowValue.Add(time.Minute)
	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(120_000), cycle.UpstreamUsedUnits, "closed traffic must settle against its own cycle")
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
		&model.CodexQuotaUnattributedUsage{},
		&model.CodexQuotaSyncState{},
	))

	user := model.User{Username: "codex-derived-reset-user", Role: common.RoleCommonUser, CodexQuotaShareBps: 2500}
	require.NoError(t, db.Create(&user).Error)

	fixedNow := time.Now().Truncate(time.Minute).Add(30 * time.Second)
	nowValue := fixedNow
	codexQuotaNow = func() time.Time { return nowValue }
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
					"account_id": "acct-codex",
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

	var initialCycle model.CodexQuotaCycle
	require.NoError(t, db.First(&initialCycle).Error)
	currentMinute := fixedNow.Unix() / 60 * 60
	require.NoError(t, db.Create(&model.CodexUsageBucket{
		UserId:       user.Id,
		AccountHash:  hashCodexAccountID("acct-codex"),
		CycleId:      initialCycle.Id,
		BucketMinute: currentMinute,
		Weight:       100,
	}).Error)
	usedPercent.Store(11)
	// The relative reset value changes on every upstream poll. It must not
	// create a new baseline instead of attributing the observed delta.
	nowValue = nowValue.Add(time.Minute)
	resetAfterSeconds.Store(604800)

	require.NoError(t, SyncCodexQuotaAllocation(context.Background()))

	var cycleCount int64
	require.NoError(t, db.Model(&model.CodexQuotaCycle{}).Count(&cycleCount).Error)
	assert.Equal(t, int64(1), cycleCount)

	var cycle model.CodexQuotaCycle
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(110_000), cycle.UpstreamUsedUnits)
	assert.Greater(t, cycle.ResetAt, fixedNow.Unix(), "derived reset must remain in the future")

	var usage model.CodexUserCycleUsage
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&usage).Error)
	assert.Equal(t, int64(10_000), usage.UsedUnits)
}

func TestSettleCodexQuotaCycleKeepsPrivilegedWeightOutOfCommonUserCharges(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
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
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.CodexQuotaCycle{},
		&model.CodexUserCycleUsage{},
		&model.CodexUsageBucket{},
	))

	trackedUser := model.User{Username: "tracked-user", Role: common.RoleCommonUser, AffCode: "tracked-user"}
	otherUser := model.User{Username: "other-user", Role: common.RoleCommonUser, AffCode: "other-user"}
	adminUser := model.User{Username: "quota-admin", Role: common.RoleAdminUser, AffCode: "quota-admin"}
	require.NoError(t, db.Create(&trackedUser).Error)
	require.NoError(t, db.Create(&otherUser).Error)
	require.NoError(t, db.Create(&adminUser).Error)

	cycleA := model.CodexQuotaCycle{
		CredentialHash:    hashCodexCredential("auth-a"),
		AccountHash:       hashCodexAccountID("acct-a"),
		WindowType:        "weekly",
		ResetAt:           10_000,
		Generation:        1,
		CapacityUnits:     model.CodexQuotaFullWindowUnits,
		UpstreamUsedUnits: 100_000,
		LastBucketMinute:  60,
		LastSeenAt:        120,
	}
	cycleB := model.CodexQuotaCycle{
		CredentialHash:    hashCodexCredential("auth-b"),
		AccountHash:       hashCodexAccountID("acct-b"),
		WindowType:        "weekly",
		ResetAt:           10_000,
		Generation:        1,
		CapacityUnits:     model.CodexQuotaFullWindowUnits,
		UpstreamUsedUnits: 100_000,
		LastBucketMinute:  60,
		LastSeenAt:        120,
	}
	require.NoError(t, db.Create(&cycleA).Error)
	require.NoError(t, db.Create(&cycleB).Error)
	require.NoError(t, db.Create([]model.CodexUsageBucket{
		{UserId: trackedUser.Id, AccountHash: cycleA.AccountHash, CycleId: cycleA.Id, BucketMinute: 120, Weight: 1},
		{UserId: otherUser.Id, AccountHash: cycleB.AccountHash, CycleId: cycleB.Id, BucketMinute: 120, Weight: 100},
		{UserId: adminUser.Id, AccountHash: cycleA.AccountHash, CycleId: cycleA.Id, BucketMinute: 120, Weight: 999},
	}).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return settleCodexQuotaCycle(tx, codexCycleObservation{
			Cycle:             cycleA,
			UpstreamUsedUnits: 110_000,
			ResetAt:           cycleA.ResetAt,
		}, 180, 120)
	}))

	var usage model.CodexUserCycleUsage
	require.NoError(t, db.Where("user_id = ? AND cycle_id = ?", trackedUser.Id, cycleA.Id).First(&usage).Error)
	assert.Equal(t, int64(10), usage.UsedUnits)

	var otherUsageCount int64
	require.NoError(t, db.Model(&model.CodexUserCycleUsage{}).
		Where("user_id = ?", otherUser.Id).
		Count(&otherUsageCount).Error)
	assert.Zero(t, otherUsageCount)

	var updatedCycleB model.CodexQuotaCycle
	require.NoError(t, db.First(&updatedCycleB, cycleB.Id).Error)
	assert.Equal(t, int64(100_000), updatedCycleB.UpstreamUsedUnits)
}

func TestSettleCodexQuotaCycleDoesNotChargeFailedProvisionalRequest(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
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
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.CodexQuotaCycle{},
		&model.CodexUserCycleUsage{},
		&model.CodexUsageBucket{},
	))

	user := model.User{Username: "failed-tracking-user", Role: common.RoleCommonUser, AffCode: "failed-tracking-user"}
	require.NoError(t, db.Create(&user).Error)
	cycle := model.CodexQuotaCycle{
		CredentialHash:    hashCodexCredential("auth-failed-tracking"),
		AccountHash:       hashCodexAccountID("acct-failed-tracking"),
		WindowType:        "weekly",
		ResetAt:           10_000,
		Generation:        1,
		CapacityUnits:     model.CodexQuotaFullWindowUnits,
		UpstreamUsedUnits: 100_000,
		LastBucketMinute:  60,
		LastSeenAt:        120,
	}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, model.ReserveCodexUsageWeight(
		user.Id,
		cycle.AccountHash,
		[]int64{cycle.Id},
		1,
		0,
		120,
		100,
	))

	observation := codexCycleObservation{Cycle: cycle, UpstreamUsedUnits: 110_000, ResetAt: cycle.ResetAt}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return settleCodexQuotaCycle(tx, observation, 180, 120)
	}))
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(100_000), cycle.UpstreamUsedUnits)

	require.NoError(t, model.CancelCodexUsageWeight(
		user.Id,
		cycle.AccountHash,
		[]int64{cycle.Id},
		120,
		100,
	))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return settleCodexQuotaCycle(tx, observation, 180, 120)
	}))

	var usageCount int64
	require.NoError(t, db.Model(&model.CodexUserCycleUsage{}).Where("user_id = ?", user.Id).Count(&usageCount).Error)
	assert.Zero(t, usageCount)
	require.NoError(t, db.First(&cycle).Error)
	assert.Equal(t, int64(110_000), cycle.UpstreamUsedUnits)
}

func TestObserveCodexQuotaCycleClaimsLegacyCycleWithoutResettingUsage(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
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
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.CodexQuotaCycle{},
		&model.CodexUserCycleUsage{},
		&model.CodexUsageBucket{},
		&model.CodexQuotaUnattributedUsage{},
		&model.CodexQuotaSyncState{},
	))

	now := time.Now().Unix()
	closedMinute := now/60*60 - 60
	legacyWatermark := closedMinute - 60
	resetAt := now + int64((7 * 24 * time.Hour).Seconds())
	user := model.User{Username: "legacy-cycle-user", Role: common.RoleCommonUser, AffCode: "legacy-cycle-user", CodexQuotaShareBps: 2500}
	require.NoError(t, db.Create(&user).Error)
	legacy := model.CodexQuotaCycle{
		CredentialHash:    hashCodexCredential("auth-legacy"),
		WindowType:        "weekly",
		ResetAt:           resetAt,
		Generation:        1,
		CapacityUnits:     model.CodexQuotaFullWindowUnits,
		UpstreamUsedUnits: 100_000,
		LastSeenAt:        now,
	}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.Create(&model.CodexUserCycleUsage{
		UserId:    user.Id,
		CycleId:   legacy.Id,
		UsedUnits: 123_456,
	}).Error)
	require.NoError(t, db.Create(&model.CodexQuotaSyncState{
		Id:               1,
		LastSuccessAt:    now,
		LastBucketMinute: legacyWatermark,
		IncludedCount:    1,
	}).Error)

	window := ManagementCodexQuotaWindow{ID: "weekly", ResetAt: &resetAt}
	observation, err := observeCodexQuotaCycle(
		db,
		hashCodexCredential("auth-legacy"),
		hashCodexAccountID("acct-legacy"),
		window,
		100_000,
		now,
		closedMinute,
		legacyWatermark,
	)
	require.NoError(t, err)
	assert.Nil(t, observation)

	var claimed model.CodexQuotaCycle
	require.NoError(t, db.First(&claimed, legacy.Id).Error)
	assert.Equal(t, hashCodexAccountID("acct-legacy"), claimed.AccountHash)
	assert.Equal(t, legacyWatermark, claimed.LastBucketMinute)
	assert.Equal(t, int64(100_000), claimed.UpstreamUsedUnits)

	var cycleCount int64
	require.NoError(t, db.Model(&model.CodexQuotaCycle{}).Count(&cycleCount).Error)
	assert.Equal(t, int64(1), cycleCount)

	summary, err := GetCodexQuotaAllocationSummary(user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(123_456), summary.SettledUsedUnits)
}

func TestCleanupSettledCodexUsageBucketsArchivesUnsettledWeights(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
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
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.CodexQuotaCycle{}, &model.CodexUsageBucket{}, &model.CodexQuotaUnattributedUsage{}))

	oldMinute := time.Now().Add(-8*24*time.Hour).Unix() / 60 * 60
	cycle := model.CodexQuotaCycle{
		CredentialHash:   hashCodexCredential("auth-cleanup"),
		AccountHash:      hashCodexAccountID("acct-cleanup"),
		WindowType:       "weekly",
		ResetAt:          time.Now().Add(time.Hour).Unix(),
		Generation:       1,
		CapacityUnits:    model.CodexQuotaFullWindowUnits,
		LastBucketMinute: oldMinute - 60,
		LastSeenAt:       time.Now().Unix(),
	}
	require.NoError(t, db.Create(&cycle).Error)
	bucket := model.CodexUsageBucket{
		UserId:       1,
		AccountHash:  cycle.AccountHash,
		CycleId:      cycle.Id,
		BucketMinute: oldMinute,
		Weight:       10,
	}
	require.NoError(t, db.Create(&bucket).Error)

	require.NoError(t, cleanupSettledCodexUsageBuckets(db, oldMinute+60, 0))
	var count int64
	require.NoError(t, db.Model(&model.CodexUsageBucket{}).Count(&count).Error)
	assert.Zero(t, count)

	var archived model.CodexQuotaUnattributedUsage
	require.NoError(t, db.Where("user_id = ?", bucket.UserId).First(&archived).Error)
	assert.Equal(t, bucket.AccountHash, archived.AccountHash)
	assert.Equal(t, int64(10), archived.Weight)
	assert.Equal(t, oldMinute, archived.Since)
}

func TestExpireCodexPendingUsageWeightsArchivesLateTracking(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
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
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.CodexUsageBucket{}, &model.CodexQuotaUnattributedUsage{}))

	accountHash := hashCodexAccountID("acct-expired-pending")
	require.NoError(t, model.ReserveCodexUsageWeight(7, accountHash, []int64{1}, 1, 0, 60, 100))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return expireCodexPendingUsageWeights(tx, 120)
	}))

	var bucketCount int64
	require.NoError(t, db.Model(&model.CodexUsageBucket{}).Count(&bucketCount).Error)
	assert.Zero(t, bucketCount)

	var archived model.CodexQuotaUnattributedUsage
	require.NoError(t, db.Where("user_id = ?", 7).First(&archived).Error)
	assert.Equal(t, int64(100), archived.Weight)
	require.Error(t, model.FinalizeCodexUsageWeight(7, accountHash, []int64{1}, 60, 100, 100))
}

func TestCheckCodexQuotaAccessBindsPrivilegedUsageForSettlement(t *testing.T) {
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalEnabled := operation_setting.CodexQuotaAllocationEnabled
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		operation_setting.CodexQuotaAllocationEnabled = originalEnabled
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
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CodexQuotaCycle{}, &model.CodexUsageBucket{}))

	admin := model.User{Username: "codex-admin", Role: common.RoleAdminUser}
	require.NoError(t, db.Create(&admin).Error)
	now := time.Now().Unix()
	accountHash := hashCodexAccountID("acct-admin")
	cycle := model.CodexQuotaCycle{
		CredentialHash:   hashCodexCredential("auth-admin"),
		AccountHash:      accountHash,
		WindowType:       "weekly",
		ResetAt:          now + int64(time.Hour.Seconds()),
		Generation:       1,
		CapacityUnits:    model.CodexQuotaFullWindowUnits,
		LastBucketMinute: now / 60 * 60,
		LastSeenAt:       now,
	}
	require.NoError(t, db.Create(&cycle).Error)

	operation_setting.CodexQuotaAllocationEnabled = true
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(context, constant.ContextKeyChannelKey, `{"account_id":"acct-admin"}`)
	require.Nil(t, CheckCodexQuotaAccess(context, admin.Id))

	cycleIds, ok := common.GetContextKeyType[[]int64](context, constant.ContextKeyCodexQuotaCycleIds)
	require.True(t, ok)
	require.Equal(t, []int64{cycle.Id}, cycleIds)
	require.Equal(t, accountHash, common.GetContextKeyString(context, constant.ContextKeyCodexQuotaAccountHash))
	require.NoError(t, model.RecordCodexUsageWeight(admin.Id, accountHash, cycleIds, 1, 0, 1))

	var bucketCount int64
	require.NoError(t, db.Model(&model.CodexUsageBucket{}).Where("user_id = ?", admin.Id).Count(&bucketCount).Error)
	assert.Equal(t, int64(1), bucketCount)

	common.SetContextKey(context, constant.ContextKeyChannelKey, `{"account_id":"acct-unmanaged"}`)
	require.Nil(t, CheckCodexQuotaAccess(context, admin.Id))
	assert.Empty(t, common.GetContextKeyString(context, constant.ContextKeyCodexQuotaAccountHash))
	_, ok = common.GetContextKeyType[[]int64](context, constant.ContextKeyCodexQuotaCycleIds)
	assert.False(t, ok)
}
