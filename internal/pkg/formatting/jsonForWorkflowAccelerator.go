package formatting

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/signavio/workflow-connector/internal/pkg/config"
	"github.com/signavio/workflow-connector/internal/pkg/descriptor"
	"github.com/signavio/workflow-connector/internal/pkg/log"
	"github.com/signavio/workflow-connector/internal/pkg/util"
)

type standardFormatter struct{}
type getCollectionFormatter struct{}

// Special formatting for the options route like `/options`, `/options?filter=`
// is required, since Workflow Accelerator expects the results returned
// by these routes to be enclosed in an array, regardless of whether
// or not the result set return 0, 1 or many results
type getSingleAsOptionFormatter struct{}
type getCollectionAsOptionsFormatter struct{}

var (
	Standard                         = &standardFormatter{}
	GetCollection                    = &standardFormatter{}
	GetSingleAsOption                = &getSingleAsOptionFormatter{}
	GetCollectionAsOptions           = &getCollectionAsOptionsFormatter{}
	GetCollectionAsOptionsFilterable = &getCollectionAsOptionsFormatter{}
)

// Format will format the results received from the backend service,
// accorind to the Workflow Accelerator Connector API
func (f *standardFormatter) Format(ctx context.Context, results []interface{}) (JSONResults []byte, err error) {
	if len(results) == 0 {
		return []byte("{}"), nil
	}
	tableName := ctx.Value(util.ContextKey("table")).(string)
	typeDescriptor := util.GetTypeDescriptorUsingDBTableName(
		config.Options.Descriptor.TypeDescriptors,
		tableName,
	)
	fields := typeDescriptor.Fields
	if len(results) == 1 {
		log.When(config.Options.Logging).Infoln("[formatter -> asWorkflowType] Format with result set == 1")
		formattedResult := formatAsAWorkflowType(
			ctx, results[0].(map[string]interface{}), tableName, fields,
		)
		log.When(config.Options.Logging).Infof("[formatter <- asWorkflowType] formattedResult: \n%+v\n", formattedResult)
		JSONResults, err = json.MarshalIndent(&formattedResult, "", "  ")
		if err != nil {
			return nil, err
		}
		log.When(config.Options.Logging).Infoln("[routeHandler <- formatter]")
		return
	}
	log.When(config.Options.Logging).Infoln("[formatter -> asWorkflowType] Format with result set > 1")
	var formattedResults []interface{}
	for _, result := range results {
		formattedResult := formatAsAWorkflowType(
			ctx, result.(map[string]interface{}), tableName, fields,
		)
		formattedResults = append(formattedResults, formattedResult)
	}
	log.When(config.Options.Logging).Infof(
		"[formatter <- asWorkflowType] formattedResult (top 2): \n%+v ...\n",
		formattedResults[0:1],
	)
	JSONResults, err = json.MarshalIndent(&formattedResults, "", "  ")
	if err != nil {
		return nil, err
	}
	log.When(config.Options.Logging).Infoln("[routeHandler <- formatter]")
	return
}
func (f *getCollectionFormatter) Format(ctx context.Context, results []interface{}) (JSONResults []byte, err error) {
	if len(results) == 0 {
		return []byte("[]"), nil
	}
	tableName := ctx.Value(util.ContextKey("table")).(string)
	fields := withRelationshipFieldsOmitted(tableName)
	var formattedResults []interface{}
	if len(results) == 1 {
		log.When(config.Options.Logging).Infoln("[formatter -> asWorkflowType] Format with result set == 1")
		formattedResult := formatAsAWorkflowType(
			ctx, results[0].(map[string]interface{}), tableName, fields,
		)
		formattedResults = append(formattedResults, formattedResult)
		log.When(config.Options.Logging).Infof("[formatter <- asWorkflowType] formattedResult: \n%+v\n", formattedResult)
		JSONResults, err = json.MarshalIndent(&formattedResults, "", "  ")
		if err != nil {
			return nil, err
		}
		log.When(config.Options.Logging).Infoln("[routeHandler <- formatter]")
		return
	}
	log.When(config.Options.Logging).Infoln("[formatter -> asWorkflowType] Format with result set > 1")
	for _, result := range results {
		formattedResult := formatAsAWorkflowType(
			ctx, result.(map[string]interface{}), tableName, fields,
		)
		formattedResults = append(formattedResults, formattedResult)
	}
	log.When(config.Options.Logging).Infof(
		"[formatter <- asWorkflowType] formattedResult (top 2): \n%+v ...\n",
		formattedResults[0:1],
	)
	JSONResults, err = json.MarshalIndent(&formattedResults, "", "  ")
	if err != nil {
		return nil, err
	}
	log.When(config.Options.Logging).Infoln("[routeHandler <- formatter]")
	return
}
func (f *getSingleAsOptionFormatter) Format(ctx context.Context, results []interface{}) (JSONResults []byte, err error) {
	tableName := ctx.Value(util.ContextKey("table")).(string)
	if len(results) == 0 {
		return []byte("{}"), nil
	}
	if len(results) > 1 {
		return nil, fmt.Errorf("formatting: expected result set to contain only one resource")
	}
	formattedResult := stringify(results[0].(map[string]interface{})[tableName])
	log.When(config.Options.Logging).Infof("[formatter <- asWorkflowType] formattedResult: \n%+v\n", formattedResult)
	JSONResults, err = json.MarshalIndent(&formattedResult, "", "  ")
	if err != nil {
		return nil, err
	}
	log.When(config.Options.Logging).Infoln("[routeHandler <- formatter]")
	return
}
func (f *getCollectionAsOptionsFormatter) Format(ctx context.Context, results []interface{}) (JSONResults []byte, err error) {
	tableName := ctx.Value(util.ContextKey("table")).(string)
	var formattedResults []interface{}
	if len(results) == 0 {
		return []byte("[]"), nil
	}
	for _, result := range results {
		formattedResults = append(
			formattedResults,
			stringify(result.(map[string]interface{})[tableName]),
		)
	}
	formattedResultsSubset := subsetForPerformance(formattedResults)
	log.When(config.Options.Logging).Infof(
		"[formatter <- asWorkflowType] formattedResult(s): \n%+v ...\n",
		formattedResults,
	)
	JSONResults, err = json.MarshalIndent(&formattedResultsSubset, "", "  ")
	if err != nil {
		return nil, err
	}
	log.When(config.Options.Logging).Infoln("[routeHandler <- formatter]")
	return
}

