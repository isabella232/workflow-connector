// Package sql defines a Backend that is responsible for communicating
// with SQL databases

package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/signavio/workflow-connector/pkg/config"
	"github.com/signavio/workflow-connector/pkg/util"
)

var (
	ErrPostFormEmpty          = errors.New("Form data sent was either empty, incomplete or of an unsupported type")
	ErrCardinalityMany        = errors.New("Form data contained multiple input values for a single column")
	ErrUnexpectedJSON         = errors.New("Received JSON data that we are unable to parse")
	ErrMismatchedAffectedRows = errors.New("The amount of rows affected should be sane")
)

type Backend struct {
	Cfg                       *config.Config
	ConvertDBSpecificDataType func(string) interface{}
	DB                        *sql.DB
	Queries                   map[string]string
	RequestData               map[string]interface{}
	Templates                 map[string]string
	getSingle                 getSingle
}
type getSingle struct {
	ctx         context.Context
	id          string
	backend     *Backend
	columnNames []string
	dataTypes   []interface{}
	table       string
}
type getCollection struct {
	ctx         context.Context
	backend     *Backend
	columnNames []string
	dataTypes   []interface{}
}
type getSingleAsOption struct {
	ctx         context.Context
	id          string
	backend     *Backend
	query       string
	columnNames []string
	dataTypes   []interface{}
}
type getCollectionAsOptions struct {
	ctx         context.Context
	backend     *Backend
	query       string
	columnNames []string
	dataTypes   []interface{}
}
type getCollectionAsOptionsFilterable struct {
	ctx         context.Context
	filter      string
	backend     *Backend
	query       string
	columnNames []string
	dataTypes   []interface{}
}
type updateSingle struct {
	backend *Backend
	request *http.Request
	id      string
}
type createSingle struct {
	backend *Backend
	request *http.Request
}

// NewBackend ...
func NewBackend(cfg *config.Config) (b *Backend) {
	return &Backend{
		Cfg:       cfg,
		Queries:   make(map[string]string),
		Templates: make(map[string]string),
	}
}

// Open a connection to the backend database
func (b *Backend) Open(driver, url string) error {
	db, err := sql.Open(driver, url)
	if err != nil {
		return fmt.Errorf("Error opening connection to database: %s", err)
	}
	b.DB = db
	return nil
}

// Tear down any connections opened by the endpoint
func (b *Backend) TearDown() {
	b.DB.Close()
}

// Fetch data from database
func (b *Backend) GetSingle(req *http.Request) (response []interface{}, err error) {
	requestID := mux.Vars(req)["id"]
	route := &getSingle{
		ctx:     req.Context(),
		id:      requestID,
		backend: b,
	}
	return route.handle()
}

func (b *Backend) GetCollection(req *http.Request) (response []interface{}, err error) {
	route := &getCollection{
		ctx:     req.Context(),
		backend: b,
	}
	return route.handle()
}

// Fetch Option data from database
func (b *Backend) GetSingleAsOption(req *http.Request) (response []interface{}, err error) {
	requestID := mux.Vars(req)["id"]
	tableFromRequest := mux.Vars(req)["table"]
	columnAsOptionName := req.Context().Value(config.ContextKey("columnAsOptionName")).(string)
	query := fmt.Sprintf(b.Queries["GetSingleAsOption"], columnAsOptionName, tableFromRequest)
	route := &getSingleAsOption{
		ctx:     req.Context(),
		id:      requestID,
		backend: b,
		query:   query,
	}
	return route.handle()
}

// Fetch Options from database
func (b *Backend) GetCollectionAsOptions(req *http.Request) (response []interface{}, err error) {
	tableFromRequest := mux.Vars(req)["table"]
	columnAsOptionName := req.Context().Value(config.ContextKey("columnAsOptionName")).(string)
	query := fmt.Sprintf(b.Queries["GetCollectionAsOptions"], columnAsOptionName, tableFromRequest)
	route := &getCollectionAsOptions{
		ctx:     req.Context(),
		backend: b,
		query:   query,
	}
	return route.handle()
}

// Fetch Options from database
func (b *Backend) GetCollectionAsOptionsFilterable(req *http.Request) (response []interface{}, err error) {
	filter := mux.Vars(req)["filter"]
	tableFromRequest := mux.Vars(req)["table"]
	columnAsOptionName := req.Context().Value(config.ContextKey("columnAsOptionName")).(string)
	query := fmt.Sprintf(b.Queries["GetCollectionAsOptionsFilterable"], columnAsOptionName, tableFromRequest, columnAsOptionName)
	route := &getCollectionAsOptionsFilterable{
		ctx:     req.Context(),
		backend: b,
		filter:  fmt.Sprintf("%%%s%%", filter),
		query:   query,
	}
	return route.handle()
}

// Update data from database
func (b *Backend) UpdateSingle(req *http.Request) (response []interface{}, err error) {
	requestID := mux.Vars(req)["id"]
	route := &updateSingle{
		backend: b,
		request: req,
		id:      requestID,
	}
	return route.handle()
}

