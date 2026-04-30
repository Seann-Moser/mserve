// file: repo/postgres.go
package repo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"reflect"
	"strings"

	"github.com/DarlingGoose/mserve"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGRepo defines the Postgres-flavoured generic repository interface.
// It mirrors Repo[T] but replaces Mongo-specific types with SQL equivalents.
type PGRepo[T any] interface {
	InsertAny(ctx context.Context, data ...any) ([]T, error)
	Insert(ctx context.Context, data ...T) ([]T, error)
	// Update applies SET fields to every row matching WHERE fields.
	Update(ctx context.Context, set map[string]interface{}, where map[string]interface{}) (T, error)
	// List returns a paginated result using LIMIT/OFFSET.
	List(ctx context.Context, where map[string]interface{}, orderBy string, page mserve.Page[T]) (mserve.Page[T], error)
	Delete(ctx context.Context, where map[string]interface{}) error
	// DB returns the underlying pool for callers that need raw access.
	DB() *pgxpool.Pool
}

// ─────────────────────────────────────────────────────────────────────────────
// Postgres[T] — generic concrete implementation
// ─────────────────────────────────────────────────────────────────────────────

// Postgres is a generic repository backed by a pgxpool.Pool.
// T must be a struct whose fields carry `db:"column_name"` tags.
// Index creation reuses the same `index` and `group` struct tags as the
// Mongo version, mapping them to CREATE INDEX statements.
type Postgres[T any] struct {
	pool      *pgxpool.Pool
	tableName string
	columns   []columnMeta // ordered list of db-tagged fields
}

// columnMeta holds reflection metadata for one struct field.
type columnMeta struct {
	fieldIndex int    // position in the struct
	column     string // db tag value
	index      string // "" | "1" | "-1" | "text"
	group      string // compound index group name
}

// NewPostgres creates a Postgres[T] repository.
// It reflects on T to derive the table name (struct name, lower-cased) and
// column list, then ensures all declared indexes exist.
func NewPostgres[T any](ctx context.Context, pool *pgxpool.Pool) (PGRepo[T], error) {
	var zero T
	tableName, cols, err := reflectType[T](zero)
	if err != nil {
		return nil, err
	}

	r := &Postgres[T]{
		pool:      pool,
		tableName: tableName,
		columns:   cols,
	}

	if err := r.createIndexes(ctx); err != nil {
		return nil, fmt.Errorf("postgres repo %s: create indexes: %w", tableName, err)
	}

	return r, nil
}

// DB returns the underlying connection pool.
func (r *Postgres[T]) DB() *pgxpool.Pool { return r.pool }

// ─────────────────────────────────────────────────────────────────────────────
// InsertAny
// ─────────────────────────────────────────────────────────────────────────────

// InsertAny inserts arbitrary values that must be convertible to T.
func (r *Postgres[T]) InsertAny(ctx context.Context, data ...any) ([]T, error) {
	if len(data) == 0 {
		return nil, nil
	}
	typed := make([]T, 0, len(data))
	for _, d := range data {
		v, ok := d.(T)
		if !ok {
			return nil, fmt.Errorf("InsertAny: value of type %T is not %T", d, *new(T))
		}
		typed = append(typed, v)
	}
	return r.Insert(ctx, typed...)
}

// ─────────────────────────────────────────────────────────────────────────────
// Insert
// ─────────────────────────────────────────────────────────────────────────────