func formatAsAWorkflowType(ctx context.Context, queryResults map[string]interface{}, table string, fields []*descriptor.Field) (formatted map[string]interface{}) {
	formatted = make(map[string]interface{})
	for _, field := range fields {
		if ctx.Value(util.ContextKey("currentRoute")).(string) == "GetCollection" {
			formatted = buildResultFromQueryResultsWithoutRelationships(
				ctx, formatted, queryResults, table, field,
			)
		} else {
			formatted = buildResultFromQueryResultsUsingField(
				ctx, formatted, queryResults, table, field,
			)
		}
	}
	return
}

func buildResultFromQueryResultsUsingField(ctx context.Context, formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	if tableHasRelationships(queryResults, table, field) {
		formatted = buildAndRecursivelyResolveRelationships(ctx, formatted, queryResults, table, field)
		return formatted
	}
	return buildResultFromQueryResultsWithoutRelationships(ctx, formatted, queryResults, table, field)
}

func buildResultFromQueryResultsWithoutRelationships(ctx context.Context, formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	typeDescriptor := util.GetTypeDescriptorUsingDBTableName(
		config.Options.Descriptor.TypeDescriptors,
		table,
	)
	switch {
	case field.FromColumn == typeDescriptor.ColumnAsOptionName:
		// Workflow Accelerator expects `columnAsOptionName`
		// to be called `name` and be of type string
		formatted["name"] = stringify(
			queryResults[table].(map[string]interface{})[field.FromColumn],
		)
	case field.FromColumn == typeDescriptor.UniqueIdColumn:
		// Workflow Accelerator expects `uniqueIdColumn`
		// to be called `id` and be of type string
		formatted["id"] = stringify(
			queryResults[table].(map[string]interface{})[field.FromColumn],
		)
	case field.Type.Name == "money":
		formatted = buildForFieldTypeMoney(formatted, queryResults, table, field)
	case field.Type.Kind == "datetime":
		formatted = buildForFieldTypeDateTime(formatted, queryResults, table, field)
	case field.Type.Kind == "date" ||
		field.Type.Kind == "time":
		formatted = buildForFieldTypeDateOrTime(formatted, queryResults, table, field)
	case field.Type.Name == "text":
		formatted = buildForFieldTypeText(formatted, queryResults, table, field)
	default:
		formatted = buildForFieldTypeOther(formatted, queryResults, table, field)
	}
	return formatted
}
func buildAndRecursivelyResolveRelationships(ctx context.Context, formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	switch field.Relationship.Kind {
	case "oneToMany":
		return relationshipKindIsOneToMany(ctx, formatted, queryResults, table, field)
	case "manyToOne", "oneToOne":
		return relationshipKindIsXToOne(ctx, formatted, queryResults, table, field)
	default:
		return make(map[string]interface{})
	}
}

