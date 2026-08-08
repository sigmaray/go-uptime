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

// sortColumn — одно поле модели, преобразованное в SQL, используемое ListSort.Apply.
type sortColumn struct {
	expr string
	join string
}

// ListSort содержит активное состояние сортировки для списка админки и формирует ссылки для шаблонов.
type ListSort struct {
	// Column — разрешённое имя Go-поля, например "IsUp". Пустое значение означает порядок по умолчанию.
	Column string
	// Order — "asc" или "desc". Игнорируется, когда Column пуст.
	Order string
	// Path — путь страницы списка, используемый при формировании URL сортировки.
	Path string
	// ExtraQuery — дополнительные query-параметры (фильтры), сохраняемые в ссылках сортировки и страниц.
	ExtraQuery url.Values

	defaultOrder string
	// stableOrder — вторичный ORDER BY (обычно «id asc»), чтобы при равных значениях
	// сортируемого поля страницы списка были детерминированными между запросами.
	stableOrder  string
	columns      map[string]sortColumn
}

var sortNamer = schema.NamingStrategy{}

// ParseListSort проверяет query-параметры сортировки по экспортированным Go-именам полей model.
// path — путь списка админки, например "/admin/monitors".
// model — нулевое значение GORM-модели для списка.
// defaultOrder — ORDER BY, используемый при отсутствии или некорректности параметров сортировки.
// rawSort — query-параметр sort (имя Go-поля, например "IsUp").
// rawOrder — "asc" или "desc".
// fields — whitelist экспортированных Go-имён полей, по которым разрешена сортировка.
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
		return sort // нет sort в query — порядок по умолчанию
	}
	if order != sortOrderAsc && order != sortOrderDesc {
		return sort // некорректный order — игнорируем сортировку
	}
	sort.Column = column
	sort.Order = order
	return sort
}

// Apply добавляет необходимые JOIN и ORDER BY для активной сортировки.
// db — GORM-запрос, уже ограниченный моделью списка.
func (s ListSort) Apply(db *gorm.DB) *gorm.DB {
	if s.IsDefault() {
		return db.Order(s.defaultOrder)
	}
	column, ok := s.columns[s.Column]
	if !ok {
		return db.Order(s.defaultOrder) // поле не в whitelist — fallback
	}
	if column.join != "" {
		db = db.Joins(column.join) // сортировка по связанной таблице (BelongsTo)
	}
	// NULLS LAST: NULL/неизвестные статусы (например IsUp без проверки) не «всплывают»
	// в начало списка. За stableOrder закрепляется стабильный порядок строк на странице.
	return db.Order(column.expr + " " + s.Order + " NULLS LAST, " + s.stableOrder)
}

// IsDefault сообщает, должен ли список использовать порядок по умолчанию.
func (s ListSort) IsDefault() bool {
	return s.Column == ""
}

// QueryValues возвращает параметры sort, order и ExtraQuery для ссылок пагинации и сортировки.
func (s ListSort) QueryValues() url.Values {
	q := url.Values{}
	if !s.IsDefault() {
		q.Set("sort", s.Column)
		q.Set("order", s.Order)
	}
	for key, values := range s.ExtraQuery {
		for _, value := range values {
			q.Add(key, value) // фильтры и прочие параметры списка
		}
	}
	if len(q) == 0 {
		return nil
	}
	return q
}

// PageURL формирует URL пагинированного списка с сохранением активной сортировки.
// page — номер страницы (с единицы), на которую ведёт ссылка.
func (s ListSort) PageURL(page int) string {
	return buildAdminListURLWithQuery(s.Path, page, s.QueryValues())
}

// AscURL формирует URL списка с сортировкой указанного поля по возрастанию с первой страницы.
// field — экспортированное Go-имя поля, например "IsUp".
func (s ListSort) AscURL(field string) string {
	next := s
	next.Column = field
	next.Order = sortOrderAsc
	return next.PageURL(1) // смена сортировки сбрасывает на первую страницу
}

// DescURL формирует URL списка с сортировкой указанного поля по убыванию с первой страницы.
// field — экспортированное Go-имя поля, например "IsUp".
func (s ListSort) DescURL(field string) string {
	next := s
	next.Column = field
	next.Order = sortOrderDesc
	return next.PageURL(1) // смена сортировки сбрасывает на первую страницу
}

// IsActiveAsc сообщает, активна ли сортировка по возрастанию для указанного поля.
// field — экспортированное Go-имя поля, сравниваемое с текущим состоянием сортировки.
func (s ListSort) IsActiveAsc(field string) bool {
	return s.Column == field && s.Order == sortOrderAsc
}

// IsActiveDesc сообщает, активна ли сортировка по убыванию для указанного поля.
// field — экспортированное Go-имя поля, сравниваемое с текущим состоянием сортировки.
func (s ListSort) IsActiveDesc(field string) bool {
	return s.Column == field && s.Order == sortOrderDesc
}

// matchSortField возвращает каноническое поле из whitelist, соответствующее rawSort, или "".
// rawSort — значение query sort; fields — список разрешённых Go-имён полей.
func matchSortField(rawSort string, fields []string) string {
	want := strings.TrimSpace(rawSort)
	if want == "" {
		return ""
	}
	for _, field := range fields {
		if strings.EqualFold(field, want) {
			return field // каноническое имя из whitelist (регистр как в коде)
		}
	}
	return ""
}

// sortableColumnsByFields преобразует экспортированные Go-имена полей model в SQL-выражения.
// model — нулевое значение GORM-модели для списка.
// fields — whitelist экспортированных Go-имён полей.
func sortableColumnsByFields(model any, fields []string) (map[string]sortColumn, string) {
	parsed, err := schema.Parse(model, &sync.Map{}, sortNamer)
	if err != nil {
		return map[string]sortColumn{}, "id asc" // безопасный минимум при ошибке схемы
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
	return columns, parsed.Table + ".id asc" // stable tie-breaker для пагинации
}

// columnFromField преобразует одно Go-поле распарсенной схемы в колонку сортировки.
// parsed — GORM-схема модели списка.
// fieldName — экспортированное Go-имя поля, например "CheckedAt".
func columnFromField(parsed *schema.Schema, fieldName string) (sortColumn, bool) {
	// BelongsTo: автоматически JOIN связанной таблицы по FK, чтобы сортировать
	// по «отображаемому» полю связанной модели, а не по сырому id.
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

	// Обычное поле модели — сортировка по колонке своей таблицы.
	field := parsed.LookUpField(fieldName)
	if field == nil || field.DBName == "" {
		return sortColumn{}, false
	}
	return sortColumn{expr: parsed.Table + "." + field.DBName}, true
}

// associationSortExpr выбирает выражение ORDER BY для связанной модели.
// related — GORM-схема цели belongs-to ассоциации.
// Логика как у MonitorDisplayName: сначала обрезанное имя, иначе URL, иначе id.
func associationSortExpr(related *schema.Schema) string {
	_, hasName := related.FieldsByName["Name"]
	_, hasURL := related.FieldsByName["URL"]
	if hasName && hasURL {
		// BTRIM убирает пробелы; пустое имя заменяется URL.
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
