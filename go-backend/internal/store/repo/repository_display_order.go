package repo

import (
	"errors"
	"gorm.io/gorm"
	"slices"
)

var ErrDisplayOrderConflict = errors.New("列表已变更，请刷新后重试")
var ErrInvalidDisplayOrder = errors.New("排序必须包含全部条目且不能重复")

// Lock the entire membership, including an empty list and concurrent INSERTs.
// PostgreSQL row locks alone cannot protect the missing-row/phantom case.
// SQLite's first write acquires its writer reservation before reading the list.
func lockDisplayOrder(tx *gorm.DB, table string) error {
	if tx.Dialector.Name() == "postgres" {
		return tx.Exec("LOCK TABLE \"" + table + "\" IN SHARE ROW EXCLUSIVE MODE").Error
	}
	return tx.Table(table).Where("1 = 0").UpdateColumn("display_order", gorm.Expr("display_order")).Error
}

func (r *Repository) UpdateUserDisplayOrder(ids, expected []int64) error {
	return r.updateDisplayOrder("user", ids, expected)
}
func (r *Repository) UpdateSpeedLimitDisplayOrder(ids, expected []int64) error {
	return r.updateDisplayOrder("speed_limit", ids, expected)
}
func (r *Repository) updateDisplayOrder(table string, ids, expected []int64) error {
	if ids == nil || expected == nil {
		return ErrInvalidDisplayOrder
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return ErrInvalidDisplayOrder
		}
		seen[id] = true
	}
	seenExpected := map[int64]bool{}
	for _, id := range expected {
		if id <= 0 || seenExpected[id] {
			return ErrInvalidDisplayOrder
		}
		seenExpected[id] = true
	}
	if len(ids) != len(expected) {
		return ErrInvalidDisplayOrder
	}
	for id := range seen {
		if !seenExpected[id] {
			return ErrInvalidDisplayOrder
		}
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := lockDisplayOrder(tx, table); err != nil {
			return err
		}
		q := tx.Table(table)
		if table == "user" {
			var admins int64
			if err := tx.Table(table).Where("role_id = ? AND id IN ?", 0, ids).Count(&admins).Error; err != nil {
				return err
			}
			if admins != 0 {
				return ErrInvalidDisplayOrder
			}
			q = q.Where("role_id != ?", 0)
		}
		var current []int64
		if err := q.Order("display_order ASC, id DESC").Pluck("id", &current).Error; err != nil {
			return err
		}
		if !slices.Equal(current, expected) {
			return ErrDisplayOrderConflict
		}
		for i, id := range ids {
			result := tx.Table(table).Where("id = ?", id).UpdateColumn("display_order", int64(i))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrDisplayOrderConflict
			}
		}
		return nil
	})
}

// New items precede all saved ranks. Zero is only the legacy migration default.
func createFirst(tx *gorm.DB, table string, item interface{}, rank *int64) error {
	return tx.Transaction(func(tx *gorm.DB) error {
		if err := lockDisplayOrder(tx, table); err != nil {
			return err
		}
		q := tx.Table(table)
		if table == "user" {
			q = q.Where("role_id != ?", 0)
		}
		var first int64
		if err := q.Select("COALESCE(MIN(display_order), 0)").Scan(&first).Error; err != nil {
			return err
		}
		if first == -1<<63 {
			return errors.New("display order exhausted")
		}
		*rank = first - 1
		return tx.Create(item).Error
	})
}
