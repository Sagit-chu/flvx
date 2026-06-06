package repo

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const userQuotaBytesPerGB int64 = 1024 * 1024 * 1024

type UserQuotaRelease struct {
	UserID      int64
	ForwardIDs  []int64
	UnblockUser bool
}

func userQuotaWindowKeys(now time.Time) (int64, int64) {
	return int64(now.Year()*10000 + int(now.Month())*100 + now.Day()), int64(now.Year()*100 + int(now.Month()))
}

func cloneUserQuotaView(q model.UserQuota) *model.UserQuotaView {
	return &model.UserQuotaView{
		UserID:           q.UserID,
		DailyLimitGB:     q.DailyLimitGB,
		MonthlyLimitGB:   q.MonthlyLimitGB,
		DailyUsedBytes:   q.DailyUsedBytes,
		MonthlyUsedBytes: q.MonthlyUsedBytes,
		DayKey:           q.DayKey,
		MonthKey:         q.MonthKey,
		DisabledByQuota:  q.DisabledByQuota,
		DisabledAt:       q.DisabledAt,
		PausedForwardIDs: q.PausedForwardIDs,
	}
}

func normalizeUserQuotaView(view *model.UserQuotaView, now time.Time) *model.UserQuotaView {
	if view == nil {
		return nil
	}
	dayKey, monthKey := userQuotaWindowKeys(now)
	out := *view
	if out.DayKey != dayKey {
		out.DayKey = dayKey
		out.DailyUsedBytes = 0
	}
	if out.MonthKey != monthKey {
		out.MonthKey = monthKey
		out.MonthlyUsedBytes = 0
	}
	return &out
}

func userQuotaExceeded(view *model.UserQuotaView) bool {
	if view == nil {
		return false
	}
	if view.DailyLimitGB > 0 && view.DailyUsedBytes >= view.DailyLimitGB*userQuotaBytesPerGB {
		return true
	}
	if view.MonthlyLimitGB > 0 && view.MonthlyUsedBytes >= view.MonthlyLimitGB*userQuotaBytesPerGB {
		return true
	}
	return false
}

