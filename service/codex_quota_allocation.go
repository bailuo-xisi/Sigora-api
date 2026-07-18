package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	codexQuotaSyncInterval = 60 * time.Second
	codexQuotaStaleAfter   = 5 * time.Minute
)

type CodexQuotaAllocationSummary struct {
	Enabled           bool  `json:"enabled"`
	ShareBps          int   `json:"share_bps"`
	BonusBps          int   `json:"bonus_bps"`
	EffectiveBps      int   `json:"effective_bps"`
	PoolCapacityUnits int64 `json:"pool_capacity_units"`
	PoolUsedUnits     int64 `json:"pool_used_units"`
	PoolRemainUnits   int64 `json:"pool_remaining_units"`
	AllocatedUnits    int64 `json:"allocated_units"`
	UsedUnits         int64 `json:"used_units"`
	RemainingUnits    int64 `json:"remaining_units"`
	IncludedCount     int   `json:"included_count"`
	ExcludedCount     int   `json:"excluded_count"`
	LastUpdatedAt     int64 `json:"last_updated_at"`
	Stale             bool  `json:"stale"`
}

type CodexQuotaPoolSummary struct {
	Enabled           bool  `json:"enabled"`
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

type codexWeight struct {
	UserId int
	Weight int64
	Role   int
}

var codexQuotaSyncLock sync.Mutex

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
	now := time.Now().Unix()
	closedMinute := now/60*60 - 60

	return model.DB.Transaction(func(tx *gorm.DB) error {
		state := model.CodexQuotaSyncState{Id: 1}
		if err := tx.FirstOrCreate(&state, model.CodexQuotaSyncState{Id: 1}).Error; err != nil {
			return err
		}

		cycleDeltas := make([]codexCycleDelta, 0)
		included := 0
		excluded := 0
		for _, item := range quotaData.Items {
			if item.Error != "" || item.AuthIndex == "" {
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
				var cycle model.CodexQuotaCycle
				err := tx.Where("credential_hash = ? AND window_type = ? AND reset_at = ?", credentialHash, window.ID, *window.ResetAt).
					Order("generation DESC").First(&cycle).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					if err := tx.Model(&model.CodexQuotaCycle{}).
						Where("credential_hash = ? AND window_type = ?", credentialHash, window.ID).
						Update("last_seen_at", 0).Error; err != nil {
						return err
					}
					cycle = model.CodexQuotaCycle{
						CredentialHash:    credentialHash,
						WindowType:        window.ID,
						ResetAt:           *window.ResetAt,
						Generation:        1,
						CapacityUnits:     model.CodexQuotaFullWindowUnits,
						UpstreamUsedUnits: usedUnits,
						LastSeenAt:        now,
					}
					if err := tx.Create(&cycle).Error; err != nil {
						return err
					}
					continue
				}
				if err != nil {
					return err
				}
				if usedUnits < cycle.UpstreamUsedUnits {
					if err := tx.Model(&model.CodexQuotaCycle{}).Where("id = ?", cycle.Id).
						Update("last_seen_at", 0).Error; err != nil {
						return err
					}
					cycle = model.CodexQuotaCycle{
						CredentialHash:    credentialHash,
						WindowType:        window.ID,
						ResetAt:           *window.ResetAt,
						Generation:        cycle.Generation + 1,
						CapacityUnits:     model.CodexQuotaFullWindowUnits,
						UpstreamUsedUnits: usedUnits,
						LastSeenAt:        now,
					}
					if err := tx.Create(&cycle).Error; err != nil {
						return err
					}
					continue
				}
				delta := usedUnits - cycle.UpstreamUsedUnits
				if delta > 0 {
					cycleDeltas = append(cycleDeltas, codexCycleDelta{CycleId: cycle.Id, Delta: delta})
				}
				if err := tx.Model(&model.CodexQuotaCycle{}).Where("id = ?", cycle.Id).Updates(map[string]interface{}{
					"upstream_used_units": usedUnits,
					"last_seen_at":        now,
				}).Error; err != nil {
					return err
				}
			}
			if foundLongWindow {
				included++
			} else {
				excluded++
			}
		}

		if state.LastBucketMinute == 0 {
			state.LastBucketMinute = closedMinute
		} else if closedMinute > state.LastBucketMinute && len(cycleDeltas) > 0 {
			weights, err := loadCodexWeights(tx, state.LastBucketMinute, closedMinute)
			if err != nil {
				return err
			}
			if err := attributeCodexCycleDeltas(tx, cycleDeltas, weights); err != nil {
				return err
			}
			state.LastBucketMinute = closedMinute
		}

		state.LastSuccessAt = now
		state.IncludedCount = included
		state.ExcludedCount = excluded
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		return tx.Where("bucket_minute < ?", state.LastBucketMinute-86400).Delete(&model.CodexUsageBucket{}).Error
	})
}

