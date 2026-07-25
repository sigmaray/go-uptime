package handlers

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	sortOrderAsc  = "asc"
	sortOrderDesc = "desc"
)

// sortColumn is one model field resolved into SQL used by ListSort.Apply.
type sortColumn struct {
	expr string
	join string
}

// ListSort holds the active sort state for an admin list and builds template links.
type ListSort struct {
	// Column is an allowed Go field name such as "IsUp". Empty means default order.
	Column string
	// Order is "asc" or "desc". Ignored when Column is empty.
	Order string
	// Path is the list page path used when building sort URLs.
	Path string
	// ExtraQuery holds additional query parameters (filters) preserved in sort and page links.
	ExtraQuery url.Values

	defaultOrder string
	stableOrder  string
	columns      map[string]sortColumn
}

var sortNamer = schema.NamingStrategy{}

// ParseListSort validates sort query params against exported Go field names on model.
// path is the admin list path such as "/admin/monitors".
// model is a zero value of the GORM model being listed.
// defaultOrder is the ORDER BY used when sort params are missing or invalid.
// rawSort is the sort query parameter (a Go field name such as "IsUp").
// rawOrder is "asc" or "desc".
// fields is the whitelist of exported Go field names that may be sorted.
func ParseListSort(path string, model any, defaultOrder, rawSort, rawOrder string, fields ...string) ListSort {
	columns, stableOrder := sortableColumnsByFields(model, fields)
	sort := ListSort{
		Path:         path,
		defaultOrder: defaultOrder,
		stableOrder:  stableOrder,
		columns:      columns,
	}

	column := matchSortField(rawSort, fields)
	order := strings.ToLower(strings.TrimSpace(rawOrder))
	if column == "" {
		return sort
	}
	if order != sortOrderAsc && order != sortOrderDesc {
		return sort
	}
	sort.Column = column
	sort.Order = order
	return sort
}

// Apply adds any required JOIN and ORDER BY for the active sort.
// db is the GORM query already scoped to the list model.
func (s ListSort) Apply(db *gorm.DB) *gorm.DB {
	if s.IsDefault() {
		return db.Order(s.defaultOrder)
	}
	column, ok := s.columns[s.Column]
	if !ok {
		return db.Order(s.defaultOrder)
	}
	if column.join != "" {
		db = db.Joins(column.join)
	}
	return db.Order(column.expr + " " + s.Order + " NULLS LAST, " + s.stableOrder)
}

// IsDefault reports whether the list should use the default order.
func (s ListSort) IsDefault() bool {
	return s.Column == ""
}

// QueryValues returns sort, order, and ExtraQuery parameters for pagination and sort links.
func (s ListSort) QueryValues() url.Values {
	q := url.Values{}
	if !s.IsDefault() {
		q.Set("sort", s.Column)
		q.Set("order", s.Order)
	}
	for key, values := range s.ExtraQuery {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	if len(q) == 0 {
		return nil
	}
	return q
}

// PageURL builds a paginated list URL that preserves the active sort.
// page is the one-based page number to link to.
func (s ListSort) PageURL(page int) string {
	return buildAdminListURLWithQuery(s.Path, page, s.QueryValues())
}

// AscURL builds the list URL that sorts the given field ascending from page 1.
// field is an exported Go field name such as "IsUp".
func (s ListSort) AscURL(field string) string {
	next := s
	next.Column = field
	next.Order = sortOrderAsc
	return next.PageURL(1)
}

// DescURL builds the list URL that sorts the given field descending from page 1.
// field is an exported Go field name such as "IsUp".
func (s ListSort) DescURL(field string) string {
	next := s
	next.Column = field
	next.Order = sortOrderDesc
	return next.PageURL(1)
}

// IsActiveAsc reports whether ascending sort is active for the given field.
// field is an exported Go field name compared to the current sort state.
func (s ListSort) IsActiveAsc(field string) bool {
	return s.Column == field && s.Order == sortOrderAsc
}

// IsActiveDesc reports whether descending sort is active for the given field.
// field is an exported Go field name compared to the current sort state.
func (s ListSort) IsActiveDesc(field string) bool {
	return s.Column == field && s.Order == sortOrderDesc
}

// matchSortField returns the canonical whitelist field matching rawSort, or "".
// rawSort is the sort query value; fields is the allowed Go field name list.
func matchSortField(rawSort string, fields []string) string {
	want := strings.TrimSpace(rawSort)
	if want == "" {
		return ""
	}
	for _, field := range fields {
		if strings.EqualFold(field, want) {
			return field
		}
	}
	return ""
}

// sortableColumnsByFields resolves exported Go field names on model into SQL expressions.
// model is a zero value of the GORM model being listed.
// fields is the whitelist of exported Go field names.
func sortableColumnsByFields(model any, fields []string) (map[string]sortColumn, string) {
	parsed, err := schema.Parse(model, &sync.Map{}, sortNamer)
	if err != nil {
		return map[string]sortColumn{}, "id asc"
	}

	columns := make(map[string]sortColumn, len(fields))
	for _, fieldName := range fields {
		if fieldName == "" {
			continue
		}
		col, ok := columnFromField(parsed, fieldName)
		if ok {
			columns[fieldName] = col
		}
	}
	return columns, parsed.Table + ".id asc"
}

// columnFromField resolves one Go field on the parsed schema into a sort column.
// parsed is the GORM schema for the list model.
// fieldName is the exported Go field name such as "CheckedAt".
func columnFromField(parsed *schema.Schema, fieldName string) (sortColumn, bool) {
	if rel, ok := parsed.Relationships.Relations[fieldName]; ok && rel.Type == schema.BelongsTo {
		related := rel.FieldSchema
		if related == nil || len(rel.References) == 0 || rel.References[0].ForeignKey == nil {
			return sortColumn{}, false
		}
		fk := rel.References[0].ForeignKey.DBName
		return sortColumn{
			join: fmt.Sprintf(
				"JOIN %s ON %s.id = %s.%s",
				related.Table, related.Table, parsed.Table, fk,
			),
			expr: associationSortExpr(related),
		}, true
	}

	field := parsed.LookUpField(fieldName)
	if field == nil || field.DBName == "" {
		return sortColumn{}, false
	}
	return sortColumn{expr: parsed.Table + "." + field.DBName}, true
}

// associationSortExpr picks an ORDER BY expression for a related model.
// related is the GORM schema of the belongs-to association target.
func associationSortExpr(related *schema.Schema) string {
	_, hasName := related.FieldsByName["Name"]
	_, hasURL := related.FieldsByName["URL"]
	if hasName && hasURL {
		return fmt.Sprintf(
			"COALESCE(NULLIF(BTRIM(%s.name), ''), %s.url)",
			related.Table, related.Table,
		)
	}
	if hasName {
		return related.Table + ".name"
	}
	return related.Table + ".id"
}
