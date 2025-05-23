package mongrator

import (
	"log"
	"reflect"
	"strings"

	"github.com/gobeam/stringy"
	"github.com/iancoleman/orderedmap"
)

func compareFields(collectionFields, schemaFields, fieldsToUpdate *orderedmap.OrderedMap) {
	for _, field := range schemaFields.Keys() {
		value, ok := schemaFields.Get(field)
		if !ok {
			continue
		}
		schemaFieldType := value.(string)
		if field == "id" || field == "_id" {
			continue
		}
		collectionValue, ok := collectionFields.Get(field)
		if !ok {
			fieldsToUpdate.Set(field, schemaFieldType)
			continue
		}
		collectionFieldType := collectionValue.(string)
		if schemaFieldType != collectionFieldType {
			fieldsToUpdate.Set(field, value)
			continue
		}
	}
}

func getRemovedFields(collectionFields *orderedmap.OrderedMap, schemaFields *orderedmap.OrderedMap) []string {
	fieldsToRemove := make([]string, 0)
	for _, field := range collectionFields.Keys() {
		if field == "id" || field == "_id" {
			continue
		}
		if _, ok := schemaFields.Get(field); !ok {
			fieldsToRemove = append(fieldsToRemove, field)
		}
	}
	return fieldsToRemove
}

func extractSchemaFields(prefix string, schema any, fields *orderedmap.OrderedMap) {
	dataValue := reflect.ValueOf(schema)
	dataType := dataValue.Type()
	if dataType.Kind() != reflect.Struct {
		log.Printf("Schema '%s' must be a struct", dataType.Name())
		return
	}

	for i := 0; i < dataType.NumField(); i++ {
		field := dataType.Field(i)
		fieldValue := dataValue.Field(i)
		fieldTag := field.Tag.Get("bson")

		if fieldTag == "" || strings.Contains(fieldTag, "-") {
			continue
		}

		if strings.Contains(fieldTag, "id") || strings.Contains(fieldTag, "_id") {
			continue
		}

		if bsonTags := strings.Split(fieldTag, ","); len(bsonTags) > 0 {
			fieldTag = bsonTags[0]
		}

		fullField := fieldTag
		if prefix != "" {
			fullField = prefix + "." + fieldTag
		}
		switch fieldValue.Kind() {
		case reflect.Struct:
			if fieldValue.Type().Name() == "Time" {
				str := stringy.New(field.Name).SnakeCase().ToLower()
				fields.Set(str, "time")
				continue
			}
			fields.Set(fullField, fieldValue.Kind().String())
			extractSchemaFields(fullField, fieldValue.Interface(), fields)
		case reflect.Slice:
			elemType := fieldValue.Type().Elem()
			fields.Set(fullField, "slice")
			if elemType.Kind() == reflect.Struct {
				value := reflect.New(elemType).Elem().Interface()
				extractSchemaFields(fullField+".$[]", value, fields)
			}
		default:
			fields.Set(fullField, fieldValue.Kind().String())
		}
	}
}
