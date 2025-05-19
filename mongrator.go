package mongrator

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/iancoleman/orderedmap"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const migratorCollectionName = "migrations"
const collectionFieldName = "migrations_collections_fields"

type (
	migration struct {
		Collection string    `json:"collection" bson:"collection"`
		Migration  string    `json:"migration" bson:"migration"`
		CreatedAt  time.Time `json:"created_at" bson:"created_at"`
	}
	Mongrator struct {
		database           *mongo.Database
		migrationCol       *mongo.Collection
		migrationFieldsCol *mongo.Collection
		collections        map[string]any
		config             *Config
	}
)

// Initialize the migrator
func New(database *mongo.Database, opts ...Option) *Mongrator {
	mongrator := &Mongrator{
		database:    database,
		collections: make(map[string]any),
	}
	config := defaultConfig()
	for _, opts := range opts {
		opts(config)
	}
	mongrator.config = defaultConfig()
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
func (m *Mongrator) RegisterSchema(collection string, schema any) {
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
	var savedFields *mongratorFields
	err = m.migrationFieldsCol.FindOne(context.Background(), bson.M{"collection": collection}).Decode(&savedFields)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			fields := orderedmap.New()
			extractSchemaFields("", schema, fields)
			savedFields = &mongratorFields{
				Collection: collection,
				Fields:     fields,
				CreatedAt:  time.Now(),
			}
			if _, err := m.migrationFieldsCol.InsertOne(context.Background(), savedFields); err != nil {
				log.Printf("Error saving fields from '%s' collection", collection)
				return
			}
		}
	}
	m.collections[collection] = schema
}

func (m *Mongrator) RunMigrations() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for collection, schema := range m.collections {
		wg.Add(1)
		go func(name string, schema any) {
			defer wg.Done()
			collectionFields := mongratorFields{}
			if err := m.migrationFieldsCol.FindOne(ctx, bson.M{"collection": collection}).Decode(&collectionFields); err != nil {
				log.Printf("Error to get fields from collection '%s', err -> %s", collection, err.Error())
				return
			}
			schemaFields := orderedmap.New()
			extractSchemaFields("", schema, schemaFields)
			updateFields := orderedmap.New()
			schemaCollection := m.database.Collection(collection)
			compareFields(collectionFields.Fields, schemaFields, updateFields)
			if len(updateFields.Values()) > 0 {
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
func (m *Mongrator) saveMigrationLog(message string, collection string, action string) {
	migration := migration{Collection: collection, Migration: message, CreatedAt: time.Now()}
	if _, err := m.migrationCol.InsertOne(context.Background(), migration); err != nil {
		log.Printf("Error saving migration log when %s\n", action)
	}
}

func (m *Mongrator) updateFields(fieldsToBeUpdate *orderedmap.OrderedMap, collection *mongo.Collection, collectionSchema *orderedmap.OrderedMap) {
	collectionName := collection.Name()
	for _, field := range fieldsToBeUpdate.Keys() {
		action := "added"
		fieldTypeValue, ok := fieldsToBeUpdate.Get(field)
		if !ok {
			continue
		}
		fieldType, ok := fieldTypeValue.(string)
		if !ok {
			continue
		}
		typeValue, ok := m.config.Types[fieldType]
		if !ok {
			continue
		}
		if _, ok := collectionSchema.Get(field); ok {
			action = "updated"
		}
		_, err := collection.UpdateMany(context.Background(), bson.M{}, bson.M{"$set": bson.M{field: typeValue}})
		if err != nil {
			log.Printf("Error %s field '%s' -> collection' %s': %v\n", action, field, collectionName, err)
			continue
		}
		message := fmt.Sprintf("Field '%s' %s with value '%v' -> collection '%s'", field, action, typeValue, collectionName)
		m.saveMigrationLog(message, collectionName, action+" fields")
		log.Printf("%s\n", message)

	}

}
func (m *Mongrator) updateMigrationFields(collection string, fieldsToUpdate, collectionFields *orderedmap.OrderedMap) {
	for _, field := range fieldsToUpdate.Keys() {
		value, _ := fieldsToUpdate.Get(field)
		collectionFields.Set(field, value)
	}
	if _, err := m.migrationFieldsCol.UpdateOne(context.Background(), bson.M{"collection": collection}, bson.M{"$set": bson.M{"fields": collectionFields.Values()}}); err != nil {
		log.Printf("Error updating migration fields from collection : '%s'", collection)
	}

}
func (m *Mongrator) removeFields(fielsToRemove []string, collection *mongo.Collection) {
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
func (m *Mongrator) removeFieldsFromMongratorCollection(fielsToRemove []string, collection string, collectionSchema *orderedmap.OrderedMap) {
	value := collectionSchema
	for _, field := range fielsToRemove {
		value.Delete(field)
	}
	if _, err := m.migrationFieldsCol.UpdateOne(context.Background(), bson.M{"collection": collection}, bson.M{"$set": bson.M{"fields": value.Values()}}); err != nil {
		log.Printf("Error to remove field from mongrator field collections")
	}
}
