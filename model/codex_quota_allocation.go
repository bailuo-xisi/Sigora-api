package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	CodexQuotaFullWindowUnits int64 = 1_000_000
	CodexQuotaMaxBps                = 10_000
)

type CodexQuotaCycle struct {
	Id                int64  `json:"id"`
	CredentialHash    string `json:"-" gorm:"type:varchar(64);not null;index:idx_codex_cycle,unique"`
	AccountHash       string `json:"-" gorm:"type:varchar(64);not null;default:'';index"`
	WindowType        string `json:"window_type" gorm:"type:varchar(16);not null;index:idx_codex_cycle,unique"`
	ResetAt           int64  `json:"reset_at" gorm:"type:bigint;not null;index:idx_codex_cycle,unique;index"`
	Generation        int    `json:"generation" gorm:"type:int;not null;default:1;index:idx_codex_cycle,unique"`
	CapacityUnits     int64  `json:"capacity_units" gorm:"type:bigint;not null"`
	UpstreamUsedUnits int64  `json:"upstream_used_units" gorm:"type:bigint;not null;default:0"`
	LastBucketMinute  int64  `json:"-" gorm:"type:bigint;not null;default:0"`
	LastSeenAt        int64  `json:"last_seen_at" gorm:"type:bigint;not null;index"`
	CreatedAt         int64  `json:"created_at" gorm:"type:bigint;autoCreateTime"`
	UpdatedAt         int64  `json:"updated_at" gorm:"type:bigint;autoUpdateTime"`
}

type CodexUserCycleUsage struct {
	Id        int64 `json:"id"`
	UserId    int   `json:"user_id" gorm:"not null;index:idx_codex_user_cycle,unique"`
	CycleId   int64 `json:"cycle_id" gorm:"not null;index:idx_codex_user_cycle,unique;index"`
	UsedUnits int64 `json:"used_units" gorm:"type:bigint;not null;default:0"`
	UpdatedAt int64 `json:"updated_at" gorm:"type:bigint;autoUpdateTime"`
}

type CodexUsageBucket struct {
	Id              int64  `json:"id"`
	UserId          int    `json:"user_id" gorm:"not null;index:idx_codex_usage_bucket_account_minute,unique"`
	AccountHash     string `json:"-" gorm:"type:varchar(64);not null;default:'';index:idx_codex_usage_bucket_account_minute,unique;index"`
	CycleId         int64  `json:"-" gorm:"type:bigint;not null;default:0;index:idx_codex_usage_bucket_account_minute,unique;index"`
	BucketMinute    int64  `json:"bucket_minute" gorm:"type:bigint;not null;index:idx_codex_usage_bucket_account_minute,unique;index"`
	Weight          int64  `json:"weight" gorm:"type:bigint;not null;default:0"`
	PendingWeight   int64  `json:"-" gorm:"type:bigint;not null;default:0"`
	ChannelId       int    `json:"-" gorm:"not null;default:0;index"`
	ChannelKeyIndex int    `json:"-" gorm:"not null;default:0"`
}

type CodexQuotaUnattributedUsage struct {
	Id          int64  `json:"id"`
	UserId      int    `json:"-" gorm:"not null;index:idx_codex_unattributed_user_account,unique"`
	AccountHash string `json:"-" gorm:"type:varchar(64);not null;default:'';index:idx_codex_unattributed_user_account,unique"`
	Weight      int64  `json:"-" gorm:"type:bigint;not null;default:0"`
	Since       int64  `json:"-" gorm:"type:bigint;not null;default:0"`
	UpdatedAt   int64  `json:"-" gorm:"autoUpdateTime"`
}

type CodexQuotaSyncState struct {
	Id               int   `json:"id" gorm:"primaryKey"`
	LastSuccessAt    int64 `json:"last_success_at" gorm:"type:bigint;not null;default:0"`
	LastBucketMinute int64 `json:"last_bucket_minute" gorm:"type:bigint;not null;default:0"`
	IncludedCount    int   `json:"included_count" gorm:"type:int;not null;default:0"`
	ExcludedCount    int   `json:"excluded_count" gorm:"type:int;not null;default:0"`
}

func RecordCodexUsageWeight(userId int, accountHash string, cycleIds []int64, channelId int, channelKeyIndex int, weight int64) error {
	return RecordCodexUsageWeightAt(
		userId,
		accountHash,
		cycleIds,
		channelId,
		channelKeyIndex,
		time.Now().Unix()/60*60,
		weight,
	)
}

func RecordCodexUsageWeightAt(userId int, accountHash string, cycleIds []int64, channelId int, channelKeyIndex int, bucketMinute int64, weight int64) error {
	if userId <= 0 || accountHash == "" || len(cycleIds) == 0 || bucketMinute <= 0 || weight <= 0 {
		return nil
	}
	return recordCodexUsageWeight(userId, accountHash, cycleIds, channelId, channelKeyIndex, bucketMinute, weight, false)
}

func ReserveCodexUsageWeight(userId int, accountHash string, cycleIds []int64, channelId int, channelKeyIndex int, bucketMinute int64, weight int64) error {
	if userId <= 0 || accountHash == "" || len(cycleIds) == 0 || bucketMinute <= 0 || weight <= 0 {
		return nil
	}
	return recordCodexUsageWeight(userId, accountHash, cycleIds, channelId, channelKeyIndex, bucketMinute, weight, true)
}