func relationshipKindIsOneToMany(ctx context.Context, formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	typeDescriptor := util.GetTypeDescriptorUsingDBTableName(
		config.Options.Descriptor.TypeDescriptors,
		field.Relationship.WithTable,
	)
	if relatedTablesResultSetNotEmpty(queryResults, table, field) {
		relatedResults := queryResults[table].(map[string]interface{})[field.Key].(map[string]interface{})[field.Relationship.WithTable].([]map[string]interface{})
		if clientWantsDenormalizedResultSet(
			ctx.Value(util.ContextKey("denormalize")).(string),
		) {
			var results []map[string]interface{}
			results = denormalizeResultSet(ctx, relatedResults, field, typeDescriptor.UniqueIdColumn)
			formatted[field.Key] = results
			return formatted
		}
		var results []interface{}
		results = normalizeResultSet(ctx, relatedResults, field, typeDescriptor.UniqueIdColumn)
		formatted[field.Key] = results
		return formatted
	}
	formatted[field.Key] = []interface{}{}
	return formatted
}
func relationshipKindIsXToOne(ctx context.Context, formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	typeDescriptor := util.GetTypeDescriptorUsingDBTableName(
		config.Options.Descriptor.TypeDescriptors,
		field.Relationship.WithTable,
	)
	if relatedTablesResultSetNotEmpty(queryResults, table, field) {
		relatedResults := queryResults[table].(map[string]interface{})[field.Key].(map[string]interface{})[field.Relationship.WithTable].([]map[string]interface{})
		if clientWantsDenormalizedResultSet(
			ctx.Value(util.ContextKey("denormalize")).(string),
		) {
			var results map[string]interface{}
			results = denormalizeResultSet(ctx, relatedResults, field, typeDescriptor.UniqueIdColumn)[0]
			formatted[field.Key] = results
			return formatted
		}
		var results interface{}
		results = normalizeResultSet(ctx, relatedResults, field, typeDescriptor.UniqueIdColumn)[0]
		formatted[field.Key] = results
		log.When(config.Options.Logging).Infof("[asWorkflowType] formatted: \n%+v\n", formatted)
		return formatted
	}
	formatted[field.Key] = nil
	return formatted
}

func denormalizeResultSet(ctx context.Context, relatedResults []map[string]interface{}, field *descriptor.Field, uniqueIdColumn string) (results []map[string]interface{}) {
	for _, r := range relatedResults {
		// remove relationships keys from recursively resolved subset
		fields := withRelationshipFieldsOmitted(field.Relationship.WithTable)
		resolvedRelationship := formatAsAWorkflowType(
			ctx,
			map[string]interface{}{field.Relationship.WithTable: r},
			field.Relationship.WithTable,
			fields,
		)
		results = append(results, resolvedRelationship)
	}
	return results
}
func normalizeResultSet(ctx context.Context, relatedResults []map[string]interface{}, field *descriptor.Field, uniqueIdColumn string) (results []interface{}) {
	for _, r := range relatedResults {
		// remove relationships keys from recursively resolved subset
		fields := withRelationshipFieldsOmitted(field.Relationship.WithTable)
		resolvedRelationship := formatAsAWorkflowType(
			ctx,
			map[string]interface{}{field.Relationship.WithTable: r},
			field.Relationship.WithTable,
			fields,
		)
		log.When(config.Options.Logging).Infof("[asWorkflowType] resolvedRelationship: \n%+v\n", resolvedRelationship)
		results = append(results, resolvedRelationship[uniqueIdColumn])
	}

	log.When(config.Options.Logging).Infof("[asWorkflowType] results: \n%+v\n", results)
	return
}

func buildForFieldTypeMoney(formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	if queryResults[table].(map[string]interface{})[field.Type.Amount.FromColumn] != nil ||
		queryResults[table].(map[string]interface{})[field.Type.Currency.FromColumn] != nil {
		formatted[field.Key] =
			resultAsWorkflowMoneyType(field, queryResults, table)
		return formatted
	}
	formatted[field.Key] = nil
	return formatted
}

