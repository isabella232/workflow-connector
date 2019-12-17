package sqlite

import (
	"database/sql"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/signavio/workflow-connector/internal/app/endpoint"
	sqlBackend "github.com/signavio/workflow-connector/internal/pkg/sql"
	"github.com/signavio/workflow-connector/internal/pkg/util"
)

var (
	QueryTemplates = map[string]string{
		`GetSingle`: `SELECT * ` +
			`FROM "{{.TableName}}" AS "_{{.TableName}}"` +
			`{{range .Relations}}` +
			`   LEFT JOIN "{{.Relationship.WithTable}}"` +
			`   ON "{{.Relationship.WithTable}}"."{{.Relationship.ForeignTableUniqueIdColumn}}"` +
			`   = "_{{$.TableName}}"."{{.Relationship.LocalTableUniqueIdColumn}}"` +
			`{{end}}` +
			` WHERE "_{{$.TableName}}"."{{.UniqueIdColumn}}" = ?`,
		`GetSingleAsOption`: `SELECT "{{.UniqueIdColumn}}", "{{.ColumnAsOptionName}}" ` +
			`FROM "{{.TableName}}" ` +
			`WHERE "{{.UniqueIdColumn}}" = ?`,
		`GetCollection`: `SELECT * ` +
			`FROM "{{.TableName}}" ` +
			`ORDER BY "{{.UniqueIdColumn}}" ASC`,
		`GetCollectionFilterable`: `SELECT * ` +
			`FROM "{{.TableName}}" AS "_{{.TableName}}"` +
			`{{range .Relations}}` +
			`   LEFT JOIN "{{.Relationship.WithTable}}"` +
			`   ON "{{.Relationship.WithTable}}"."{{.Relationship.ForeignTableUniqueIdColumn}}"` +
			`   = "_{{$.TableName}}"."{{.Relationship.LocalTableUniqueIdColumn}}"` +
			`{{end}}` +
			` WHERE "_{{$.TableName}}"."{{.FilterOnColumn}}" {{.Operator}} ?`,
		`GetCollectionAsOptions`: `SELECT "{{.UniqueIdColumn}}", "{{.ColumnAsOptionName}}" ` +
			`FROM "{{.TableName}}" ` +
			`ORDER BY "{{.UniqueIdColumn}}" ASC`,
		`GetCollectionAsOptionsFilterable`: `SELECT "{{.UniqueIdColumn}}", "{{.ColumnAsOptionName}}" ` +
			`FROM "{{.TableName}}" ` +
			`WHERE "{{.ColumnAsOptionName}}" LIKE ?`,
		`GetCollectionAsOptionsWithParams`: `SELECT "{{.UniqueIdColumn}}", "{{.ColumnAsOptionName}}" ` +
			`FROM "{{.TableName}}" ` +
			`WHERE "{{.ColumnAsOptionName}}" LIKE ? ` +
			"{{range $index, $element := .ColumnNames}}" +
			`AND "{{$element}}" = ?` +
			"{{end}}",
		`UpdateSingle`: `UPDATE "{{.TableName}}" SET "{{.ColumnNames | head}}"` +
			` = ?{{range .ColumnNames | tail}},`+
			` "{{.}}" = ?{{end}}`+
			` WHERE "{{.UniqueIdColumn}}" = ?`,
		`CreateSingle`: `INSERT INTO "{{.TableName}}"`+
			`("{{.ColumnNames | head}}"` +
			`{{range .ColumnNames | tail}},`+
			` "{{.}}"`+
			`{{end}}) ` +
			`VALUES(?{{range .ColumnNames | tail}}, ?{{end}})`,
		`DeleteSingle`: `DELETE FROM "{{.TableName}}" WHERE "{{.UniqueIdColumn}}" = ?`, `GetTableSchema`: `SELECT * ` +
			`FROM "{{.TableName}}" ` +
			`LIMIT 1`,
		`GetTableWithRelationshipsSchema`: `SELECT * FROM "{{.TableName}}" AS "_{{.TableName}}" ` +
			`{{range .Relations}}` +
			` LEFT JOIN "{{.Relationship.WithTable}}"` +
			` ON "{{.Relationship.WithTable}}"."{{.Relationship.ForeignTableUniqueIdColumn}}"` +
			` = "_{{$.TableName}}"."{{.Relationship.LocalTableUniqueIdColumn}}"{{end}} LIMIT 1`,
	}
	integer = []string{
		"BIGINT",
		"INT",
		"INT2",
		"INT8",
		"INTEGER",
		"MEDIUMINT",
		"SMALLINT",
		"TINYINT",
		"UNSIGNED",
	}
	text = []string{
		"CHARACTER",
		"CLOB",
		"NATIVE CHARACTER",
		"NCHAR",
		"NVARCHAR",
		"TEXT",
		"VARCHAR",
		"VARYING CHARACTER",
	}
	real = []string{
		"DOUBLE PRECISION",
		"DOUBLE",
		"FLOAT",
		"REAL",
	}
	numeric = []string{
		"BOOLEAN",
		"DECIMAL",
		"NUMERIC",
	}
	dateTime = []string{
		"DATE",
		"DATETIME",
	}
)

type Sqlite struct {
	*sqlBackend.SqlBackend
}

func New() endpoint.Endpoint {
	s := &Sqlite{sqlBackend.New().(*sqlBackend.SqlBackend)}
	s.Templates = QueryTemplates
	s.CastBackendTypeToGolangType = convertFromSqliteDataType
	return s
}

func convertFromSqliteDataType(fieldDataType string) interface{} {
	switch {
	case isOfDataType(integer, fieldDataType):
		return &sql.NullInt64{}
	case isOfDataType(text, fieldDataType):
		return &sql.NullString{}
	case isOfDataType(real, fieldDataType):
		return &sql.NullFloat64{}
	case isOfDataType(numeric, fieldDataType):
		return &sql.NullFloat64{}
	case isOfDataType(dateTime, fieldDataType):
		return &util.NullTime{}
	default:
		return &sql.NullString{}
	}
}

func isOfDataType(ts []string, fieldDataType string) (result bool) {
	result = false
	for _, t := range ts {
		if strings.HasPrefix(strings.ToUpper(fieldDataType), t) {
			return true
		}
	}
	return
}
