package util

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/mux"
	"github.com/signavio/workflow-connector/internal/pkg/config"
	"github.com/signavio/workflow-connector/internal/pkg/descriptor"
)

// ContextKey is used as a key when populating a context.Context with values
type ContextKey string

type ResponseMessage struct {
	Code int
	Tx   string
	Msg  string
}

type NullTime struct {
	Time  time.Time
	Valid bool // Valid is true if Time is not NULL
}

var (
	ErrCardinalityMany = errors.New("Form data contained multiple input values for a single column")
	ErrUnexpectedJSON  = errors.New("Received JSON data that we are unable to parse")
)

func (rm *ResponseMessage) Byte() []byte {
	msg := map[string]interface{}{
		"status": map[string]interface{}{
			"code":        rm.Code,
			"description": rm.Msg,
		},
	}
	if rm.Tx != "" {
		msg["status"].(map[string]interface{})["tx"] = rm.Tx
	}
	result, err := json.MarshalIndent(&msg, "", "  ")
	if err != nil {
		panic(err)
	}
	return result
}
func (rm *ResponseMessage) String() string {
	return string(rm.Byte()[:])
}
func (rm *ResponseMessage) Error() string {
	return rm.String()
}

// GetColumnNamesFromRequestData will, for every parameter in the request data,
// retrieve the corresponding table column name
func GetColumnNamesFromRequestData(tableName string, requestData map[string]interface{}) (columnNames []string) {
	td := GetTypeDescriptorUsingDBTableName(
		config.Options.Descriptor.TypeDescriptors,
		tableName,
	)
	for _, field := range td.Fields {
		if field.Type.Name == "money" {
			if _, ok := requestData[field.Type.Amount.Key]; ok {
				columnNames = append(columnNames, field.Type.Amount.FromColumn)
			}
			if _, ok := requestData[field.Type.Currency.Key]; ok {
				columnNames = append(columnNames, field.Type.Currency.FromColumn)
			}
		} else {
			if _, ok := requestData[field.Key]; ok {
				columnNames = append(columnNames, field.FromColumn)
			}
		}
	}
	return
}
// GetTypeDescriptorUsingDBTableName will return the typeDescriptor from the descriptor.json
// file defined for the table provided in the function's second parameter
func GetTypeDescriptorUsingDBTableName(typeDescriptors []*descriptor.TypeDescriptor, tableName string) (td *descriptor.TypeDescriptor) {
	for _, typeDescriptor := range typeDescriptors {
		if tableName == typeDescriptor.TableName {
			td = typeDescriptor
		}
	}
	return
}

// GetDBTableNameUsingTypeDescriptorKey will return the typeDescriptor from the descriptor.json file defined for the table provided in the function's second parameter
func GetDBTableNameUsingTypeDescriptorKey(typeDescriptors []*descriptor.TypeDescriptor, typeDescriptorKey string) (tableName string, ok bool) {
	for _, typeDescriptor := range typeDescriptors {
		if typeDescriptorKey == typeDescriptor.Key {
			tableName = typeDescriptor.TableName
			return tableName, true
		}
	}
	return "", false
}

func GetTypeDescriptorUsingTypeDescriptorKey(typeDescriptors []*descriptor.TypeDescriptor, typeDescriptorKey string) (result *descriptor.TypeDescriptor) {
	for _, typeDescriptor := range typeDescriptors {
		if typeDescriptorKey == typeDescriptor.Key {
			result = typeDescriptor
		}
	}
	return
}
func GetColumnNameAndTypeFromQueryParameterName(typeDescriptors []*descriptor.TypeDescriptor, tableName string, paramName string) (columnName, columnType string, ok bool) {
	typeDescriptor := GetTypeDescriptorUsingDBTableName(typeDescriptors, tableName)
	for _, field := range typeDescriptor.Fields {
		switch field.Type.Name {
		case "money":
			if field.Type.Amount.Key == paramName {
				return field.Type.Amount.FromColumn, field.Type.Name, true
			}
			if field.Type.Currency.Key == paramName {
				return field.Type.Currency.FromColumn, field.Type.Name, true
			}
		case "date":
			if field.Key == paramName {
				return field.FromColumn, field.Type.Kind, true
			}

		default:
			if field.Key == paramName {
				return field.FromColumn, field.Type.Name, true
			}
		}
	}
	return "", "", false
}