func parsePausedForwardIDs(raw string) []int64 {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	out := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func joinPausedForwardIDs(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func (r *Repository) loadOrCreateUserQuotaTx(tx *gorm.DB, userID int64, now time.Time) (*model.UserQuota, error) {
	if tx == nil {
		return nil, errors.New("database unavailable")
	}
	dayKey, monthKey := userQuotaWindowKeys(now)
	q := &model.UserQuota{}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(q).Error
	if err == nil {
		return q, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	nowMs := now.UnixMilli()
	q = &model.UserQuota{
		UserID:           userID,
		DayKey:           dayKey,
		MonthKey:         monthKey,
		CreatedTime:      nowMs,
		UpdatedTime:      nowMs,
		PausedForwardIDs: "",
	}
	if err := tx.Create(q).Error; err != nil {
		return nil, err
	}
	return q, nil
}

func applyUserQuotaWindowRoll(q *model.UserQuota, now time.Time) bool {
	if q == nil {
		return false
	}
	changed := false
	dayKey, monthKey := userQuotaWindowKeys(now)
	if q.DayKey != dayKey {
		q.DayKey = dayKey
		q.DailyUsedBytes = 0
		changed = true
	}
	if q.MonthKey != monthKey {
		q.MonthKey = monthKey
		q.MonthlyUsedBytes = 0
		changed = true
	}
	return changed
}

func (r *Repository) SaveUserQuotaConfigTx(tx *gorm.DB, userID, dailyLimitGB, monthlyLimitGB int64, now int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	if userID <= 0 {
		return errors.New("user id is required")
	}
	if dailyLimitGB < 0 || monthlyLimitGB < 0 {
		return errors.New("quota limit cannot be negative")
	}
	current := time.UnixMilli(now)
	q, err := r.loadOrCreateUserQuotaTx(tx, userID, current)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"daily_limit_gb":   dailyLimitGB,
		"monthly_limit_gb": monthlyLimitGB,
		"updated_time":     now,
	}
	if q.DayKey == 0 || q.MonthKey == 0 {
		dayKey, monthKey := userQuotaWindowKeys(current)
		updates["day_key"] = dayKey
		updates["month_key"] = monthKey
	}
	return tx.Model(&model.UserQuota{}).Where("user_id = ?", userID).Updates(updates).Error
}

func (r *Repository) ListUserQuotaViewsByUserIDs(userIDs []int64, now time.Time) (map[int64]*model.UserQuotaView, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	out := make(map[int64]*model.UserQuotaView)
	if len(userIDs) == 0 {
		return out, nil
	}
	var rows []model.UserQuota
	if err := r.db.Where("user_id IN ?", userIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = normalizeUserQuotaView(cloneUserQuotaView(row), now)
	}
	return out, nil
}

func (r *Repository) GetUserQuotaView(userID int64, now time.Time) (*model.UserQuotaView, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if userID <= 0 {
		return nil, nil
	}
	var row model.UserQuota
	err := r.db.Where("user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return normalizeUserQuotaView(cloneUserQuotaView(row), now), nil
}

func (r *Repository) AddUserQuotaUsage(userID int64, usedBytes int64, now time.Time) (*model.UserQuotaView, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if userID <= 0 {
		return nil, nil
	}
	result := &model.UserQuotaView{}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		q, err := r.loadOrCreateUserQuotaTx(tx, userID, now)
		if err != nil {
			return err
		}
		applyUserQuotaWindowRoll(q, now)
		if usedBytes > 0 {
			q.DailyUsedBytes += usedBytes
			q.MonthlyUsedBytes += usedBytes
		}
		q.UpdatedTime = now.UnixMilli()
		if err := tx.Model(&model.UserQuota{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
			"daily_used_bytes":   q.DailyUsedBytes,
			"monthly_used_bytes": q.MonthlyUsedBytes,
			"day_key":            q.DayKey,
			"month_key":          q.MonthKey,
			"updated_time":       q.UpdatedTime,
		}).Error; err != nil {
			return err
		}
		*result = *cloneUserQuotaView(*q)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return normalizeUserQuotaView(result, now), nil
}

func (r *Repository) AddUserQuotaUsageBatch(usages map[int64]int64, now time.Time) (map[int64]*model.UserQuotaView, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if len(usages) == 0 {
		return map[int64]*model.UserQuotaView{}, nil
	}

	var result map[int64]*model.UserQuotaView
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = r.addUserQuotaUsageBatchTx(tx, usages, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) addUserQuotaUsageBatchTx(tx *gorm.DB, usages map[int64]int64, now time.Time) (map[int64]*model.UserQuotaView, error) {
	if tx == nil {
		return nil, errors.New("database unavailable")
	}
	if len(usages) == 0 {
		return map[int64]*model.UserQuotaView{}, nil
	}

	result := make(map[int64]*model.UserQuotaView, len(usages))
	userIDs := make([]int64, 0, len(usages))
	for userID := range usages {
		if userID > 0 {
			userIDs = append(userIDs, userID)
		}
	}
	sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })

	for _, userID := range userIDs {
		q, err := r.loadOrCreateUserQuotaTx(tx, userID, now)
		if err != nil {
			return nil, err
		}
		applyUserQuotaWindowRoll(q, now)
		if usages[userID] > 0 {
			q.DailyUsedBytes += usages[userID]
			q.MonthlyUsedBytes += usages[userID]
		}
		q.UpdatedTime = now.UnixMilli()
		if err := tx.Model(&model.UserQuota{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
			"daily_used_bytes":   q.DailyUsedBytes,
			"monthly_used_bytes": q.MonthlyUsedBytes,
			"day_key":            q.DayKey,
			"month_key":          q.MonthKey,
			"updated_time":       q.UpdatedTime,
		}).Error; err != nil {
			return nil, err
		}
		result[userID] = normalizeUserQuotaView(cloneUserQuotaView(*q), now)
	}
	return result, nil
}

func (r *Repository) MarkUserQuotaDisabled(userID int64, pausedForwardIDs []int64, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if userID <= 0 {
		return errors.New("user id is required")
	}
	return r.db.Model(&model.UserQuota{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
		"disabled_by_quota":  1,
		"disabled_at":        now,
		"paused_forward_ids": joinPausedForwardIDs(pausedForwardIDs),
		"updated_time":       now,
	}).Error
}

func (r *Repository) ResetUserQuotaUsage(userID int64, scope string, now time.Time) (*UserQuotaRelease, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if userID <= 0 {
		return nil, errors.New("user id is required")
	}
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope == "" {
		scope = "all"
	}
	if scope != "daily" && scope != "monthly" && scope != "all" {
		return nil, fmt.Errorf("unsupported quota reset scope: %s", scope)
	}
	var release *UserQuotaRelease
	err := r.db.Transaction(func(tx *gorm.DB) error {
		q, err := r.loadOrCreateUserQuotaTx(tx, userID, now)
		if err != nil {
			return err
		}
		applyUserQuotaWindowRoll(q, now)
		switch scope {
		case "daily":
			q.DailyUsedBytes = 0
		case "monthly":
			q.MonthlyUsedBytes = 0
		case "all":
			q.DailyUsedBytes = 0
			q.MonthlyUsedBytes = 0
		}
		q.UpdatedTime = now.UnixMilli()
		release = &UserQuotaRelease{UserID: userID}
		if q.DisabledByQuota == 1 && !userQuotaExceeded(cloneUserQuotaView(*q)) {
			release.UnblockUser = true
			release.ForwardIDs = parsePausedForwardIDs(q.PausedForwardIDs)
			q.DisabledByQuota = 0
			q.DisabledAt = 0
			q.PausedForwardIDs = ""
		}
		return tx.Model(&model.UserQuota{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
			"daily_used_bytes":   q.DailyUsedBytes,
			"monthly_used_bytes": q.MonthlyUsedBytes,
			"day_key":            q.DayKey,
			"month_key":          q.MonthKey,
			"disabled_by_quota":  q.DisabledByQuota,
			"disabled_at":        q.DisabledAt,
			"paused_forward_ids": q.PausedForwardIDs,
			"updated_time":       q.UpdatedTime,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return release, nil
}

func (r *Repository) RollUserQuotaWindows(now time.Time) ([]UserQuotaRelease, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var releases []UserQuotaRelease
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var rows []model.UserQuota
		if err := tx.Find(&rows).Error; err != nil {
			return err
		}
		nowMs := now.UnixMilli()
		for _, row := range rows {
			q := row
			changed := applyUserQuotaWindowRoll(&q, now)
			release := UserQuotaRelease{UserID: q.UserID}
			if q.DisabledByQuota == 1 && !userQuotaExceeded(cloneUserQuotaView(q)) {
				release.UnblockUser = true
				release.ForwardIDs = parsePausedForwardIDs(q.PausedForwardIDs)
				q.DisabledByQuota = 0
				q.DisabledAt = 0
				q.PausedForwardIDs = ""
				changed = true
			}
			if !changed {
				continue
			}
			q.UpdatedTime = nowMs
			if err := tx.Model(&model.UserQuota{}).Where("user_id = ?", q.UserID).Updates(map[string]interface{}{
				"daily_used_bytes":   q.DailyUsedBytes,
				"monthly_used_bytes": q.MonthlyUsedBytes,
				"day_key":            q.DayKey,
				"month_key":          q.MonthKey,
				"disabled_by_quota":  q.DisabledByQuota,
				"disabled_at":        q.DisabledAt,
				"paused_forward_ids": q.PausedForwardIDs,
				"updated_time":       q.UpdatedTime,
			}).Error; err != nil {
				return err
			}
			if release.UnblockUser {
				releases = append(releases, release)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return releases, nil
}
