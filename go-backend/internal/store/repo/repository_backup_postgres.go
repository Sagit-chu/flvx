package repo

import (
	"errors"
	"fmt"
	"time"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
)

var errBackupImportBusy = errors.New("database is busy; retry the backup import")

// Only retry lock acquisition failures, which occur before any backup writes.
// NOWAIT releases partial lock sets instead of deadlocking with existing
// child-before-parent deletes or forward-before-user traffic updates.
func backupImportTransaction(db *gorm.DB, importRows func(*gorm.DB) error) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		err = db.Transaction(importRows)
		if !errors.Is(err, errBackupImportBusy) {
			return err
		}
		if attempt < 9 {
			time.Sleep(min(10*time.Millisecond<<attempt, 100*time.Millisecond))
		}
	}
	return err
}

// Lock every table that import can write, including generated-ID child tables.
// No writes happen until the complete set is held. Existing CRUD transactions
// can therefore finish regardless of their own parent/child mutation order.
// The returned subset contains only tables receiving explicit backup IDs.
func lockPostgresBackupTables(tx *gorm.DB, backup *model.BackupData, types map[string]bool) ([]string, error) {
	if tx.Dialector.Name() != "postgres" {
		return nil, nil
	}
	chains, grants, ports := 0, 0, 0
	for _, tunnel := range backup.Tunnels {
		chains += len(tunnel.ChainTunnels)
	}
	for _, permission := range backup.Permissions {
		grants += len(permission.Grants)
	}
	for _, forward := range backup.Forwards {
		if forward.ForwardPorts != nil {
			ports++
		}
	}
	var tables []string
	for _, item := range []struct {
		kind        string
		count       int
		name        string
		explicitIDs bool
	}{
		{"users", len(backup.Users), "user", true},
		{"speedLimits", len(backup.SpeedLimits), "speed_limit", true},
		{"nodes", len(backup.Nodes), "node", true},
		{"tunnels", len(backup.Tunnels), "tunnel", true},
		{"tunnels", chains, "chain_tunnel", true},
		{"forwards", len(backup.Forwards), "forward", true},
		{"userTunnels", len(backup.UserTunnels), "user_tunnel", true},
		{"tunnelGroups", len(backup.TunnelGroups), "tunnel_group", true},
		{"userGroups", len(backup.UserGroups), "user_group", true},
		{"permissions", len(backup.Permissions), "group_permission", true},
		{"permissions", grants, "group_permission_grant", true},
		{"users", len(backup.Users), "user_quota", false},
		{"forwards", ports, "forward_port", false},
		{"tunnelGroups", len(backup.TunnelGroups), "tunnel_group_tunnel", false},
		{"userGroups", len(backup.UserGroups), "user_group_user", false},
		{"configs", len(FilterBackupConfigs(backup.Configs)), "vite_config", false},
	} {
		if !types[item.kind] || item.count == 0 {
			continue
		}
		// Resolve the same relation as normal unqualified repository queries,
		// even when the first search_path schema has no copy of this table.
		var relation struct {
			Schema string
			Name   string
		}
		if err := tx.Raw(`SELECT n.nspname AS schema, c.relname AS name
			FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
			WHERE c.oid = pg_catalog.to_regclass(?)`, quoteSQLIdentifier(item.name)).Scan(&relation).Error; err != nil {
			return nil, err
		}
		if relation.Name == "" {
			return nil, fmt.Errorf("missing import table %s", item.name)
		}
		table := quoteSQLIdentifier(relation.Schema) + "." + quoteSQLIdentifier(relation.Name)
		if err := tx.Exec("LOCK TABLE " + table + " IN SHARE ROW EXCLUSIVE MODE NOWAIT").Error; err != nil {
			var pgError interface{ SQLState() string }
			if errors.As(err, &pgError) && pgError.SQLState() == "55P03" {
				return nil, fmt.Errorf("%w: %w", errBackupImportBusy, err)
			}
			return nil, err
		}
		if item.explicitIDs {
			tables = append(tables, table)
		}
	}
	return tables, nil
}