// Create data from database
func (b *Backend) CreateSingle(req *http.Request) (response []interface{}, err error) {
	route := &createSingle{
		backend: b,
		request: req,
	}
	return route.handle()
}

// insert context
func (b *Backend) SaveTableSchemas() (err error) {
	for _, table := range b.Cfg.Database.Tables {
		query := fmt.Sprintf(b.Queries["GetTableSchema"], table.Name)
		b.Cfg.TableSchemas[table.Name], err = b.getTableSchema(query, table.Name)
		if err != nil {
			return fmt.Errorf("Unable to retrieve columns and data types from table schema: %s", err)
		}
	}
	for _, table := range b.Cfg.Database.Tables {
		if util.TableHasRelationships(b.Cfg, table.Name) {
			ctx := util.ContextWithRelationships(
				context.Background(),
				b.Cfg.Descriptor.TypeDescriptors,
				table.Name,
			)
			queryText, err := b.interpolateGetTemplate(
				ctx,
				b.Templates["GetTableWithRelationshipsSchema"],
				table.Name,
			)
			if err != nil {
				return fmt.Errorf("Unable to interpolate queryText to retrieve table schema: %s", err)
			}
			b.Cfg.TableSchemas[fmt.Sprintf("%s_relationships", table.Name)], err =
				b.getTableSchemaWithRelationships(queryText, table.Name)
		}
	}
	return nil
}

func (b *Backend) getTableSchemaWithRelationships(query, table string) (*config.TableSchema, error) {
	var dataTypes []interface{}
	var columnNames []string
	var columnsPrepended []string
	rows, err := b.DB.Query(query)
	if err != nil {
		return nil, err
	}
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	td := util.TypeDescriptorForCurrentTable(b.Cfg.Descriptor.TypeDescriptors, table)
	fields := util.TypeDescriptorRelationships(td)
	currentColumnsIdx := len(b.Cfg.TableSchemas[table].ColumnNames)
	currentColumns := columns[0:currentColumnsIdx]
	for _, cc := range currentColumns {
		columnsPrepended = append(
			columnsPrepended,
			fmt.Sprintf("%s_%s", table, cc),
		)
	}
	var previousTableIdx = currentColumnsIdx
	var newTableIdx = 0
	for _, field := range fields {
		thisTable := field.Relationship.WithTable
		newTableIdx = previousTableIdx + len(b.Cfg.TableSchemas[thisTable].ColumnNames)
		currentColumns := columns[previousTableIdx:newTableIdx]
		for _, cc := range currentColumns {
			columnsPrepended = append(
				columnsPrepended,
				fmt.Sprintf("%s_%s", thisTable, cc),
			)
		}
		previousTableIdx = newTableIdx
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	for i := range columnTypes {
		dataType := columnTypes[i].DatabaseTypeName()
		nativeType := b.ConvertDBSpecificDataType(dataType)
		dataTypes = append(dataTypes, nativeType)
		columnNames = append(columnNames, columnsPrepended[i])
	}
	return &config.TableSchema{columnNames, dataTypes}, nil
}

func (b *Backend) getTableSchema(query, table string) (*config.TableSchema, error) {
	var dataTypes []interface{}
	var columnNames []string
	var columnsPrepended []string

	rows, err := b.DB.Query(query)
	if err != nil {
		return nil, err
	}
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	for _, column := range columns {
		columnsPrepended = append(
			columnsPrepended,
			fmt.Sprintf("%s_%s", table, column),
		)
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	for i := range columnTypes {
		dataType := columnTypes[i].DatabaseTypeName()
		nativeType := b.ConvertDBSpecificDataType(dataType)
		dataTypes = append(dataTypes, nativeType)
		columnNames = append(columnNames, columnsPrepended[i])
	}
	return &config.TableSchema{columnNames, dataTypes}, nil
}

func parseDataForm(req *http.Request) (data map[string]interface{}, err error) {
	switch req.Header.Get("Content-Type") {
	case "application/json":
		return parseApplicationJSON(req)
	case "application/x-www-form-urlencoded":
		return parseFormURLEncoded(req)
	}
	return nil, ErrPostFormEmpty
}

func parseFormURLEncoded(req *http.Request) (data map[string]interface{}, err error) {
	if err := req.ParseForm(); err != nil {
		return nil, err
	}
	if len(req.PostForm) == 0 {
		return nil, ErrPostFormEmpty
	}
	data = make(map[string]interface{})
	for k, v := range req.PostForm {
		if len(v) > 1 {
			return nil, ErrCardinalityMany
		}
		data[k] = v[0]
	}
	return data, nil
}

func parseApplicationJSON(req *http.Request) (data map[string]interface{}, err error) {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	defer req.Body.Close()
	data = make(map[string]interface{})
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf(ErrUnexpectedJSON.Error()+": %v\n", err)
	}
	return
}

type NullTime struct {
	Time  time.Time
	Valid bool // Valid is true if Time is not NULL
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
