package com.admin.common.migration;

import lombok.extern.slf4j.Slf4j;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

import java.util.HashSet;
import java.util.Set;

/**
 * Lightweight SQLite schema migration.
 *
 * Spring Boot SQL init uses CREATE TABLE IF NOT EXISTS, so existing installations
 * won't automatically receive new columns. This runner adds missing columns in-place.
 */
@Slf4j
@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
public class SqliteSchemaMigration implements ApplicationRunner {

    private final JdbcTemplate jdbcTemplate;

    public SqliteSchemaMigration(JdbcTemplate jdbcTemplate) {
        this.jdbcTemplate = jdbcTemplate;
    }

    @Override
    public void run(ApplicationArguments args) {
        ensureColumn("node", "inx", "INTEGER NOT NULL DEFAULT 0");
        ensureColumn("tunnel", "inx", "INTEGER NOT NULL DEFAULT 0");
        ensureTable("CREATE TABLE IF NOT EXISTS tunnel_group (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(100) NOT NULL, created_time INTEGER NOT NULL, updated_time INTEGER NOT NULL, status INTEGER NOT NULL)");
        ensureTable("CREATE TABLE IF NOT EXISTS user_group (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(100) NOT NULL, created_time INTEGER NOT NULL, updated_time INTEGER NOT NULL, status INTEGER NOT NULL)");
        ensureTable("CREATE TABLE IF NOT EXISTS tunnel_group_tunnel (id INTEGER PRIMARY KEY AUTOINCREMENT, tunnel_group_id INTEGER NOT NULL, tunnel_id INTEGER NOT NULL, created_time INTEGER NOT NULL)");
        ensureTable("CREATE TABLE IF NOT EXISTS user_group_user (id INTEGER PRIMARY KEY AUTOINCREMENT, user_group_id INTEGER NOT NULL, user_id INTEGER NOT NULL, created_time INTEGER NOT NULL)");
        ensureTable("CREATE TABLE IF NOT EXISTS group_permission (id INTEGER PRIMARY KEY AUTOINCREMENT, user_group_id INTEGER NOT NULL, tunnel_group_id INTEGER NOT NULL, created_time INTEGER NOT NULL)");
        ensureTable("CREATE TABLE IF NOT EXISTS group_permission_grant (id INTEGER PRIMARY KEY AUTOINCREMENT, user_group_id INTEGER NOT NULL, tunnel_group_id INTEGER NOT NULL, user_tunnel_id INTEGER NOT NULL, created_by_group INTEGER NOT NULL DEFAULT 0, created_time INTEGER NOT NULL)");
        ensureColumn("group_permission_grant", "created_by_group", "INTEGER NOT NULL DEFAULT 0");
        ensureTable("CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnel_group_name ON tunnel_group(name)");
        ensureTable("CREATE UNIQUE INDEX IF NOT EXISTS idx_user_group_name ON user_group(name)");
        ensureTable("CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnel_group_tunnel_unique ON tunnel_group_tunnel(tunnel_group_id, tunnel_id)");
        ensureTable("CREATE UNIQUE INDEX IF NOT EXISTS idx_user_group_user_unique ON user_group_user(user_group_id, user_id)");
        ensureTable("CREATE UNIQUE INDEX IF NOT EXISTS idx_group_permission_unique ON group_permission(user_group_id, tunnel_group_id)");
        ensureTable("CREATE UNIQUE INDEX IF NOT EXISTS idx_group_permission_grant_unique ON group_permission_grant(user_group_id, tunnel_group_id, user_tunnel_id)");
    }

    private void ensureColumn(String table, String column, String columnDefinition) {
        Set<String> columns = new HashSet<>(
                jdbcTemplate.query(
                        "PRAGMA table_info(" + table + ")",
                        (rs, rowNum) -> rs.getString("name")
                )
        );

        if (columns.contains(column)) {
            return;
        }

        log.info("Adding missing column {}.{}", table, column);
        jdbcTemplate.execute(
                "ALTER TABLE " + table + " ADD COLUMN " + column + " " + columnDefinition
        );
    }

    private void ensureTable(String ddl) {
        jdbcTemplate.execute(ddl);
    }
}