// Insert bulk-inserts rows and returns them (re-read via RETURNING *).
func (r *Postgres[T]) Insert(ctx context.Context, data ...T) ([]T, error) {
	if len(data) == 0 {
		return nil, nil
	}

	colNames := make([]string, len(r.columns))
	for i, c := range r.columns {
		colNames[i] = c.column
	}

	// Build: INSERT INTO table (c1,c2,...) VALUES ($1,$2,...),($n+1,...) RETURNING *
	placeholders := make([]string, len(data))
	args := make([]interface{}, 0, len(data)*len(r.columns))
	argIdx := 1
	for i, row := range data {
		rv := reflect.ValueOf(row)
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		ph := make([]string, len(r.columns))
		for j, col := range r.columns {
			ph[j] = fmt.Sprintf("$%d", argIdx)
			args = append(args, rv.Field(col.fieldIndex).Interface())
			argIdx++
		}
		placeholders[i] = "(" + strings.Join(ph, ", ") + ")"
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s RETURNING *",
		r.tableName,
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "),
	)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insert: %w", err)
	}
	defer rows.Close()

	return scanRows[T](rows, r.columns)
}

// ─────────────────────────────────────────────────────────────────────────────
// Update
// ─────────────────────────────────────────────────────────────────────────────

// Update applies SET to every row matching WHERE and returns the first updated row.
func (r *Postgres[T]) Update(ctx context.Context, set map[string]interface{}, where map[string]interface{}) (T, error) {
	var zero T
	if len(set) == 0 {
		return zero, errors.New("update: SET map must not be empty")
	}
	if len(where) == 0 {
		return zero, errors.New("update: WHERE map must not be empty (full-table updates not allowed)")
	}

	args := make([]interface{}, 0, len(set)+len(where))
	argIdx := 1

	setClauses := make([]string, 0, len(set))
	for col, val := range set {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	whereClauses := make([]string, 0, len(where))
	for col, val := range where {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s RETURNING *",
		r.tableName,
		strings.Join(setClauses, ", "),
		strings.Join(whereClauses, " AND "),
	)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return zero, fmt.Errorf("update: %w", err)
	}
	defer rows.Close()

	results, err := scanRows[T](rows, r.columns)
	if err != nil {
		return zero, err
	}
	if len(results) == 0 {
		return zero, errors.New("update: no document found to update")
	}
	return results[0], nil
}

// ─────────────────────────────────────────────────────────────────────────────
// List  (paginated)
// ─────────────────────────────────────────────────────────────────────────────