func FinalizeCodexUsageWeight(userId int, accountHash string, cycleIds []int64, bucketMinute int64, provisionalWeight int64, finalWeight int64) error {
	if userId <= 0 || accountHash == "" || len(cycleIds) == 0 || bucketMinute <= 0 || provisionalWeight <= 0 || finalWeight <= 0 {
		return nil
	}
	return transitionCodexUsageWeight(userId, accountHash, cycleIds, bucketMinute, provisionalWeight, finalWeight)
}

func CancelCodexUsageWeight(userId int, accountHash string, cycleIds []int64, bucketMinute int64, provisionalWeight int64) error {
	if userId <= 0 || accountHash == "" || len(cycleIds) == 0 || bucketMinute <= 0 || provisionalWeight <= 0 {
		return nil
	}
	return transitionCodexUsageWeight(userId, accountHash, cycleIds, bucketMinute, provisionalWeight, 0)
}

func recordCodexUsageWeight(userId int, accountHash string, cycleIds []int64, channelId int, channelKeyIndex int, bucketMinute int64, weight int64, pending bool) error {
	seenCycles := make(map[int64]struct{}, len(cycleIds))
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, cycleId := range cycleIds {
			if cycleId <= 0 {
				continue
			}
			if _, exists := seenCycles[cycleId]; exists {
				continue
			}
			seenCycles[cycleId] = struct{}{}
			row := CodexUsageBucket{
				UserId:          userId,
				AccountHash:     accountHash,
				CycleId:         cycleId,
				BucketMinute:    bucketMinute,
				ChannelId:       channelId,
				ChannelKeyIndex: channelKeyIndex,
			}
			column := "weight"
			if pending {
				row.PendingWeight = weight
				column = "pending_weight"
			} else {
				row.Weight = weight
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "user_id"},
					{Name: "account_hash"},
					{Name: "cycle_id"},
					{Name: "bucket_minute"},
				},
				DoUpdates: clause.Assignments(map[string]interface{}{
					column: gorm.Expr(column+" + ?", weight),
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func transitionCodexUsageWeight(userId int, accountHash string, cycleIds []int64, bucketMinute int64, provisionalWeight int64, finalWeight int64) error {
	seenCycles := make(map[int64]struct{}, len(cycleIds))
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, cycleId := range cycleIds {
			if cycleId <= 0 {
				continue
			}
			if _, exists := seenCycles[cycleId]; exists {
				continue
			}
			seenCycles[cycleId] = struct{}{}
			filter := tx.Model(&CodexUsageBucket{}).Where(
				"user_id = ? AND account_hash = ? AND cycle_id = ? AND bucket_minute = ? AND pending_weight >= ?",
				userId,
				accountHash,
				cycleId,
				bucketMinute,
				provisionalWeight,
			)
			result := filter.Updates(map[string]interface{}{
				"weight":         gorm.Expr("weight + ?", finalWeight),
				"pending_weight": gorm.Expr("pending_weight - ?", provisionalWeight),
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errors.New("Codex quota tracking bucket is no longer available")
			}
			if finalWeight == 0 {
				if err := tx.Where(
					"user_id = ? AND account_hash = ? AND cycle_id = ? AND bucket_minute = ? AND weight <= ? AND pending_weight <= ?",
					userId,
					accountHash,
					cycleId,
					bucketMinute,
					0,
					0,
				).Delete(&CodexUsageBucket{}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func MigrateCodexQuotaUsageBucketIndex() error {
	const legacyIndex = "idx_codex_usage_bucket"
	const currentIndex = "idx_codex_usage_bucket_account_minute"
	migrator := DB.Migrator()
	if !migrator.HasIndex(&CodexUsageBucket{}, currentIndex) {
		if err := migrator.CreateIndex(&CodexUsageBucket{}, currentIndex); err != nil {
			return err
		}
	}
	if migrator.HasIndex(&CodexUsageBucket{}, legacyIndex) {
		if err := migrator.DropIndex(&CodexUsageBucket{}, legacyIndex); err != nil {
			return err
		}
	}
	return nil
}

func GetCodexQuotaPolicy(userId int) (shareBps int, bonusBps int, role int, err error) {
	var user User
	err = DB.Select("role", "codex_quota_share_bps", "codex_quota_bonus_bps").First(&user, userId).Error
	return user.CodexQuotaShareBps, user.CodexQuotaBonusBps, user.Role, err
}

func UpdateCodexQuotaPolicy(userId int, shareBps int, bonusBps int) error {
	if shareBps < 0 || bonusBps < 0 || shareBps+bonusBps > CodexQuotaMaxBps {
		return errors.New("Codex quota share must be between 0 and 100%")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		state := CodexQuotaSyncState{Id: 1}
		if err := tx.FirstOrCreate(&state, CodexQuotaSyncState{Id: 1}).Error; err != nil {
			return err
		}
		if err := tx.Model(&CodexQuotaSyncState{}).Where("id = ?", 1).
			Update("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var target User
		if err := tx.Select("id", "role").First(&target, userId).Error; err != nil {
			return err
		}
		if target.Role != common.RoleCommonUser {
			return errors.New("Codex quota allocation only applies to common users")
		}

		return tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
			"codex_quota_share_bps": shareBps,
			"codex_quota_bonus_bps": bonusBps,
		}).Error
	})
}

func GetCodexQuotaSyncState() (CodexQuotaSyncState, error) {
	state := CodexQuotaSyncState{Id: 1}
	err := DB.FirstOrCreate(&state, CodexQuotaSyncState{Id: 1}).Error
	return state, err
}
