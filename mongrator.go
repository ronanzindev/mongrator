package mongrator

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/RonanzinDev/mongrator/utils"
	"github.com/gobeam/stringy"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/mgo.v2/bson"
)

const migratorCollectionName = "migrations"
const collectionFieldName = "migrations_collections_fields"

type (
	migration struct {
		Collection string    `json:"collection" bson:"collection"`
		Migration  string    `json:"migration" bson:"migration"`
		CreatedAt  time.Time `json:"created_at" bson:"created_at"`
	}
	mongrator struct {
		database           *mongo.Database
		migrationCol       *mongo.Collection
		migrationFieldsCol *mongo.Collection
		collections        map[string]any
	}
	mongratorFields struct {
		Collection string    `bson:"collection"`
		Fields     document  `bson:"fields"`
		CreatedAt  time.Time `bson:"created_at"`
	}
	fieldsComparation struct {
		fieldsToAdd, fieldUpdateType map[string]interface{}
	}
	document = map[string]string
)

// Initialize the migrator
func New(database *mongo.Database) *mongrator {
	mongrator := &mongrator{
		database:    database,
		collections: make(map[string]any),
	}

	collections, _ := mongrator.database.ListCollectionNames(context.Background(), bson.M{})
	if !slices.Contains(collections, migratorCollectionName) {
		if err := mongrator.database.CreateCollection(context.Background(), migratorCollectionName); err != nil {
			log.Printf("Error to create 'migrations' collection. Err -> %s", err.Error())
		}
	}
	if !slices.Contains(collections, collectionFieldName) {
		if err := mongrator.database.CreateCollection(context.Background(), collectionFieldName); err != nil {
			log.Printf("Error to create 'migrations' collection. Err -> %s", err.Error())
		}
	}
	mongratorCol := mongrator.database.Collection(migratorCollectionName)
	mongratorFieldsCol := mongrator.database.Collection(collectionFieldName)
	mongrator.migrationCol = mongratorCol
	mongrator.migrationFieldsCol = mongratorFieldsCol
	return mongrator
}

// Registers a schema and its corresponding collection for automatic migration during application startup.
func (m *mongrator) RegisterSchema(collection string, schema any) {
	value := reflect.ValueOf(schema)
	if value.Kind() != reflect.Struct {
		log.Printf("Schema of '%s' must be a struct\n", collection)
		return
	}
	collections, err := m.database.ListCollectionNames(context.Background(), bson.M{})
	if err != nil {
		log.Printf("Error list collections: err -> %s", err.Error())
		return
	}
	if !slices.Contains(collections, collection) {
		err = m.database.CreateCollection(context.Background(), collection)
		if err != nil {
			log.Printf("Error create collection '%s', err -> %s", collection, err.Error())
			return
		}
	}
	var savedFields mongratorFields
	err = m.migrationFieldsCol.FindOne(context.Background(), bson.M{"collection": collection}).Decode(&savedFields)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			fields := make(document)
			extractSchemaFields("", schema, fields)
			savedFields := mongratorFields{
				Collection: collection,
				Fields:     fields,
			}
			if _, err := m.migrationFieldsCol.InsertOne(context.Background(), savedFields); err != nil {
				log.Printf("Error saving fields from '%s' collection", collection)
				return
			}
		}
	}
	m.collections[collection] = schema
}

