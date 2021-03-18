package descriptor

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
)

type Descriptor struct {
	Key             string            `json:"key,omitempty"`
	Name            string            `json:"name,omitempty"`
	Description     string            `json:"description,omitempty"`
	TypeDescriptors []*TypeDescriptor `json:"typeDescriptors,omitempty"`
	Version         int               `json:"version,omitempty"`
	ProtocolVersion int               `json:"protocolVersion,omitempty"`
}

type TypeDescriptor struct {
	Key                string       `json:"key,omitempty"`
	Name               string       `json:"name,omitempty"`
	TableName          string       `json:"tableName,omitempty"`
	ColumnAsOptionName string       `json:"columnAsOptionName,omitempty"`
	UniqueIdColumn     string       `json:"uniqueIdColumn,omitempty"`
	RecordType         string       `json:"recordType,omitempty"`
	Parameters         []*Parameter `json:"parameters,omitempty"`
	Fields             []*Field     `json:"fields,omitempty"`
	OptionsAvailable   bool         `json:"optionsAvailable,omitempty"`
	FetchOneAvailable  bool         `json:"fetchOneAvailable,omitempty"`
}

type Parameter struct {
	Key  string        `json:"key,omitempty"`
	Name string        `json:"name,omitempty"`
	Type *WorkflowType `json:"type,omitempty"`
}
type Field struct {
	Key          string        `json:"key,omitempty"`
	Name         string        `json:"name,omitempty"`
	Type         *WorkflowType `json:"type,omitempty"`
	FromColumn   string        `json:"fromColumn,omitempty"`
	Relationship *Relationship `json:"relationship,omitempty"`
}

type WorkflowType struct {
	Name        string       `json:"name,omitempty"`
	Kind        string       `json:"kind,omitempty"`
	Amount      *Amount      `json:"amount,omitempty"`
	Currency    *Currency    `json:"currency,omitempty"`
	Options     []*Option    `json:"options,omitempty"`
	ElementType *ElementType `json:"elementType,omitempty"`
	MultiLine   bool         `json:"multiLine,omitempty"`
}
type Amount struct {
	Key        string `json:"key,omitempty"`
	FromColumn string `json:"fromColumn,omitempty"`
}

type Currency struct {
	Key        string `json:"key,omitempty"`
	FromColumn string `json:"fromColumn,omitempty"`
	Value      string `json:"value,omitempty"`
}

type Option struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}
type ElementType struct {
	Name string `json:"name,omitempty"`
}

type Relationship struct {
	Kind                       string `json:"kind,omitempty"`
	WithTable                  string `json:"withTable,omitempty"`
	ForeignTableUniqueIdColumn string `json:"foreignTableUniqueIdColumn,omitempty"`
	LocalTableUniqueIdColumn   string `json:"localTableUniqueIdColumn,omitempty"`
}

// SchemaMapping defines the schema of data retrieved from a particular backend
type SchemaMapping struct {
	FieldNames    []string
	BackendTypes  []interface{}
	GolangTypes   []interface{}
	WorkflowTypes []interface{}
}

// ParseDescriptorFile will parse the descriptor.json file and make sure
// to add an `id` field if the user has not already specified it
func ParseDescriptorFile(file io.Reader) (descriptor *Descriptor) {
	var content []byte
	content, err := ioutil.ReadAll(file)
	if err != nil {
		panic(fmt.Errorf("Unable to read descriptor.json file: %v", err))
	}
	err = json.Unmarshal(content, &descriptor)
	if err != nil {
		panic(fmt.Errorf("Unable to unmarshal descriptor.json: %v", err))
	}
	if err := performSanityChecks(descriptor); err != nil {
		panic(err)
	}
	return
}
func performSanityChecks(descriptor *Descriptor) error {
	for _, td := range descriptor.TypeDescriptors {
		if err := errUniqueIdColumnAndIdColumnDiffer(td); err != nil {
			return err
		}
		if err := errColumnAsOptionNameAndNameColumnDiffer(td); err != nil {
			return err
		}
		for _, field := range td.Fields {
			if err := errCurrencyHasDefaultValue(field, td.Key); err != nil {
				return err
			}
			if err := errFromColumnPropertyIsMissing(field); err != nil {
				return err
			}
			if err := errTypeNameIsMissing(field); err != nil {
				return err
			}
		}
	}
	return nil
}

func errUniqueIdColumnAndIdColumnDiffer(td *TypeDescriptor) error {
	msg := "The `uniqueIdColumn` property for type descriptor `%s` must be set " +
		"to `id` when the type descriptor contains a field called `id`"
	for _, field := range td.Fields {
		if field.Key == "id" && td.UniqueIdColumn != "id" {
			return fmt.Errorf(msg, td.Key)
		}
	}
	return nil
}

func errColumnAsOptionNameAndNameColumnDiffer(td *TypeDescriptor) error {
	msg := "The `columnAsOptionName` property for type descriptor `%s` must be set " +
		"to `name` when the type descriptor contains a field called `name`"
	for _, field := range td.Fields {
		if field.Key == "name" && td.ColumnAsOptionName != "name" {
			return fmt.Errorf(msg, td.Key)
		}
	}
	return nil
}

func errTypeNameIsMissing(field *Field) error {
	msg := "Unable to parse descriptor.json: " +
		"%s should not have an empty type name"
	if len(field.Type.Name) == 0 {
		return fmt.Errorf(
			msg,
			field.Key,
		)
	}
	return nil
}

func errCurrencyHasDefaultValue(field *Field, td string) error {
	msg := "Unable to parse descriptor.json: " +
		"%s.%s specifies a default currency value" +
		"*and* a fromColumn. You must specify *only* one."
	if field.Type.Name == "money" {
		if field.Type.Currency.Value != "" &&
			field.Type.Currency.FromColumn != "" {
			return fmt.Errorf(
				msg,
				td,
				field.Key,
			)
		}
	}
	return nil
}

func errFromColumnPropertyIsMissing(field *Field) error {
	msg := "Unable to parse descriptor.json: " +
		"field of type '%s' should contain a fromColumn property"
	if field.Type.Name != "money" && field.Relationship == nil {
		if field.FromColumn == "" {
			return fmt.Errorf(msg, field.Type.Name)
		}
	}
	return nil
}
