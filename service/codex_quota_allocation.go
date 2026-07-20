package service

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	codexQuotaSyncInterval    = 60 * time.Second
	codexQuotaStaleAfter      = 5 * time.Minute
	codexQuotaBucketRetention = 7 * 24 * time.Hour
	codexQuotaPendingGrace    = 30 * time.Minute
)

type CodexQuotaAllocationSummary struct {
	Enabled            bool  `json:"enabled"`
	PoolAvailable      bool  `json:"pool_available"`
	ShareBps           int   `json:"share_bps"`
	BonusBps           int   `json:"bonus_bps"`
	EffectiveBps       int   `json:"effective_bps"`
	PoolCapacityUnits  int64 `json:"pool_capacity_units"`
	PoolUsedUnits      int64 `json:"pool_used_units"`
	PoolRemainUnits    int64 `json:"pool_remaining_units"`
	AllocatedUnits     int64 `json:"allocated_units"`
	UsedUnits          int64 `json:"used_units"`
	SettledUsedUnits   int64 `json:"settled_used_units"`
	RemainingUnits     int64 `json:"remaining_units"`
	PendingWeight      int64 `json:"pending_weight"`
	PendingSince       int64 `json:"pending_since"`
	UnattributedWeight int64 `json:"unattributed_weight"`
	UnattributedSince  int64 `json:"unattributed_since"`
	IncludedCount      int   `json:"included_count"`
	ExcludedCount      int   `json:"excluded_count"`
	LastUpdatedAt      int64 `json:"last_updated_at"`
	Stale              bool  `json:"stale"`
}

type CodexQuotaPoolSummary struct {
	Enabled           bool  `json:"enabled"`
	PoolAvailable     bool  `json:"pool_available"`
	PoolCapacityUnits int64 `json:"pool_capacity_units"`
	PoolUsedUnits     int64 `json:"pool_used_units"`
	PoolRemainUnits   int64 `json:"pool_remaining_units"`
	IncludedCount     int   `json:"included_count"`
	ExcludedCount     int   `json:"excluded_count"`
	AllocatedBps      int64 `json:"allocated_bps"`
	LastUpdatedAt     int64 `json:"last_updated_at"`
	Stale             bool  `json:"stale"`
}

type codexCycleDelta struct {
	CycleId int64
	Delta   int64
}

type codexCycleObservation struct {
	Cycle             model.CodexQuotaCycle
	UpstreamUsedUnits int64
	ResetAt           int64
}

type codexWeight struct {
	UserId int
	Weight int64
	Role   int
}

type codexUsageBucketKey struct {
	UserId       int
	AccountHash  string
	BucketMinute int64
}

type codexUsageArchiveKey struct {
	UserId      int
	AccountHash string
}

type codexUsageArchiveEntry struct {
	Weight int64
	Since  int64
}

type codexQuotaUsageTracking struct {
	AccountHash       string
	CycleIds          []int64
	BucketMinute      int64
	ProvisionalWeight int64
	Finalized         bool
}

var codexQuotaSyncLock sync.Mutex
var codexQuotaNow = time.Now

func StartCodexQuotaAllocationTask() {
	go func() {
		ticker := time.NewTicker(codexQuotaSyncInterval)
		defer ticker.Stop()
		for {
			if common.IsMasterNode && operation_setting.CodexQuotaAllocationEnabled {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				if err := SyncCodexQuotaAllocation(ctx); err != nil {
					common.SysLog("Codex quota allocation sync failed: " + err.Error())
				}
				cancel()
			}
			<-ticker.C
		}
	}()
}

func SyncCodexQuotaAllocation(ctx context.Context) error {
	codexQuotaSyncLock.Lock()
	defer codexQuotaSyncLock.Unlock()

	quotaData, err := RefreshManagementCodexQuotas(ctx)
	if err != nil {
		return err
	}
	now := codexQuotaNow().Unix()
	closedMinute := now/60*60 - 60

	return model.DB.Transaction(func(tx *gorm.DB) error {
		state := model.CodexQuotaSyncState{Id: 1}
		if err := tx.FirstOrCreate(&state, model.CodexQuotaSyncState{Id: 1}).Error; err != nil {
			return err
		}
		if err := expireCodexPendingUsageWeights(tx, now-int64(codexQuotaPendingGrace.Seconds())); err != nil {
			return err
		}

		cycleObservations := make([]codexCycleObservation, 0)
		seenAccounts := make(map[string]struct{})
		included := 0
		excluded := 0
		for _, item := range quotaData.Items {
			if item.Error != "" || item.AuthIndex == "" || item.AccountHash == "" {
				excluded++
				continue
			}
			if _, duplicate := seenAccounts[item.AccountHash]; duplicate {
				excluded++
				continue
			}
			foundLongWindow := false
			for _, window := range item.Windows {
				if window.ID != "weekly" && window.ID != "monthly" {
					continue
				}
				if window.UsedPercent == nil || window.ResetAt == nil || *window.ResetAt <= now {
					continue
				}
				foundLongWindow = true
				usedUnits := percentToCodexUnits(*window.UsedPercent)
				credentialHash := hashCodexCredential(item.AuthIndex)
				observation, err := observeCodexQuotaCycle(
					tx,
					credentialHash,
					item.AccountHash,
					window,
					usedUnits,
					now,
					closedMinute,
					state.LastBucketMinute,
				)
				if err != nil {
					return err
				}
				if observation != nil {
					cycleObservations = append(cycleObservations, *observation)
				}
			}
			if foundLongWindow {
				seenAccounts[item.AccountHash] = struct{}{}
				included++
			} else {
				excluded++
			}
		}

		for _, observation := range cycleObservations {
			if err := settleCodexQuotaCycle(tx, observation, now, closedMinute); err != nil {
				return err
			}
		}

		state.LastSuccessAt = now
		state.IncludedCount = included
		state.ExcludedCount = excluded
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		return cleanupSettledCodexUsageBuckets(
			tx,
			now-int64(codexQuotaBucketRetention.Seconds()),
			state.LastBucketMinute,
		)
	})
}

