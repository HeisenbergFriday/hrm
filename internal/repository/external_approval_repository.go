package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ExternalSourceApprovalTable is a read-only Doris table. Keep this identifier
// constant so callers cannot inject a table name into the query.
const ExternalSourceApprovalTable = "dwd_dingtalk_attendance_approval_detail_di"

// ExternalApprovalSourceRepository reads the OA approval detail table from Doris.
// The table evolves independently of this service, so rows are returned as a
// field map instead of requiring a migration for every new source column.
type ExternalApprovalSourceRepository struct {
	db           *sql.DB
	queryTimeout time.Duration
}

func NewExternalApprovalSourceRepository(db *sql.DB, queryTimeout time.Duration) *ExternalApprovalSourceRepository {
	if queryTimeout <= 0 {
		queryTimeout = 30 * time.Second
	}
	return &ExternalApprovalSourceRepository{db: db, queryTimeout: queryTimeout}
}

// ListApprovalDetails returns all source columns for the requested corporation.
// Keyword matching is built only from columns that actually exist in the source
// table, allowing the OA table to add/remove optional fields safely.
func (r *ExternalApprovalSourceRepository) ListApprovalDetails(
	ctx context.Context,
	corpName string,
	keyword string,
	page int,
	pageSize int,
) (result []map[string]interface{}, total int64, returnErr error) {
	if r == nil || r.db == nil {
		return nil, 0, fmt.Errorf("external approval source db unavailable")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	table, columns, err := r.resolveTable(ctx)
	if err != nil {
		return nil, 0, err
	}
	where, args := approvalWhere(columns, corpName, keyword)

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", table, where)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count external approvals: %w", err)
	}

	orderBy := "process_instance_id"
	if !hasColumn(columns, orderBy) {
		orderBy = columns[0]
	}
	offset := (page - 1) * pageSize
	query := fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY %s DESC LIMIT %d OFFSET %d", table, where, quoteIdentifier(orderBy), pageSize, offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query external approvals: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && returnErr == nil {
			returnErr = fmt.Errorf("close external approval rows: %w", err)
		}
	}()

	result = make([]map[string]interface{}, 0, pageSize)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		dest := make([]interface{}, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, 0, fmt.Errorf("scan external approval: %w", err)
		}
		fields := make(map[string]interface{}, len(columns))
		for i, column := range columns {
			fields[strings.ToLower(strings.TrimSpace(column))] = normalizeApprovalValue(values[i])
		}
		result = append(result, fields)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func (r *ExternalApprovalSourceRepository) resolveTable(ctx context.Context) (string, []string, error) {
	for _, table := range []string{"dwd." + ExternalSourceApprovalTable, ExternalSourceApprovalTable} {
		rows, err := r.db.QueryContext(ctx, "SELECT * FROM "+table+" WHERE 1=0")
		if err != nil {
			continue
		}
		columns, err := rows.Columns()
		_ = rows.Close()
		if err == nil && len(columns) > 0 {
			return table, columns, nil
		}
	}
	return "", nil, fmt.Errorf("external approval table %s is unavailable", ExternalSourceApprovalTable)
}

func approvalWhere(columns []string, corpName, keyword string) (string, []interface{}) {
	conditions := []string{"corp_name = ?"}
	args := []interface{}{strings.TrimSpace(corpName)}
	if strings.TrimSpace(keyword) == "" {
		return strings.Join(conditions, " AND "), args
	}
	searchColumns := []string{
		"process_instance_id", "business_id", "process_name", "process_code",
		"approval_title", "originator_user_id", "originator_user_name",
	}
	like := "%" + strings.TrimSpace(keyword) + "%"
	parts := make([]string, 0, len(searchColumns))
	for _, column := range searchColumns {
		if hasColumn(columns, column) {
			parts = append(parts, "CAST("+quoteIdentifier(column)+" AS CHAR) LIKE ?")
			args = append(args, like)
		}
	}
	if len(parts) > 0 {
		conditions = append(conditions, "("+strings.Join(parts, " OR ")+")")
	}
	return strings.Join(conditions, " AND "), args
}

func hasColumn(columns []string, wanted string) bool {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	for _, column := range columns {
		if strings.ToLower(strings.TrimSpace(column)) == wanted {
			return true
		}
	}
	return false
}

func quoteIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

func normalizeApprovalValue(value interface{}) interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		return v
	}
}