func (m *mongrator) RunMigrations() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for collection, schema := range m.collections {
		wg.Add(1)
		go func(name string, schema any) {
			defer wg.Done()
			var collectionFields mongratorFields
			if err := m.migrationFieldsCol.FindOne(ctx, bson.M{"collection": collection}).Decode(&collectionFields); err != nil {
				log.Printf("Error to get fields from collection '%s', err -> %s", collection, err.Error())
				return
			}
			schemaFields := make(document)
			extractSchemaFields("", schema, schemaFields)
			updateFields := make(document)
			schemaCollection := m.database.Collection(collection)
			compareFields(collectionFields.Fields, schemaFields, updateFields)
			if len(updateFields) > 0 {
				m.updateFields(updateFields, schemaCollection, collectionFields.Fields)
				m.updateMigrationFields(collection, updateFields, collectionFields.Fields)
			}
			fieldsToRemove := getRemovedFields(collectionFields.Fields, schemaFields)
			if len(fieldsToRemove) > 0 {
				m.removeFields(fieldsToRemove, schemaCollection)
				m.removeFieldsFromMongratorCollection(fieldsToRemove, collection, collectionFields.Fields)
			}
		}(collection, schema)
	}
	wg.Wait()
}
func (m *mongrator) saveMigrationLog(message string, collection string, action string) {
	migration := migration{Collection: collection, Migration: message, CreatedAt: bson.Now()}
	if _, err := m.migrationCol.InsertOne(context.Background(), migration); err != nil {
		log.Printf("Error saving migration log when %s\n", action)
	}
}
func (m *mongrator) updateFields(fields document, collection *mongo.Collection, collectionSchema document) {
	collectionName := collection.Name()
	for field, value := range fields {
		var action string
		var actionLog string
		typeValue, ok := utils.Types[value]
		if !ok {
			continue
		}
		if _, ok := collectionSchema[field]; ok {
			action = "updated"
			actionLog = "updating"
		} else {
			action = "added"
			actionLog = "adding"
		}
		if strings.Contains(field, "[]") {
			if _, ok := collectionSchema[field]; !ok {
				splitedField := strings.Split(field, ".")
				if len(splitedField) > 1 {
					fieldName := splitedField[0]
					_, err := collection.UpdateMany(context.Background(), bson.M{}, bson.M{"$set": bson.M{fieldName: make([]any, 0)}})
					if err != nil {
						log.Printf("Error to added array to collection '%s', err -> %s", collectionName, err.Error())
						continue
					}
				}
			}
		}
		_, err := collection.UpdateMany(context.Background(), bson.M{}, bson.M{"$set": bson.M{field: typeValue}})
		if err != nil {
			log.Printf("Error %s field '%s' -> collection' %s': %v\n", action, field, collectionName, err)
			continue
		}
		message := fmt.Sprintf("Field '%s' %s -> collection '%s'", field, action, collectionName)
		m.saveMigrationLog(message, collectionName, actionLog+" fields")
		log.Printf("%s\n", message)
	}

}
func (m *mongrator) updateMigrationFields(collection string, fields, collectionSchema document) {
	value := fields
	for field, value := range value {
		collectionSchema[field] = value
	}
	if _, err := m.migrationFieldsCol.UpdateOne(context.Background(), bson.M{"collection": collection}, bson.M{"$set": bson.M{"fields": collectionSchema, "updated_at": bson.Now()}}); err != nil {
		log.Printf("Error updating migration fields from collection : '%s'", collection)
	}

}
func (m *mongrator) removeFields(fielsToRemove []string, collection *mongo.Collection) {
	collectionName := collection.Name()
	for _, field := range fielsToRemove {
		_, err := collection.UpdateMany(context.Background(), bson.M{}, bson.M{"$unset": bson.M{field: ""}})
		if err != nil {
			log.Printf("Error to remove field '%s' from collection '%s'", field, collectionName)
			continue
		}
		log.Printf("Field '%s' removed from collection '%s'", field, collectionName)
	}
}
func (m *mongrator) removeFieldsFromMongratorCollection(fielsToRemove []string, collection string, collectionSchema document) {
	value := collectionSchema
	for _, field := range fielsToRemove {
		delete(value, field)
	}
	if _, err := m.migrationFieldsCol.UpdateMany(context.Background(), bson.M{"collection": collection}, bson.M{"$set": bson.M{"fields": value}}); err != nil {
		log.Printf("Error to remove field from mongrator field collections")
	}
}
func compareFields(collectionFields, schemaFields, fieldsToUpdate document) {
	for field, value := range schemaFields {
		if field == "id" || field == "_id" {
			continue
		}
		collectionValue, ok := collectionFields[field]
		if !ok {
			fieldsToUpdate[field] = value
			continue
		}
		if value != collectionValue {
			fieldsToUpdate[field] = value
			continue
		}
	}
}

func getRemovedFields(collectionFields, schemaFields document) []string {
	fieldsToRemove := make([]string, 0)
	for field := range collectionFields {
		if field == "id" || field == "_id" {
			continue
		}
		if _, ok := schemaFields[field]; !ok {
			fieldsToRemove = append(fieldsToRemove, field)
		}
	}
	return fieldsToRemove
}

func extractSchemaFields(prefix string, schema any, fields map[string]string) {
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
			log.Printf("Field '%s' of schema '%s' must has a bson tag", field.Name, dataValue.Type().Name())
			continue
		}
		if fieldTag == "id" || fieldTag == "_id" {
			continue
		}
		fullField := fieldTag
		if prefix != "" {
			fullField = prefix + "." + fullField
		}
		switch fieldValue.Kind() {
		case reflect.Struct:
			if fieldValue.Type().Name() == "Time" {
				str := stringy.New(field.Name).SnakeCase().ToLower()
				fields[str] = "time"
				continue
			}
			extractSchemaFields(fullField, fieldValue.Interface(), fields)
		case reflect.Slice:
			elemType := fieldValue.Type().Elem()
			if elemType.Kind() == reflect.Struct {
				value := reflect.Zero(elemType).Interface()
				extractSchemaFields(fullField+".$[]", value, fields)
			} else {
				fields[fullField] = "slice"
			}
		default:
			fields[fullField] = fieldValue.Kind().String()
		}
	}
}