func cleanupSettledCodexUsageBuckets(tx *gorm.DB, beforeMinute int64, legacyWatermark int64) error {
	type bucketRef struct {
		Id            int64
		UserId        int
		AccountHash   string
		CycleId       int64
		BucketMinute  int64
		Weight        int64
		PendingWeight int64
	}

	var buckets []bucketRef
	if err := tx.Model(&model.CodexUsageBucket{}).
		Select("id", "user_id", "account_hash", "cycle_id", "bucket_minute", "weight", "pending_weight").
		Where("bucket_minute < ?", beforeMinute).
		Find(&buckets).Error; err != nil {
		return err
	}
	if len(buckets) == 0 {
		return nil
	}

	cycleIds := make([]int64, 0, len(buckets))
	seenCycleIds := make(map[int64]struct{}, len(buckets))
	for _, bucket := range buckets {
		if bucket.CycleId <= 0 {
			continue
		}
		if _, exists := seenCycleIds[bucket.CycleId]; exists {
			continue
		}
		seenCycleIds[bucket.CycleId] = struct{}{}
		cycleIds = append(cycleIds, bucket.CycleId)
	}

	watermarks := make(map[int64]int64, len(cycleIds))
	if len(cycleIds) > 0 {
		var cycles []model.CodexQuotaCycle
		if err := tx.Select("id", "last_bucket_minute").Where("id IN ?", cycleIds).Find(&cycles).Error; err != nil {
			return err
		}
		for _, cycle := range cycles {
			watermarks[cycle.Id] = cycle.LastBucketMinute
		}
	}

	deleteIds := make([]int64, 0, len(buckets))
	unattributedByBucket := make(map[codexUsageBucketKey]int64)
	for _, bucket := range buckets {
		settled := false
		if bucket.CycleId == 0 {
			if legacyWatermark > 0 && bucket.BucketMinute <= legacyWatermark {
				settled = true
			}
		} else if watermark, known := watermarks[bucket.CycleId]; known && bucket.BucketMinute <= watermark && bucket.PendingWeight == 0 {
			settled = true
		}
		if settled {
			deleteIds = append(deleteIds, bucket.Id)
			continue
		}
		deleteIds = append(deleteIds, bucket.Id)
		bucketWeight := bucket.Weight + bucket.PendingWeight
		if bucketWeight > 0 {
			key := codexUsageBucketKey{UserId: bucket.UserId, AccountHash: bucket.AccountHash, BucketMinute: bucket.BucketMinute}
			if bucketWeight > unattributedByBucket[key] {
				unattributedByBucket[key] = bucketWeight
			}
		}
	}
	if err := archiveCodexUsageWeights(tx, unattributedByBucket); err != nil {
		return err
	}
	if len(deleteIds) == 0 {
		return nil
	}
	return tx.Where("id IN ?", deleteIds).Delete(&model.CodexUsageBucket{}).Error
}

func expireCodexPendingUsageWeights(tx *gorm.DB, beforeMinute int64) error {
	type pendingBucketRef struct {
		Id            int64
		UserId        int
		AccountHash   string
		BucketMinute  int64
		PendingWeight int64
	}
	var buckets []pendingBucketRef
	if err := tx.Model(&model.CodexUsageBucket{}).
		Select("id", "user_id", "account_hash", "bucket_minute", "pending_weight").
		Where("bucket_minute < ? AND pending_weight > ?", beforeMinute, 0).
		Find(&buckets).Error; err != nil {
		return err
	}
	if len(buckets) == 0 {
		return nil
	}

	// A worker may die before it can finalize or cancel an outbound request.
	// Archive stale provisional weight explicitly instead of blocking a cycle forever.
	unattributedByBucket := make(map[codexUsageBucketKey]int64)
	ids := make([]int64, 0, len(buckets))
	for _, bucket := range buckets {
		ids = append(ids, bucket.Id)
		key := codexUsageBucketKey{UserId: bucket.UserId, AccountHash: bucket.AccountHash, BucketMinute: bucket.BucketMinute}
		if bucket.PendingWeight > unattributedByBucket[key] {
			unattributedByBucket[key] = bucket.PendingWeight
		}
	}
	if err := archiveCodexUsageWeights(tx, unattributedByBucket); err != nil {
		return err
	}
	if err := tx.Model(&model.CodexUsageBucket{}).Where("id IN ?", ids).Update("pending_weight", 0).Error; err != nil {
		return err
	}
	return tx.Where("id IN ? AND weight <= ? AND pending_weight <= ?", ids, 0, 0).Delete(&model.CodexUsageBucket{}).Error
}

