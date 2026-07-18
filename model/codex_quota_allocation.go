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
	WindowType        string `json:"window_type" gorm:"type:varchar(16);not null;index:idx_codex_cycle,unique"`
	ResetAt           int64  `json:"reset_at" gorm:"type:bigint;not null;index:idx_codex_cycle,unique;index"`
	Generation        int    `json:"generation" gorm:"type:int;not null;default:1;index:idx_codex_cycle,unique"`
	CapacityUnits     int64  `json:"capacity_units" gorm:"type:bigint;not null"`
	UpstreamUsedUnits int64  `json:"upstream_used_units" gorm:"type:bigint;not null;default:0"`
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
	Id           int64 `json:"id"`
	UserId       int   `json:"user_id" gorm:"not null;index:idx_codex_usage_bucket,unique"`
	BucketMinute int64 `json:"bucket_minute" gorm:"type:bigint;not null;index:idx_codex_usage_bucket,unique;index"`
	Weight       int64 `json:"weight" gorm:"type:bigint;not null;default:0"`
}

type CodexQuotaSyncState struct {
	Id               int   `json:"id" gorm:"primaryKey"`
	LastSuccessAt    int64 `json:"last_success_at" gorm:"type:bigint;not null;default:0"`
	LastBucketMinute int64 `json:"last_bucket_minute" gorm:"type:bigint;not null;default:0"`
	IncludedCount    int   `json:"included_count" gorm:"type:int;not null;default:0"`
	ExcludedCount    int   `json:"excluded_count" gorm:"type:int;not null;default:0"`
}

func RecordCodexUsageWeight(userId int, weight int64) error {
	if userId <= 0 || weight <= 0 {
		return nil
	}
	bucket := time.Now().Unix() / 60 * 60
	row := CodexUsageBucket{UserId: userId, BucketMinute: bucket, Weight: weight}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "bucket_minute"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"weight": gorm.Expr("weight + ?", weight),
		}),
	}).Create(&row).Error
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

		var allocated int64
		if err := tx.Model(&User{}).
			Where("role = ? AND id <> ?", common.RoleCommonUser, userId).
			Select("COALESCE(SUM(codex_quota_share_bps + codex_quota_bonus_bps), 0)").
			Scan(&allocated).Error; err != nil {
			return err
		}
		if allocated+int64(shareBps+bonusBps) > CodexQuotaMaxBps {
			return errors.New("total Codex quota allocation exceeds 100%")
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