func buildForFieldTypeText(formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	if queryResults[table].(map[string]interface{})[field.FromColumn] != nil {
		formatted[field.Key] =
			stringify(queryResults[table].(map[string]interface{})[field.FromColumn])
		return formatted
	}
	formatted[field.Key] = nil
	return formatted
}

func buildForFieldTypeDateOrTime(formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	if queryResults[table].(map[string]interface{})[field.FromColumn] != nil {
		dateTime := queryResults[table].(map[string]interface{})[field.FromColumn].(time.Time)
		// Don't convert dateTime to UTC since when a DATE type is coerced
		// into a *time.Time it can contain the database's timezone.
		// Converting the dateTime to UTC can change the original
		// date from 2006-01-02T00:00:00+01:00 to
		// 2006-01-01T23:00:00+0:00 when in UTC
		formatted[field.Key] = dateTime.Format("2006-01-02T15:04:05.000Z")
		return formatted
	}
	formatted[field.Key] = nil
	return formatted
}
func buildForFieldTypeDateTime(formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	if queryResults[table].(map[string]interface{})[field.FromColumn] != nil {
		dateTime := queryResults[table].(map[string]interface{})[field.FromColumn].(time.Time)
		formatted[field.Key] = dateTime.UTC().Format("2006-01-02T15:04:05.000Z")
		return formatted
	}
	formatted[field.Key] = nil
	return formatted
}

func buildForFieldTypeOther(formatted, queryResults map[string]interface{}, table string, field *descriptor.Field) map[string]interface{} {
	if queryResults[table].(map[string]interface{})[field.FromColumn] != nil {
		formatted[field.Key] =
			queryResults[table].(map[string]interface{})[field.FromColumn]
		return formatted
	}
	formatted[field.Key] = nil
	return formatted
}

func resultAsWorkflowMoneyType(field *descriptor.Field, queryResults map[string]interface{}, table string) map[string]interface{} {
	result := make(map[string]interface{})
	var currency interface{}
	if field.Type.Currency.FromColumn == "" {
		if field.Type.Currency.Value == "" {
			// Default to EUR if no other information is provided
			currency = "EUR"
		} else {
			// Otherwise use the currency that the user defines
			// in the `value` field
			currency = field.Type.Currency.Value
		}
	} else {
		currency = queryResults[table].(map[string]interface{})[field.Type.Currency.FromColumn]
	}
	result = map[string]interface{}{
		"amount":   queryResults[table].(map[string]interface{})[field.Type.Amount.FromColumn],
		"currency": currency,
	}
	return result
}

func relatedTablesResultSetNotEmpty(queryResults map[string]interface{}, table string, field *descriptor.Field) bool {
	fieldKey := queryResults[table].(map[string]interface{})[field.Key].(map[string]interface{})
	fieldKeyRelationshipWithTable := fieldKey[field.Relationship.WithTable].([]map[string]interface{})
	return len(fieldKeyRelationshipWithTable) > 0
}
func stringify(value interface{}) (stringified interface{}) {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case int64:
		stringified = fmt.Sprintf("%d", v)
	case float64:
		stringified = fmt.Sprintf("%f", v)
	case time.Time:
		stringified = v.String()
	case string:
		stringified = v
	case map[string]interface{}:
		idAndNameStringified := make(map[string]interface{})
		for ki, vi := range v {
			idAndNameStringified[ki] = stringify(vi)
		}
		stringified = idAndNameStringified
	}
	return stringified
}

func tableHasRelationships(queryResults map[string]interface{}, table string, field *descriptor.Field) bool {
	return field.Relationship != nil && queryResults[table].(map[string]interface{})[field.Key] != nil
}

func clientWantsDenormalizedResultSet(queryValue string) bool {
	return queryValue != ""
}
func subsetForPerformance(in []interface{}) []interface{} {
	// To avoid performance issues, return only the first
	// 42 results when querying the /options route
	if len(in) > 42 {
		return in[0:42]
	}
	return in
}
func withRelationshipFieldsOmitted(table string) (fields []*descriptor.Field) {
	typeDescriptor := util.GetTypeDescriptorUsingDBTableName(
		config.Options.Descriptor.TypeDescriptors,
		table,
	)
	for _, field := range typeDescriptor.Fields {
		if field.Relationship == nil {
			fields = append(fields, field)
		}
	}
	return fields
}