func archiveCodexUsageWeights(tx *gorm.DB, byBucket map[codexUsageBucketKey]int64) error {
	archiveByAccount := make(map[codexUsageArchiveKey]codexUsageArchiveEntry)
	for key, weight := range byBucket {
		if weight <= 0 {
			continue
		}
		archiveKey := codexUsageArchiveKey{UserId: key.UserId, AccountHash: key.AccountHash}
		entry := archiveByAccount[archiveKey]
		entry.Weight += weight
		if entry.Since == 0 || key.BucketMinute < entry.Since {
			entry.Since = key.BucketMinute
		}
		archiveByAccount[archiveKey] = entry
	}
	for key, entry := range archiveByAccount {
		archive := model.CodexQuotaUnattributedUsage{
			UserId:      key.UserId,
			AccountHash: key.AccountHash,
			Weight:      entry.Weight,
			Since:       entry.Since,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "account_hash"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"weight": gorm.Expr("weight + ?", entry.Weight),
				"since":  gorm.Expr("CASE WHEN since = 0 OR since > ? THEN ? ELSE since END", entry.Since, entry.Since),
			}),
		}).Create(&archive).Error; err != nil {
			return err
		}
	}
	return nil
}

func observeCodexQuotaCycle(
	tx *gorm.DB,
	credentialHash string,
	accountHash string,
	window ManagementCodexQuotaWindow,
	usedUnits int64,
	now int64,
	closedMinute int64,
	legacyWatermark int64,
) (*codexCycleObservation, error) {
	var cycle model.CodexQuotaCycle
	cycleQuery := tx.Where("account_hash = ? AND window_type = ?", accountHash, window.ID)
	if window.ResetAtDerived {
		cycleQuery = cycleQuery.Where("reset_at > ?", now).Order("last_seen_at DESC, id DESC")
	} else {
		cycleQuery = cycleQuery.Where("reset_at = ?", *window.ResetAt).Order("generation DESC")
	}
	err := cycleQuery.First(&cycle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		claimed, err := claimLegacyCodexQuotaCycle(
			tx,
			credentialHash,
			accountHash,
			window,
			usedUnits,
			now,
			closedMinute,
			legacyWatermark,
		)
		if err != nil {
			return nil, err
		}
		if claimed {
			return nil, nil
		}
		if err := tx.Model(&model.CodexQuotaCycle{}).
			Where("account_hash = ? AND window_type = ?", accountHash, window.ID).
			Update("last_seen_at", 0).Error; err != nil {
			return nil, err
		}
		generation, err := nextCodexQuotaCycleGeneration(tx, credentialHash, accountHash, window.ID)
		if err != nil {
			return nil, err
		}
		cycle = model.CodexQuotaCycle{
			CredentialHash:    credentialHash,
			AccountHash:       accountHash,
			WindowType:        window.ID,
			ResetAt:           *window.ResetAt,
			Generation:        generation,
			CapacityUnits:     model.CodexQuotaFullWindowUnits,
			UpstreamUsedUnits: usedUnits,
			LastBucketMinute:  closedMinute,
			LastSeenAt:        now,
		}
		return nil, tx.Create(&cycle).Error
	}
	if err != nil {
		return nil, err
	}
	if usedUnits < cycle.UpstreamUsedUnits {
		if err := tx.Model(&model.CodexQuotaCycle{}).Where("id = ?", cycle.Id).
			Update("last_seen_at", 0).Error; err != nil {
			return nil, err
		}
		generation, err := nextCodexQuotaCycleGeneration(tx, credentialHash, accountHash, window.ID)
		if err != nil {
			return nil, err
		}
		cycle = model.CodexQuotaCycle{
			CredentialHash:    credentialHash,
			AccountHash:       accountHash,
			WindowType:        window.ID,
			ResetAt:           *window.ResetAt,
			Generation:        generation,
			CapacityUnits:     model.CodexQuotaFullWindowUnits,
			UpstreamUsedUnits: usedUnits,
			LastBucketMinute:  closedMinute,
			LastSeenAt:        now,
		}
		return nil, tx.Create(&cycle).Error
	}
	return &codexCycleObservation{
		Cycle:             cycle,
		UpstreamUsedUnits: usedUnits,
		ResetAt:           *window.ResetAt,
	}, nil
}