// ContextWithRelationships will return a new context which will included an array of all relationships for the table provided in the function's second parameter
func ContextWithRelationships(ctx context.Context, typeDescriptors []*descriptor.TypeDescriptor, table string) context.Context {
	typeDescriptor := GetTypeDescriptorUsingDBTableName(typeDescriptors, table)
	relationships := TypeDescriptorRelationships(typeDescriptor)
	return context.WithValue(ctx, ContextKey("relationships"), relationships)
}

func TableHasRelationships(cfg config.Config, table string) bool {
	result := false
	td := GetTypeDescriptorUsingDBTableName(cfg.Descriptor.TypeDescriptors, table)
	if td.TableName == table {
		if TypeDescriptorRelationships(td) != nil {
			result = true
		}
	}
	return result
}

func TypeDescriptorRelationships(td *descriptor.TypeDescriptor) []*descriptor.Field {
	var relationships []*descriptor.Field
	for _, field := range td.Fields {
		if field.Relationship != nil {
			relationships = append(relationships, field)
		}
	}
	return relationships
}

func AppendNoDuplicates(list []map[string]interface{}, item map[string]interface{}) []map[string]interface{} {
	exists := false
	for _, l := range list {
		// TODO: Not performant
		if reflect.DeepEqual(l, item) {
			exists = true
		}
	}
	if !exists {
		return append(list, item)
	}
	return list
}

func ParseDataForm(req *http.Request) (data map[string]interface{}, err error) {
	if req.Method == "GET" {
		return parseURLValues(req)
	}
	switch req.Header.Get("Content-Type") {
	case "application/json":
		return parseApplicationJSON(req)
	default:
		return parseFormURLEncoded(req)
	}
}

func parseURLValues(req *http.Request) (data map[string]interface{}, err error) {
	u := req.URL.Query()
	data = make(map[string]interface{})
	// We assume the last occurence of a provided
	// query will be the one the user actually
	// wants to use
	for k, v := range u {
		data[k] = v[len(v)-1]
	}
	return data, nil
}

func parseFormURLEncoded(req *http.Request) (data map[string]interface{}, err error) {
	if err := req.ParseForm(); err != nil {
		return nil, err
	}
	if len(req.Form) == 0 {
		body, _ := ioutil.ReadAll(req.Body)
		defer req.Body.Close()
		return nil, fmt.Errorf("Unable to parse the request body:\n%s\n", body)
	}
	data = make(map[string]interface{})
	for k, v := range req.Form {
		if len(v) > 1 {
			return nil, ErrCardinalityMany
		}
		data[k] = v[0]
	}
	return data, nil
}

func parseApplicationJSON(req *http.Request) (data map[string]interface{}, err error) {
	if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf(ErrUnexpectedJSON.Error()+": %v\n", err)
	}
	return
}

func IsOptionsRoute(req *http.Request) bool {
	currentRoute := mux.CurrentRoute(req).GetName()
	if currentRoute == "GetCollectionAsOptionsFilterable" ||
		currentRoute == "GetCollectionAsOptions" {
		return true
	}
	return false
}

func IsOptionRoute(req *http.Request) bool {
	currentRoute := mux.CurrentRoute(req).GetName()
	if currentRoute == "GetSingleAsOption" {
		return true
	}
	return false
}

// Scan implements the Scanner interface.
func (nt *NullTime) Scan(value interface{}) error {
	nt.Time, nt.Valid = value.(time.Time)
	return nil
}

// Value implements the driver Valuer interface.
func (nt NullTime) Value() (driver.Value, error) {
	if !nt.Valid {
		return nil, nil
	}
	return nt.Time, nil
}