func loadCodexWeights(tx *gorm.DB, afterMinute int64, throughMinute int64) ([]codexWeight, error) {
	type row struct {
		UserId int
		Weight int64
	}
	var rows []row
	if err := tx.Model(&model.CodexUsageBucket{}).
		Select("user_id, SUM(weight) AS weight").
		Where("bucket_minute > ? AND bucket_minute <= ?", afterMinute, throughMinute).
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
	share, bonus, _, err := model.GetCodexQuotaPolicy(userId)
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
	allocated := pool.PoolCapacityUnits * int64(effective) / model.CodexQuotaMaxBps
	remaining := allocated - used
	if remaining < 0 {
		remaining = 0
	}
	return &CodexQuotaAllocationSummary{
		Enabled:  operation_setting.CodexQuotaAllocationEnabled,
		ShareBps: share, BonusBps: bonus, EffectiveBps: effective,
		PoolCapacityUnits: pool.PoolCapacityUnits, PoolUsedUnits: pool.PoolUsedUnits, PoolRemainUnits: pool.PoolRemainUnits,
		AllocatedUnits: allocated, UsedUnits: used, RemainingUnits: remaining,
		IncludedCount: len(cycles), ExcludedCount: state.ExcludedCount,
		LastUpdatedAt: state.LastSuccessAt, Stale: time.Now().Unix()-state.LastSuccessAt > int64(codexQuotaStaleAfter.Seconds()),
	}, nil
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
	pool.Enabled = operation_setting.CodexQuotaAllocationEnabled
	pool.AllocatedBps = allocated
	pool.IncludedCount = len(cycles)
	pool.ExcludedCount = state.ExcludedCount
	pool.LastUpdatedAt = state.LastSuccessAt
	pool.Stale = time.Now().Unix()-state.LastSuccessAt > int64(codexQuotaStaleAfter.Seconds())
	return &pool, nil
}

func loadCodexPoolState() (CodexQuotaPoolSummary, []model.CodexQuotaCycle, model.CodexQuotaSyncState, error) {
	state, err := model.GetCodexQuotaSyncState()
	if err != nil {
		return CodexQuotaPoolSummary{}, nil, state, err
	}
	now := time.Now().Unix()
	var cycles []model.CodexQuotaCycle
	err = model.DB.Where("reset_at > ? AND last_seen_at >= ?", now, now-int64(codexQuotaStaleAfter.Seconds())).Find(&cycles).Error
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

func CheckCodexQuotaAccess(userId int) *types.NewAPIError {
	if !operation_setting.CodexQuotaAllocationEnabled {
		return nil
	}
	_, _, role, policyErr := model.GetCodexQuotaPolicy(userId)
	if policyErr != nil {
		return types.NewErrorWithStatusCode(policyErr, types.ErrorCodeCodexQuotaUnavailable, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if role >= common.RoleAdminUser {
		return nil
	}
	summary, err := GetCodexQuotaAllocationSummary(userId)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeCodexQuotaUnavailable, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if summary.Stale || summary.PoolCapacityUnits == 0 {
		return types.NewErrorWithStatusCode(errors.New("Codex quota pool is unavailable"), types.ErrorCodeCodexQuotaUnavailable, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if summary.EffectiveBps <= 0 || summary.RemainingUnits <= 0 || summary.PoolRemainUnits <= 0 {
		return types.NewErrorWithStatusCode(errors.New("Codex quota share exhausted"), types.ErrorCodeCodexQuotaShareExhausted, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	return nil
}

func percentToCodexUnits(percent float64) int64 {
	clamped := math.Max(0, math.Min(100, percent))
	return int64(math.Round(clamped / 100 * float64(model.CodexQuotaFullWindowUnits)))
}

func hashCodexCredential(authIndex string) string {
	sum := sha256.Sum256([]byte(authIndex))
	return hex.EncodeToString(sum[:])
}