func claimLegacyCodexQuotaCycle(
	tx *gorm.DB,
	credentialHash string,
	accountHash string,
	window ManagementCodexQuotaWindow,
	usedUnits int64,
	now int64,
	closedMinute int64,
	legacyWatermark int64,
) (bool, error) {
	var legacy model.CodexQuotaCycle
	query := tx.Where("credential_hash = ? AND account_hash = ? AND window_type = ?", credentialHash, "", window.ID)
	if window.ResetAtDerived {
		query = query.Where("reset_at > ?", now).Order("last_seen_at DESC, id DESC")
	} else {
		query = query.Where("reset_at = ?", *window.ResetAt).Order("generation DESC")
	}
	if err := query.First(&legacy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	if legacyWatermark <= 0 || legacyWatermark > closedMinute {
		legacyWatermark = closedMinute
	}
	updates := map[string]interface{}{
		"account_hash":        accountHash,
		"upstream_used_units": usedUnits,
		"last_bucket_minute":  legacyWatermark,
		"last_seen_at":        now,
		"reset_at":            *window.ResetAt,
	}
	if err := tx.Model(&model.CodexQuotaCycle{}).Where("id = ?", legacy.Id).Updates(updates).Error; err != nil {
		return false, err
	}
	return true, nil
}

func nextCodexQuotaCycleGeneration(tx *gorm.DB, credentialHash string, accountHash string, windowType string) (int, error) {
	var generation int
	if err := tx.Model(&model.CodexQuotaCycle{}).
		Where("(account_hash = ? OR credential_hash = ?) AND window_type = ?", accountHash, credentialHash, windowType).
		Select("COALESCE(MAX(generation), 0)").Scan(&generation).Error; err != nil {
		return 0, err
	}
	return generation + 1, nil
}

func settleCodexQuotaCycle(tx *gorm.DB, observation codexCycleObservation, now int64, closedMinute int64) error {
	cycle := observation.Cycle
	updates := map[string]interface{}{
		"last_seen_at": now,
		"reset_at":     observation.ResetAt,
	}
	if cycle.LastBucketMinute == 0 {
		updates["upstream_used_units"] = observation.UpstreamUsedUnits
		updates["last_bucket_minute"] = closedMinute
		return tx.Model(&model.CodexQuotaCycle{}).Where("id = ?", cycle.Id).Updates(updates).Error
	}

	delta := observation.UpstreamUsedUnits - cycle.UpstreamUsedUnits
	if delta <= 0 || closedMinute <= cycle.LastBucketMinute {
		return tx.Model(&model.CodexQuotaCycle{}).Where("id = ?", cycle.Id).Updates(updates).Error
	}
	hasPendingWeights, err := hasCodexPendingWeightsAfter(tx, cycle.Id, cycle.LastBucketMinute)
	if err != nil {
		return err
	}
	if hasPendingWeights {
		// Do not settle against an in-flight request. If it fails after this
		// poll, its provisional weight must not become confirmed usage.
		return tx.Model(&model.CodexQuotaCycle{}).Where("id = ?", cycle.Id).Updates(updates).Error
	}

	weights, err := loadCodexWeights(tx, cycle.Id, cycle.LastBucketMinute, closedMinute)
	if err != nil {
		return err
	}
	if len(weights) == 0 {
		hasOpenWeights, err := hasCodexWeightsAfter(tx, cycle.Id, closedMinute)
		if err != nil {
			return err
		}
		if !hasOpenWeights {
			updates["upstream_used_units"] = observation.UpstreamUsedUnits
			updates["last_bucket_minute"] = closedMinute
		}
		return tx.Model(&model.CodexQuotaCycle{}).Where("id = ?", cycle.Id).Updates(updates).Error
	}

	if err := attributeCodexCycleDeltas(tx, []codexCycleDelta{{CycleId: cycle.Id, Delta: delta}}, weights); err != nil {
		return err
	}
	updates["upstream_used_units"] = observation.UpstreamUsedUnits
	updates["last_bucket_minute"] = closedMinute
	return tx.Model(&model.CodexQuotaCycle{}).Where("id = ?", cycle.Id).Updates(updates).Error
}

func hasCodexWeightsAfter(tx *gorm.DB, cycleId int64, bucketMinute int64) (bool, error) {
	var count int64
	err := tx.Model(&model.CodexUsageBucket{}).
		Where("cycle_id = ? AND bucket_minute > ? AND (weight > ? OR pending_weight > ?)", cycleId, bucketMinute, 0, 0).
		Count(&count).Error
	return count > 0, err
}

func hasCodexPendingWeightsAfter(tx *gorm.DB, cycleId int64, afterMinute int64) (bool, error) {
	var count int64
	err := tx.Model(&model.CodexUsageBucket{}).
		Where("cycle_id = ? AND bucket_minute > ? AND pending_weight > ?", cycleId, afterMinute, 0).
		Count(&count).Error
	return count > 0, err
}

func loadCodexWeights(tx *gorm.DB, cycleId int64, afterMinute int64, throughMinute int64) ([]codexWeight, error) {
	type row struct {
		UserId int
		Weight int64
	}
	var rows []row
	if err := tx.Model(&model.CodexUsageBucket{}).
		Select("user_id, SUM(weight) AS weight").
		Where("cycle_id = ? AND bucket_minute > ? AND bucket_minute <= ?", cycleId, afterMinute, throughMinute).
		Group("user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserId)
	}
	var users []model.User
	if err := tx.Select("id", "role").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	roles := make(map[int]int, len(users))
	for _, user := range users {
		roles[user.Id] = user.Role
	}
	weights := make([]codexWeight, 0, len(rows))
	for _, row := range rows {
		if row.Weight > 0 {
			// Privileged users consume the same upstream window, so their weight
			// must remain in the denominator even though they are not quota-billed.
			weights = append(weights, codexWeight{UserId: row.UserId, Weight: row.Weight, Role: roles[row.UserId]})
		}
	}
	return weights, nil
}

func attributeCodexCycleDeltas(tx *gorm.DB, deltas []codexCycleDelta, weights []codexWeight) error {
	if len(deltas) == 0 || len(weights) == 0 {
		return nil
	}
	totalWeight := int64(0)
	for _, weight := range weights {
		totalWeight += weight.Weight
	}
	if totalWeight <= 0 {
		return nil
	}
	for _, delta := range deltas {
		shares := proportionalCodexShares(delta.Delta, weights, totalWeight)
		for index, amount := range shares {
			if amount <= 0 || weights[index].Role != common.RoleCommonUser {
				continue
			}
			usage := model.CodexUserCycleUsage{UserId: weights[index].UserId, CycleId: delta.CycleId, UsedUnits: amount}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}, {Name: "cycle_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{"used_units": gorm.Expr("used_units + ?", amount)}),
			}).Create(&usage).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func proportionalCodexShares(total int64, weights []codexWeight, totalWeight int64) []int64 {
	type remainder struct {
		Index int
		Value int64
	}
	result := make([]int64, len(weights))
	remainders := make([]remainder, len(weights))
	allocated := int64(0)
	for index, weight := range weights {
		product := total * weight.Weight
		result[index] = product / totalWeight
		remainders[index] = remainder{Index: index, Value: product % totalWeight}
		allocated += result[index]
	}
	sort.SliceStable(remainders, func(i, j int) bool {
		if remainders[i].Value == remainders[j].Value {
			return weights[remainders[i].Index].UserId < weights[remainders[j].Index].UserId
		}
		return remainders[i].Value > remainders[j].Value
	})
	for remaining, index := total-allocated, 0; remaining > 0 && index < len(remainders); remaining, index = remaining-1, index+1 {
		result[remainders[index].Index]++
	}
	return result
}

func GetCodexQuotaAllocationSummary(userId int) (*CodexQuotaAllocationSummary, error) {
	share, bonus, role, err := model.GetCodexQuotaPolicy(userId)
	if err != nil {
		return nil, err
	}
	pool, cycles, state, err := loadCodexPoolState()
	if err != nil {
		return nil, err
	}
	effective := share + bonus
	cycleIds := make([]int64, 0, len(cycles))
	for _, cycle := range cycles {
		cycleIds = append(cycleIds, cycle.Id)
	}
	used := int64(0)
	if len(cycleIds) > 0 {
		if err := model.DB.Model(&model.CodexUserCycleUsage{}).Where("user_id = ? AND cycle_id IN ?", userId, cycleIds).Select("COALESCE(SUM(used_units), 0)").Scan(&used).Error; err != nil {
			return nil, err
		}
	}
	pending := codexPendingUsage{}
	if role == common.RoleCommonUser {
		pending, err = getCodexPendingUsage(userId, cycles, state.LastBucketMinute)
		if err != nil {
			return nil, err
		}
	}
	allocated := pool.PoolCapacityUnits * int64(effective) / model.CodexQuotaMaxBps
	remaining := allocated - used
	if remaining < 0 {
		remaining = 0
	}
	now := time.Now().Unix()
	stale := isCodexQuotaSyncStale(state.LastSuccessAt, now)
	poolAvailable := isCodexQuotaPoolAvailable(pool, cycles, state, now)
	return &CodexQuotaAllocationSummary{
		Enabled:            operation_setting.CodexQuotaAllocationEnabled,
		PoolAvailable:      poolAvailable,
		ShareBps:           share,
		BonusBps:           bonus,
		EffectiveBps:       effective,
		PoolCapacityUnits:  pool.PoolCapacityUnits,
		PoolUsedUnits:      pool.PoolUsedUnits,
		PoolRemainUnits:    pool.PoolRemainUnits,
		AllocatedUnits:     allocated,
		UsedUnits:          used,
		SettledUsedUnits:   used,
		RemainingUnits:     remaining,
		PendingWeight:      pending.PendingWeight,
		PendingSince:       pending.PendingSince,
		UnattributedWeight: pending.UnattributedWeight,
		UnattributedSince:  pending.UnattributedSince,
		IncludedCount:      len(cycles),
		ExcludedCount:      state.ExcludedCount,
		LastUpdatedAt:      state.LastSuccessAt,
		Stale:              stale,
	}, nil
}

type codexPendingUsage struct {
	PendingWeight      int64
	PendingSince       int64
	UnattributedWeight int64
	UnattributedSince  int64
}

func getCodexPendingUsage(userId int, cycles []model.CodexQuotaCycle, legacyWatermark int64) (codexPendingUsage, error) {
	activeCycleIds := make(map[int64]struct{}, len(cycles))
	cycleIds := make([]int64, 0, len(cycles))
	for _, cycle := range cycles {
		activeCycleIds[cycle.Id] = struct{}{}
		cycleIds = append(cycleIds, cycle.Id)
	}

	var buckets []model.CodexUsageBucket
	if err := model.DB.Where("user_id = ?", userId).Find(&buckets).Error; err != nil {
		return codexPendingUsage{}, err
	}
	for _, bucket := range buckets {
		if bucket.CycleId > 0 {
			cycleIds = append(cycleIds, bucket.CycleId)
		}
	}
	cycleWatermarks := make(map[int64]int64, len(cycleIds))
	if len(cycleIds) > 0 {
		var allCycles []model.CodexQuotaCycle
		if err := model.DB.Select("id", "last_bucket_minute").Where("id IN ?", cycleIds).Find(&allCycles).Error; err != nil {
			return codexPendingUsage{}, err
		}
		for _, cycle := range allCycles {
			cycleWatermarks[cycle.Id] = cycle.LastBucketMinute
		}
	}

	type bucketKey struct {
		AccountHash  string
		BucketMinute int64
	}
	pendingByBucket := make(map[bucketKey]int64)
	unattributedByBucket := make(map[bucketKey]int64)
	for _, bucket := range buckets {
		bucketWeight := bucket.Weight + bucket.PendingWeight
		if bucketWeight <= 0 {
			continue
		}
		if bucket.CycleId == 0 && legacyWatermark > 0 && bucket.BucketMinute <= legacyWatermark {
			continue
		}
		key := bucketKey{AccountHash: bucket.AccountHash, BucketMinute: bucket.BucketMinute}
		if _, active := activeCycleIds[bucket.CycleId]; active {
			watermark := cycleWatermarks[bucket.CycleId]
			if bucket.BucketMinute > watermark && bucketWeight > pendingByBucket[key] {
				pendingByBucket[key] = bucketWeight
			}
			if bucket.BucketMinute <= watermark && bucket.PendingWeight > 0 && bucketWeight > unattributedByBucket[key] {
				unattributedByBucket[key] = bucketWeight
			}
			continue
		}
		if watermark, known := cycleWatermarks[bucket.CycleId]; known && bucket.BucketMinute <= watermark {
			continue
		}
		if bucketWeight > unattributedByBucket[key] {
			unattributedByBucket[key] = bucketWeight
		}
	}

	pending := codexPendingUsage{}
	for key, weight := range pendingByBucket {
		pending.PendingWeight += weight
		if pending.PendingSince == 0 || key.BucketMinute < pending.PendingSince {
			pending.PendingSince = key.BucketMinute
		}
	}
	for key, weight := range unattributedByBucket {
		pending.UnattributedWeight += weight
		if pending.UnattributedSince == 0 || key.BucketMinute < pending.UnattributedSince {
			pending.UnattributedSince = key.BucketMinute
		}
	}
	var archived []model.CodexQuotaUnattributedUsage
	if err := model.DB.Where("user_id = ?", userId).Find(&archived).Error; err != nil {
		return codexPendingUsage{}, err
	}
	for _, entry := range archived {
		if entry.Weight <= 0 {
			continue
		}
		pending.UnattributedWeight += entry.Weight
		if pending.UnattributedSince == 0 || (entry.Since > 0 && entry.Since < pending.UnattributedSince) {
			pending.UnattributedSince = entry.Since
		}
	}
	return pending, nil
}

func GetCodexQuotaPoolSummary() (*CodexQuotaPoolSummary, error) {
	pool, cycles, state, err := loadCodexPoolState()
	if err != nil {
		return nil, err
	}
	var allocated int64
	if err := model.DB.Model(&model.User{}).Where("role = ?", common.RoleCommonUser).
		Select("COALESCE(SUM(codex_quota_share_bps + codex_quota_bonus_bps), 0)").Scan(&allocated).Error; err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	pool.Enabled = operation_setting.CodexQuotaAllocationEnabled
	pool.PoolAvailable = isCodexQuotaPoolAvailable(pool, cycles, state, now)
	pool.AllocatedBps = allocated
	pool.IncludedCount = len(cycles)
	pool.ExcludedCount = state.ExcludedCount
	pool.LastUpdatedAt = state.LastSuccessAt
	pool.Stale = isCodexQuotaSyncStale(state.LastSuccessAt, now)
	return &pool, nil
}

func isCodexQuotaSyncStale(lastSuccessAt int64, now int64) bool {
	return lastSuccessAt <= 0 || now-lastSuccessAt > int64(codexQuotaStaleAfter.Seconds())
}

func isCodexQuotaPoolAvailable(pool CodexQuotaPoolSummary, cycles []model.CodexQuotaCycle, state model.CodexQuotaSyncState, now int64) bool {
	return !isCodexQuotaSyncStale(state.LastSuccessAt, now) &&
		state.IncludedCount > 0 &&
		len(cycles) > 0 &&
		pool.PoolCapacityUnits > 0
}

func loadCodexPoolState() (CodexQuotaPoolSummary, []model.CodexQuotaCycle, model.CodexQuotaSyncState, error) {
	state, err := model.GetCodexQuotaSyncState()
	if err != nil {
		return CodexQuotaPoolSummary{}, nil, state, err
	}
	now := time.Now().Unix()
	var cycles []model.CodexQuotaCycle
	err = model.DB.Where(
		"account_hash <> ? AND last_bucket_minute > ? AND reset_at > ? AND last_seen_at >= ?",
		"",
		0,
		now,
		now-int64(codexQuotaStaleAfter.Seconds()),
	).Find(&cycles).Error
	if err != nil {
		return CodexQuotaPoolSummary{}, nil, state, err
	}
	pool := CodexQuotaPoolSummary{}
	for _, cycle := range cycles {
		pool.PoolCapacityUnits += cycle.CapacityUnits
		pool.PoolUsedUnits += cycle.UpstreamUsedUnits
	}
	pool.PoolRemainUnits = pool.PoolCapacityUnits - pool.PoolUsedUnits
	if pool.PoolRemainUnits < 0 {
		pool.PoolRemainUnits = 0
	}
	return pool, cycles, state, nil
}

func CheckCodexQuotaAccess(c *gin.Context, userId int) *types.NewAPIError {
	clearCodexQuotaBinding(c)
	if !operation_setting.CodexQuotaAllocationEnabled {
		return nil
	}
	_, _, role, policyErr := model.GetCodexQuotaPolicy(userId)
	if policyErr != nil {
		return types.NewErrorWithStatusCode(policyErr, types.ErrorCodeCodexQuotaUnavailable, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	accountHash := resolveCodexQuotaAccountHash(common.GetContextKeyString(c, constant.ContextKeyChannelKey))
	if accountHash == "" {
		if role >= common.RoleAdminUser {
			return nil
		}
		return types.NewErrorWithStatusCode(errors.New("Codex channel is missing a quota account binding"), types.ErrorCodeCodexQuotaUnavailable, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	now := time.Now().Unix()
	cycles, err := loadCodexQuotaCyclesForAccount(accountHash, now, role < common.RoleAdminUser)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeCodexQuotaUnavailable, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if role >= common.RoleAdminUser {
		// Admins bypass quota admission, but their successful requests must be
		// recorded as non-billable weight so they cannot be charged to users.
		bindCodexQuotaCycles(c, accountHash, cycles)
		return nil
	}
	summary, err := GetCodexQuotaAllocationSummary(userId)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeCodexQuotaUnavailable, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if !summary.PoolAvailable {
		return types.NewErrorWithStatusCode(errors.New("Codex quota pool is unavailable"), types.ErrorCodeCodexQuotaUnavailable, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if summary.EffectiveBps <= 0 || summary.RemainingUnits <= 0 || summary.PoolRemainUnits <= 0 {
		return types.NewErrorWithStatusCode(errors.New("Codex quota share exhausted"), types.ErrorCodeCodexQuotaShareExhausted, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if len(cycles) == 0 {
		return types.NewErrorWithStatusCode(errors.New("Codex channel account is not included in the managed quota pool"), types.ErrorCodeCodexQuotaUnavailable, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	bindCodexQuotaCycles(c, accountHash, cycles)
	return nil
}

func loadActiveCodexQuotaCyclesForAccount(accountHash string, now int64) ([]model.CodexQuotaCycle, error) {
	return loadCodexQuotaCyclesForAccount(accountHash, now, true)
}

func loadCodexQuotaCyclesForAccount(accountHash string, now int64, requireFreshSync bool) ([]model.CodexQuotaCycle, error) {
	var cycles []model.CodexQuotaCycle
	query := model.DB.Where(
		"account_hash = ? AND last_bucket_minute > ? AND reset_at > ?",
		accountHash,
		0,
		now,
	)
	if requireFreshSync {
		query = query.Where("last_seen_at >= ?", now-int64(codexQuotaStaleAfter.Seconds()))
	}
	err := query.Find(&cycles).Error
	return cycles, err
}

func bindCodexQuotaCycles(c *gin.Context, accountHash string, cycles []model.CodexQuotaCycle) {
	if len(cycles) == 0 {
		return
	}
	cycleIds := make([]int64, 0, len(cycles))
	for _, cycle := range cycles {
		cycleIds = append(cycleIds, cycle.Id)
	}
	common.SetContextKey(c, constant.ContextKeyCodexQuotaAccountHash, accountHash)
	common.SetContextKey(c, constant.ContextKeyCodexQuotaCycleIds, cycleIds)
}

func clearCodexQuotaBinding(c *gin.Context) {
	common.SetContextKey(c, constant.ContextKeyCodexQuotaAccountHash, "")
	common.SetContextKey(c, constant.ContextKeyCodexQuotaCycleIds, nil)
	common.SetContextKey(c, constant.ContextKeyCodexQuotaTracking, nil)
}

func StartCodexQuotaUsageTracking(c *gin.Context, userId int, channelId int, channelKeyIndex int, estimatedPromptTokens int) error {
	cycleIds, hasCycleBinding := common.GetContextKeyType[[]int64](c, constant.ContextKeyCodexQuotaCycleIds)
	accountHash := common.GetContextKeyString(c, constant.ContextKeyCodexQuotaAccountHash)
	if !hasCycleBinding || accountHash == "" || len(cycleIds) == 0 {
		return nil
	}

	provisionalWeight := codexUsageWeight(0, estimatedPromptTokens)
	bucketMinute := codexQuotaNow().Unix() / 60 * 60
	if err := model.ReserveCodexUsageWeight(
		userId,
		accountHash,
		cycleIds,
		channelId,
		channelKeyIndex,
		bucketMinute,
		provisionalWeight,
	); err != nil {
		return err
	}
	common.SetContextKey(c, constant.ContextKeyCodexQuotaTracking, &codexQuotaUsageTracking{
		AccountHash:       accountHash,
		CycleIds:          append([]int64(nil), cycleIds...),
		BucketMinute:      bucketMinute,
		ProvisionalWeight: provisionalWeight,
	})
	return nil
}

func FinalizeCodexQuotaUsageTracking(c *gin.Context, userId int, channelId int, channelKeyIndex int, totalTokens int, estimatedPromptTokens int) error {
	tracking, ok := common.GetContextKeyType[*codexQuotaUsageTracking](c, constant.ContextKeyCodexQuotaTracking)
	if !ok || tracking == nil {
		cycleIds, hasCycleBinding := common.GetContextKeyType[[]int64](c, constant.ContextKeyCodexQuotaCycleIds)
		accountHash := common.GetContextKeyString(c, constant.ContextKeyCodexQuotaAccountHash)
		if !hasCycleBinding || accountHash == "" {
			return nil
		}
		return model.RecordCodexUsageWeight(
			userId,
			accountHash,
			cycleIds,
			channelId,
			channelKeyIndex,
			codexUsageWeight(totalTokens, estimatedPromptTokens),
		)
	}
	if tracking.Finalized {
		return nil
	}

	finalWeight := codexUsageWeight(totalTokens, estimatedPromptTokens)
	if err := model.FinalizeCodexUsageWeight(
		userId,
		tracking.AccountHash,
		tracking.CycleIds,
		tracking.BucketMinute,
		tracking.ProvisionalWeight,
		finalWeight,
	); err != nil {
		return err
	}
	tracking.Finalized = true
	return nil
}

func CancelCodexQuotaUsageTracking(c *gin.Context, userId int) error {
	tracking, ok := common.GetContextKeyType[*codexQuotaUsageTracking](c, constant.ContextKeyCodexQuotaTracking)
	if !ok || tracking == nil || tracking.Finalized {
		return nil
	}
	if err := model.CancelCodexUsageWeight(
		userId,
		tracking.AccountHash,
		tracking.CycleIds,
		tracking.BucketMinute,
		tracking.ProvisionalWeight,
	); err != nil {
		return err
	}
	tracking.Finalized = true
	return nil
}

func percentToCodexUnits(percent float64) int64 {
	clamped := math.Max(0, math.Min(100, percent))
	return int64(math.Round(clamped / 100 * float64(model.CodexQuotaFullWindowUnits)))
}

func hashCodexCredential(authIndex string) string {
	return common.Sha256([]byte(authIndex))
}

func hashCodexAccountID(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ""
	}
	return common.Sha256([]byte("codex-account:" + accountID))
}

func resolveCodexQuotaAccountHash(rawKey string) string {
	var key struct {
		AccountID string `json:"account_id"`
	}
	if err := common.Unmarshal([]byte(strings.TrimSpace(rawKey)), &key); err != nil {
		return ""
	}
	return hashCodexAccountID(key.AccountID)
}