func advancePostgresBackupSequences(tx *gorm.DB, tables []string) error {
	for _, table := range tables {
		if err := advancePostgresBackupSequence(tx, table); err != nil {
			return fmt.Errorf("advance imported %s id sequence: %w", table, err)
		}
	}
	return nil
}

// The caller holds the table's SHARE ROW EXCLUSIVE lock until commit. Unlike
// startup's setval repair, RESTART is transactional and invalidates other
// connections' cached values. Never return to IDs already allocated, even if
// their rows were deleted or their transactions rolled back.
func advancePostgresBackupSequence(tx *gorm.DB, table string) error {
	var maxID int64
	if err := tx.Raw("SELECT COALESCE(MAX(id), 0) FROM " + table).Scan(&maxID).Error; err != nil {
		return err
	}
	if maxID <= 0 {
		return nil
	}
	type sequenceInfo struct {
		Schema    string
		Name      string
		Cache     int64
		Increment int64
		Maximum   int64
	}
	var sequences []sequenceInfo
	// Prefer the actual default's catalog dependency: a legacy custom nextval
	// default need not own its sequence, and the old owned serial may remain.
	// Identity columns have no pg_attrdef, so use their owned sequence instead.
	if err := tx.Raw(`WITH defaults AS (
		SELECT dep.refobjid AS oid FROM pg_catalog.pg_attrdef def
		JOIN pg_catalog.pg_attribute attr ON attr.attrelid = def.adrelid AND attr.attnum = def.adnum
		JOIN pg_catalog.pg_depend dep ON dep.classid = 'pg_catalog.pg_attrdef'::regclass
			AND dep.objid = def.oid AND dep.refclassid = 'pg_catalog.pg_class'::regclass
		JOIN pg_catalog.pg_sequence seq ON seq.seqrelid = dep.refobjid
		WHERE def.adrelid = ?::regclass AND attr.attname = 'id'
	), candidates AS (
		SELECT oid FROM defaults
		UNION SELECT pg_catalog.pg_get_serial_sequence(?, 'id')::regclass WHERE NOT EXISTS (SELECT 1 FROM defaults)
	)
	SELECT n.nspname AS schema, c.relname AS name,
		s.seqcache AS cache, s.seqincrement AS increment, s.seqmax AS maximum
		FROM pg_catalog.pg_sequence s
		JOIN pg_catalog.pg_class c ON c.oid = s.seqrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE s.seqrelid IN (SELECT oid FROM candidates)`, table, table).Scan(&sequences).Error; err != nil {
		return err
	}
	if len(sequences) != 1 || sequences[0].Increment <= 0 {
		return fmt.Errorf("missing or ambiguous ascending ID sequence")
	}
	sequence := sequences[0]
	name := quoteSQLIdentifier(sequence.Schema) + "." + quoteSQLIdentifier(sequence.Name)
	// Reapplying CACHE without changing its setting takes a sequence DDL lock.
	// Read last_value only after that lock: even direct concurrent nextval calls
	// must finish first. The table locks already exclude normal INSERTs.
	if err := tx.Exec(fmt.Sprintf("ALTER SEQUENCE %s CACHE %d", name, sequence.Cache)).Error; err != nil {
		return err
	}
	var state struct {
		LastValue int64
		IsCalled  bool
	}
	if err := tx.Raw("SELECT last_value, is_called FROM " + name).Scan(&state).Error; err != nil {
		return err
	}
	if state.LastValue < 1 {
		return fmt.Errorf("ID sequence must allocate positive IDs")
	}
	if maxID >= sequence.Maximum || (state.IsCalled && state.LastValue > sequence.Maximum-sequence.Increment) {
		return fmt.Errorf("ID sequence exhausted")
	}
	next := state.LastValue
	if state.IsCalled {
		next += sequence.Increment
	}
	if next <= maxID {
		steps := (maxID-next)/sequence.Increment + 1
		if steps > (sequence.Maximum-next)/sequence.Increment {
			return fmt.Errorf("ID sequence exhausted")
		}
		next += steps * sequence.Increment
	}
	return tx.Exec(fmt.Sprintf("ALTER SEQUENCE %s RESTART WITH %d", name, next)).Error
}