// List returns a paginated slice of T rows matching where.
// orderBy is an optional raw ORDER BY expression, e.g. "created_at DESC".
func (r *Postgres[T]) List(ctx context.Context, where map[string]interface{}, orderBy string, page mserve.Page[T]) (mserve.Page[T], error) {
	pageNum := page.Page
	limit := page.Limit
	if pageNum < 1 {
		pageNum = 1
	}
	if limit < 1 {
		limit = 20
	}

	args := make([]interface{}, 0, len(where)+2)
	argIdx := 1

	whereClauses := make([]string, 0, len(where))
	for col, val := range where {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count total matching rows.
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", r.tableName, whereSQL)
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return mserve.Page[T]{}, fmt.Errorf("list count: %w", err)
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	offset := (pageNum - 1) * limit

	// Append LIMIT / OFFSET args.
	orderSQL := ""
	if orderBy != "" {
		orderSQL = "ORDER BY " + orderBy
	}

	dataArgs := append(args, limit, offset)
	dataQuery := fmt.Sprintf(
		"SELECT * FROM %s %s %s LIMIT $%d OFFSET $%d",
		r.tableName, whereSQL, orderSQL, argIdx, argIdx+1,
	)

	rows, err := r.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return mserve.Page[T]{}, fmt.Errorf("list query: %w", err)
	}
	defer rows.Close()

	items, err := scanRows[T](rows, r.columns)
	if err != nil {
		return mserve.Page[T]{}, err
	}

	return mserve.Page[T]{
		Items:      items,
		Page:       pageNum,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Delete
// ─────────────────────────────────────────────────────────────────────────────

// Delete removes all rows matching where.
func (r *Postgres[T]) Delete(ctx context.Context, where map[string]interface{}) error {
	if len(where) == 0 {
		return errors.New("delete: WHERE map must not be empty (full-table deletes not allowed)")
	}

	args := make([]interface{}, 0, len(where))
	argIdx := 1
	whereClauses := make([]string, 0, len(where))
	for col, val := range where {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	query := fmt.Sprintf(
		"DELETE FROM %s WHERE %s",
		r.tableName,
		strings.Join(whereClauses, " AND "),
	)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	slog.Info("deleted rows", "table", r.tableName, "count", tag.RowsAffected())
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Index creation
// ─────────────────────────────────────────────────────────────────────────────

// createIndexes reflects on T and issues CREATE INDEX IF NOT EXISTS for every
// field tagged with `index`, using `group` to combine compound indexes.
func (r *Postgres[T]) createIndexes(ctx context.Context) error {
	// Single-column indexes.
	for _, col := range r.columns {
		if col.index == "" || col.group != "" {
			continue
		}
		idxName := fmt.Sprintf("idx_%s_%s", r.tableName, col.column)
		var ddl string
		if col.index == "text" {
			// Full-text: use a GIN index on a tsvector expression.
			ddl = fmt.Sprintf(
				"CREATE INDEX IF NOT EXISTS %s ON %s USING GIN (to_tsvector('english', %s))",
				idxName, r.tableName, col.column,
			)
		} else {
			ddl = fmt.Sprintf(
				"CREATE INDEX IF NOT EXISTS %s ON %s (%s)",
				idxName, r.tableName, col.column,
			)
		}
		if _, err := r.pool.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("create index %s: %w", idxName, err)
		}
	}

	// Compound indexes — group columns by their `group` tag.
	groups := make(map[string][]string)
	for _, col := range r.columns {
		if col.group == "" {
			continue
		}
		groups[col.group] = append(groups[col.group], col.column)
	}
	for grp, cols := range groups {
		idxName := fmt.Sprintf("idx_%s_%s", r.tableName, grp)
		ddl := fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s (%s)",
			idxName, r.tableName, strings.Join(cols, ", "),
		)
		if _, err := r.pool.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("create compound index %s: %w", idxName, err)
		}
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Reflection helpers
// ─────────────────────────────────────────────────────────────────────────────

// reflectType derives the table name and column metadata from a value of type T.
func reflectType[T any](zero T) (tableName string, cols []columnMeta, err error) {
	t := reflect.TypeOf(zero)
	if t == nil {
		return "", nil, errors.New("reflectType: T must not be an interface")
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return "", nil, errors.New("reflectType: T must be a struct")
	}

	tableName = strings.ToLower(t.Name())

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		dbTag := f.Tag.Get("db")
		if dbTag == "" || dbTag == "-" {
			continue
		}
		// Strip options like `db:"name,omitempty"`.
		col := strings.Split(dbTag, ",")[0]

		cols = append(cols, columnMeta{
			fieldIndex: i,
			column:     col,
			index:      f.Tag.Get("index"),
			group:      f.Tag.Get("group"),
		})
	}

	if len(cols) == 0 {
		return "", nil, fmt.Errorf("reflectType: struct %s has no `db`-tagged fields", t.Name())
	}
	return tableName, cols, nil
}

// scanRows decodes pgx rows into []T using the column metadata for field mapping.
func scanRows[T any](rows pgx.Rows, cols []columnMeta) ([]T, error) {
	// Build a map from column name → field index for fast lookup.
	colIndex := make(map[string]int, len(cols))
	for _, c := range cols {
		colIndex[c.column] = c.fieldIndex
	}

	var out []T
	for rows.Next() {
		descriptions := rows.FieldDescriptions()
		ptrs := make([]interface{}, len(descriptions))

		var t T
		rv := reflect.ValueOf(&t).Elem()

		for i, fd := range descriptions {
			colName := string(fd.Name)
			if fi, ok := colIndex[colName]; ok {
				ptrs[i] = rv.Field(fi).Addr().Interface()
			} else {
				// Column returned by DB but not in struct — discard.
				var discard interface{}
				ptrs[i] = &discard
			}
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
