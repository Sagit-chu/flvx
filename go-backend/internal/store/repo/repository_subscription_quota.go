package repo

import (
	"errors"
	"time"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type subscriptionQuotaUsageKey struct {
	UserID   int64
	TunnelID int64
}

func subscriptionQuotaDayKey(now time.Time) int64 {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).UnixMilli()
}

func subscriptionQuotaMonthKey(now time.Time) int64 {
	y, m, _ := now.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, now.Location()).UnixMilli()
}

func applySubscriptionQuotaWindowRoll(q *model.UserSubscriptionQuota, now time.Time) {
	dayKey := subscriptionQuotaDayKey(now)
	monthKey := subscriptionQuotaMonthKey(now)
	if q.DayKey != dayKey {
		q.DayKey = dayKey
		q.DailyUsedBytes = 0
	}
	if q.MonthKey != monthKey {
		q.MonthKey = monthKey
		q.MonthlyUsedBytes = 0
	}
}

func (r *Repository) UpsertSubscriptionQuotaConfigTx(tx *gorm.DB, sub model.UserSubscription, plan model.Plan, now int64) error {
	if r == nil || tx == nil {
		return errors.New("repository not initialized")
	}
	current := time.UnixMilli(now)
	item := model.UserSubscriptionQuota{
		SubscriptionID: sub.ID,
		UserID:         sub.UserID,
		PlanID:         sub.PlanID,
		DailyLimitGB:   plan.DailyQuotaGB,
		MonthlyLimitGB: plan.MonthlyQuotaGB,
		DayKey:         subscriptionQuotaDayKey(current),
		MonthKey:       subscriptionQuotaMonthKey(current),
		CreatedTime:    now,
		UpdatedTime:    now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "subscription_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id", "plan_id", "daily_limit_gb", "monthly_limit_gb", "updated_time",
		}),
	}).Create(&item).Error
}

func (r *Repository) ResetSubscriptionQuotaUsage(subscriptionID int64, now time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if subscriptionID <= 0 {
		return errors.New("subscription id is required")
	}
	nowMs := now.UnixMilli()
	return r.db.Model(&model.UserSubscriptionQuota{}).Where("subscription_id = ?", subscriptionID).Updates(map[string]interface{}{
		"daily_used_bytes":   int64(0),
		"monthly_used_bytes": int64(0),
		"day_key":            subscriptionQuotaDayKey(now),
		"month_key":          subscriptionQuotaMonthKey(now),
		"updated_time":       nowMs,
	}).Error
}

func (r *Repository) AddSubscriptionQuotaUsageBatch(deltas []FlowUploadCounterDelta, now time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	usages := make(map[subscriptionQuotaUsageKey]int64)
	for _, delta := range deltas {
		if delta.UserID <= 0 || delta.TunnelID <= 0 {
			continue
		}
		used := delta.InFlow + delta.OutFlow
		if used <= 0 {
			continue
		}
		key := subscriptionQuotaUsageKey{UserID: delta.UserID, TunnelID: delta.TunnelID}
		usages[key] += used
	}
	if len(usages) == 0 {
		return nil
	}

	nowMs := now.UnixMilli()
	return r.db.Transaction(func(tx *gorm.DB) error {
		for key, used := range usages {
			sub, plan, err := r.activeSubscriptionForTunnelTx(tx, key.UserID, key.TunnelID, nowMs)
			if err != nil {
				return err
			}
			if sub.ID <= 0 {
				continue
			}
			if err := r.UpsertSubscriptionQuotaConfigTx(tx, sub, plan, nowMs); err != nil {
				return err
			}
			var quota model.UserSubscriptionQuota
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("subscription_id = ?", sub.ID).First(&quota).Error; err != nil {
				return err
			}
			applySubscriptionQuotaWindowRoll(&quota, now)
			quota.DailyUsedBytes += used
			quota.MonthlyUsedBytes += used
			quota.UpdatedTime = nowMs
			if err := tx.Model(&model.UserSubscriptionQuota{}).Where("subscription_id = ?", quota.SubscriptionID).Updates(map[string]interface{}{
				"daily_used_bytes":   quota.DailyUsedBytes,
				"monthly_used_bytes": quota.MonthlyUsedBytes,
				"day_key":            quota.DayKey,
				"month_key":          quota.MonthKey,
				"updated_time":       quota.UpdatedTime,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) activeSubscriptionForTunnelTx(tx *gorm.DB, userID, tunnelID, nowMs int64) (model.UserSubscription, model.Plan, error) {
	var subs []model.UserSubscription
	if err := tx.Where("user_id = ? AND status = ? AND expires_at > ?", userID, "active", nowMs).
		Order("expires_at ASC, id ASC").Find(&subs).Error; err != nil {
		return model.UserSubscription{}, model.Plan{}, err
	}
	for _, sub := range subs {
		var plan model.Plan
		if err := tx.Where("id = ?", sub.PlanID).First(&plan).Error; err != nil {
			return model.UserSubscription{}, model.Plan{}, err
		}
		ok, err := r.planHasTunnelTx(tx, sub.PlanID, tunnelID)
		if err != nil {
			return model.UserSubscription{}, model.Plan{}, err
		}
		if ok {
			return sub, plan, nil
		}
	}
	return model.UserSubscription{}, model.Plan{}, nil
}

func (r *Repository) planHasTunnelTx(tx *gorm.DB, planID, tunnelID int64) (bool, error) {
	var direct int64
	if err := tx.Model(&model.PlanEntitlement{}).
		Where("plan_id = ? AND scope_type = ? AND scope_id = ?", planID, "tunnel", tunnelID).
		Count(&direct).Error; err != nil {
		return false, err
	}
	if direct > 0 {
		return true, nil
	}
	var group int64
	err := tx.Table("plan_entitlement").
		Joins("JOIN tunnel_group_tunnel ON tunnel_group_tunnel.tunnel_group_id = plan_entitlement.scope_id").
		Where("plan_entitlement.plan_id = ? AND plan_entitlement.scope_type = ? AND tunnel_group_tunnel.tunnel_id = ?", planID, "tunnel_group", tunnelID).
		Count(&group).Error
	return group > 0, err
}
