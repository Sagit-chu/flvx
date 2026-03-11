package repo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const tunnelQuotaBytesPerGB int64 = 1024 * 1024 * 1024

type TunnelQuotaRelease struct {
	TunnelID     int64
	ForwardIDs   []int64
	EnableTunnel bool
}

func tunnelQuotaWindowKeys(now time.Time) (int64, int64) {
	return int64(now.Year()*10000 + int(now.Month())*100 + now.Day()), int64(now.Year()*100 + int(now.Month()))
}

func cloneTunnelQuotaView(q model.TunnelQuota) *model.TunnelQuotaView {
	return &model.TunnelQuotaView{
		TunnelID:         q.TunnelID,
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

func normalizeTunnelQuotaView(view *model.TunnelQuotaView, now time.Time) *model.TunnelQuotaView {
	if view == nil {
		return nil
	}
	dayKey, monthKey := tunnelQuotaWindowKeys(now)
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

func tunnelQuotaExceeded(view *model.TunnelQuotaView) bool {
	if view == nil {
		return false
	}
	if view.DailyLimitGB > 0 && view.DailyUsedBytes >= view.DailyLimitGB*tunnelQuotaBytesPerGB {
		return true
	}
	if view.MonthlyLimitGB > 0 && view.MonthlyUsedBytes >= view.MonthlyLimitGB*tunnelQuotaBytesPerGB {
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

func (r *Repository) loadOrCreateTunnelQuotaTx(tx *gorm.DB, tunnelID int64, now time.Time) (*model.TunnelQuota, error) {
	if tx == nil {
		return nil, errors.New("database unavailable")
	}
	dayKey, monthKey := tunnelQuotaWindowKeys(now)
	q := &model.TunnelQuota{}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tunnel_id = ?", tunnelID).First(q).Error
	if err == nil {
		return q, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	nowMs := now.UnixMilli()
	q = &model.TunnelQuota{
		TunnelID:         tunnelID,
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

func applyTunnelQuotaWindowRoll(q *model.TunnelQuota, now time.Time) bool {
	if q == nil {
		return false
	}
	changed := false
	dayKey, monthKey := tunnelQuotaWindowKeys(now)
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

func (r *Repository) SaveTunnelQuotaConfigTx(tx *gorm.DB, tunnelID, dailyLimitGB, monthlyLimitGB int64, now int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	if tunnelID <= 0 {
		return errors.New("tunnel id is required")
	}
	if dailyLimitGB < 0 || monthlyLimitGB < 0 {
		return errors.New("quota limit cannot be negative")
	}
	current := time.UnixMilli(now)
	q, err := r.loadOrCreateTunnelQuotaTx(tx, tunnelID, current)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"daily_limit_gb":   dailyLimitGB,
		"monthly_limit_gb": monthlyLimitGB,
		"updated_time":     now,
	}
	if q.DayKey == 0 || q.MonthKey == 0 {
		dayKey, monthKey := tunnelQuotaWindowKeys(current)
		updates["day_key"] = dayKey
		updates["month_key"] = monthKey
	}
	return tx.Model(&model.TunnelQuota{}).Where("tunnel_id = ?", tunnelID).Updates(updates).Error
}

func (r *Repository) ListTunnelQuotaViewsByTunnelIDs(tunnelIDs []int64, now time.Time) (map[int64]*model.TunnelQuotaView, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	out := make(map[int64]*model.TunnelQuotaView)
	if len(tunnelIDs) == 0 {
		return out, nil
	}
	var rows []model.TunnelQuota
	if err := r.db.Where("tunnel_id IN ?", tunnelIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.TunnelID] = normalizeTunnelQuotaView(cloneTunnelQuotaView(row), now)
	}
	return out, nil
}

func (r *Repository) GetTunnelQuotaView(tunnelID int64, now time.Time) (*model.TunnelQuotaView, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if tunnelID <= 0 {
		return nil, nil
	}
	var row model.TunnelQuota
	err := r.db.Where("tunnel_id = ?", tunnelID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return normalizeTunnelQuotaView(cloneTunnelQuotaView(row), now), nil
}

func (r *Repository) AddTunnelQuotaUsage(tunnelID int64, usedBytes int64, now time.Time) (*model.TunnelQuotaView, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if tunnelID <= 0 {
		return nil, nil
	}
	result := &model.TunnelQuotaView{}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		q, err := r.loadOrCreateTunnelQuotaTx(tx, tunnelID, now)
		if err != nil {
			return err
		}
		applyTunnelQuotaWindowRoll(q, now)
		if usedBytes > 0 {
			q.DailyUsedBytes += usedBytes
			q.MonthlyUsedBytes += usedBytes
		}
		q.UpdatedTime = now.UnixMilli()
		if err := tx.Model(&model.TunnelQuota{}).Where("tunnel_id = ?", tunnelID).Updates(map[string]interface{}{
			"daily_used_bytes":   q.DailyUsedBytes,
			"monthly_used_bytes": q.MonthlyUsedBytes,
			"day_key":            q.DayKey,
			"month_key":          q.MonthKey,
			"updated_time":       q.UpdatedTime,
		}).Error; err != nil {
			return err
		}
		*result = *cloneTunnelQuotaView(*q)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return normalizeTunnelQuotaView(result, now), nil
}

func (r *Repository) MarkTunnelQuotaDisabled(tunnelID int64, pausedForwardIDs []int64, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if tunnelID <= 0 {
		return errors.New("tunnel id is required")
	}
	return r.db.Model(&model.TunnelQuota{}).Where("tunnel_id = ?", tunnelID).Updates(map[string]interface{}{
		"disabled_by_quota":  1,
		"disabled_at":        now,
		"paused_forward_ids": joinPausedForwardIDs(pausedForwardIDs),
		"updated_time":       now,
	}).Error
}

func (r *Repository) ResetTunnelQuotaUsage(tunnelID int64, scope string, now time.Time) (*TunnelQuotaRelease, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if tunnelID <= 0 {
		return nil, errors.New("tunnel id is required")
	}
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope == "" {
		scope = "all"
	}
	if scope != "daily" && scope != "monthly" && scope != "all" {
		return nil, fmt.Errorf("unsupported quota reset scope: %s", scope)
	}
	var release *TunnelQuotaRelease
	err := r.db.Transaction(func(tx *gorm.DB) error {
		q, err := r.loadOrCreateTunnelQuotaTx(tx, tunnelID, now)
		if err != nil {
			return err
		}
		applyTunnelQuotaWindowRoll(q, now)
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
		release = &TunnelQuotaRelease{TunnelID: tunnelID}
		if q.DisabledByQuota == 1 && !tunnelQuotaExceeded(cloneTunnelQuotaView(*q)) {
			release.EnableTunnel = true
			release.ForwardIDs = parsePausedForwardIDs(q.PausedForwardIDs)
			q.DisabledByQuota = 0
			q.DisabledAt = 0
			q.PausedForwardIDs = ""
		}
		return tx.Model(&model.TunnelQuota{}).Where("tunnel_id = ?", tunnelID).Updates(map[string]interface{}{
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

func (r *Repository) RollTunnelQuotaWindows(now time.Time) ([]TunnelQuotaRelease, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var releases []TunnelQuotaRelease
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var rows []model.TunnelQuota
		if err := tx.Find(&rows).Error; err != nil {
			return err
		}
		nowMs := now.UnixMilli()
		for _, row := range rows {
			q := row
			changed := applyTunnelQuotaWindowRoll(&q, now)
			release := TunnelQuotaRelease{TunnelID: q.TunnelID}
			if q.DisabledByQuota == 1 && !tunnelQuotaExceeded(cloneTunnelQuotaView(q)) {
				release.EnableTunnel = true
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
			if err := tx.Model(&model.TunnelQuota{}).Where("tunnel_id = ?", q.TunnelID).Updates(map[string]interface{}{
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
			if release.EnableTunnel {
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

func (r *Repository) UpdateTunnelStatus(tunnelID int64, status int, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if tunnelID <= 0 {
		return errors.New("tunnel id is required")
	}
	return r.db.Model(&model.Tunnel{}).Where("id = ?", tunnelID).Updates(map[string]interface{}{
		"status":       status,
		"updated_time": now,
	}).Error
}
