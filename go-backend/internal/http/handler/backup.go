package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sort"
	"strings"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/store"
)

type backupPayload struct {
	Version    int                         `json:"version"`
	ExportedAt int64                       `json:"exportedAt"`
	Dialect    string                      `json:"dialect"`
	Tables     map[string][]map[string]any `json:"tables"`
}

func (h *Handler) backupExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if h == nil || h.repo == nil || h.repo.DB() == nil {
		response.WriteJSON(w, response.Err(-2, "database unavailable"))
		return
	}

	db := h.repo.DB()
	tableNames, err := listBackupTables(db)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	tables := make(map[string][]map[string]any, len(tableNames))
	for _, tableName := range tableNames {
		rows, err := dumpTableRows(db, tableName)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		tables[tableName] = rows
	}

	payload := backupPayload{
		Version:    1,
		ExportedAt: time.Now().UnixMilli(),
		Dialect:    db.Dialect().String(),
		Tables:     tables,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, "备份导出失败"))
		return
	}

	fileName := fmt.Sprintf("flvx-backup-%s.json", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) backupImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if h == nil || h.repo == nil || h.repo.DB() == nil {
		response.WriteJSON(w, response.Err(-2, "database unavailable"))
		return
	}

	raw, err := readBackupImportBody(r)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	var payload backupPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		response.WriteJSON(w, response.ErrDefault("备份文件格式错误"))
		return
	}
	if len(payload.Tables) == 0 {
		response.WriteJSON(w, response.ErrDefault("备份数据为空"))
		return
	}

	db := h.repo.DB()
	tableNames, err := listBackupTables(db)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	tableSet := make(map[string]struct{}, len(tableNames))
	for _, tableName := range tableNames {
		tableSet[tableName] = struct{}{}
	}
	for tableName := range payload.Tables {
		if _, ok := tableSet[tableName]; !ok {
			response.WriteJSON(w, response.ErrDefault("备份文件包含未知数据表"))
			return
		}
	}

	tx, err := db.Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer tx.Rollback()

	for i := len(tableNames) - 1; i >= 0; i-- {
		tableName := tableNames[i]
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", quoteIdentifier(tableName))); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}

	inserted := 0
	for _, tableName := range tableNames {
		rows := payload.Tables[tableName]
		if len(rows) == 0 {
			continue
		}

		columns, err := tableColumns(db, tableName)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}

		for _, row := range rows {
			insertCols := make([]string, 0, len(columns))
			insertVals := make([]any, 0, len(columns))
			for _, col := range columns {
				val, ok := row[col]
				if !ok {
					continue
				}
				insertCols = append(insertCols, quoteIdentifier(col))
				insertVals = append(insertVals, val)
			}
			if len(insertCols) == 0 {
				continue
			}

			placeholders := make([]string, len(insertCols))
			for i := range placeholders {
				placeholders[i] = "?"
			}

			query := fmt.Sprintf(
				"INSERT INTO %s (%s) VALUES (%s)",
				quoteIdentifier(tableName),
				strings.Join(insertCols, ","),
				strings.Join(placeholders, ","),
			)

			if _, err := tx.Exec(query, insertVals...); err != nil {
				response.WriteJSON(w, response.Err(-2, err.Error()))
				return
			}
			inserted++
		}
	}

	if err := tx.Commit(); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(map[string]any{
		"tables":   len(tableNames),
		"inserted": inserted,
	}))
}

func readBackupImportBody(r *http.Request) ([]byte, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return nil, fmt.Errorf("读取上传文件失败")
		}

		if r.MultipartForm != nil {
			preferredFields := []string{"file", "backup", "data"}
			for _, field := range preferredFields {
				if files := r.MultipartForm.File[field]; len(files) > 0 {
					return readMultipartFile(files[0])
				}
			}
			for _, files := range r.MultipartForm.File {
				if len(files) > 0 {
					return readMultipartFile(files[0])
				}
			}
		}

		if data := strings.TrimSpace(r.FormValue("data")); data != "" {
			return []byte(data), nil
		}
		return nil, fmt.Errorf("未找到备份文件")
	}

	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("读取请求数据失败")
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, fmt.Errorf("备份数据不能为空")
	}
	return body, nil
}

func readMultipartFile(header *multipart.FileHeader) ([]byte, error) {
	if header == nil {
		return nil, fmt.Errorf("未找到备份文件")
	}
	file, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("读取上传文件失败")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("读取上传文件失败")
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("备份数据不能为空")
	}
	return data, nil
}

func listBackupTables(db *store.DB) ([]string, error) {
	query := `
    SELECT name
    FROM sqlite_master
    WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
    ORDER BY name
  `
	if db.Dialect() == store.DialectPostgres {
		query = `
      SELECT table_name
      FROM information_schema.tables
      WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
      ORDER BY table_name
    `
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		if isSafeIdentifier(tableName) {
			out = append(out, tableName)
		}
	}
	return out, rows.Err()
}

func dumpTableRows(db *store.DB, tableName string) ([]map[string]any, error) {
	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s", quoteIdentifier(tableName)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, 0)
	for rows.Next() {
		vals := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(columns))
		for i, col := range columns {
			row[col] = normalizeExportedValue(vals[i])
		}
		items = append(items, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func tableColumns(db *store.DB, tableName string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT 0", quoteIdentifier(tableName)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	sort.Strings(columns)
	return columns, nil
}

func normalizeExportedValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	default:
		return t
	}
}

func quoteIdentifier(name string) string {
	if !isSafeIdentifier(name) {
		return "\"\""
	}
	return fmt.Sprintf("\"%s\"", name)
}

func isSafeIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			continue
		}
		return false
	}
	return true
}
