package migrator

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"slices"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

const migratorCollectionName = "migrations"

type mongoDocument = map[string]interface{}
type migratorOptions func(*Migrator)
type migration struct {
	Collection string    `json:"collection" bson:"collection"`
	Migration  string    `json:"migration" bson:"migration"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
}
type Migrator struct {
	database         *mongo.Database
	collections      map[string]any
	createCollection bool
	saveMigration    bool
}

// Determines whether a collection should be created when registering a schema
func ShouldCreateCollection(v bool) migratorOptions {
	return func(m *Migrator) {
		m.createCollection = v
	}
}

// Enables or disables saving the migration log in the database
func ShouldSaveMigrations(v bool) migratorOptions {
	return func(m *Migrator) {
		m.saveMigration = v
	}
}

// Initialize the migrator
func New(database *mongo.Database, opts ...migratorOptions) *Migrator {
	migrate := &Migrator{
		database:    database,
		collections: make(map[string]any),
	}
	for _, opt := range opts {
		opt(migrate)
	}
	if migrate.saveMigration {
		collections, _ := migrate.database.ListCollectionNames(context.Background(), bson.M{})
		if !slices.Contains(collections, migratorCollectionName) {
			if err := migrate.database.CreateCollection(context.Background(), "migrations"); err != nil {
				log.Printf("Error to create 'migrations' collection. Err -> %s", err.Error())
			}
		}
	}
	return migrate

}

// Registers a schema and its corresponding collection for automatic migration during application startup.
func (m *Migrator) RegisterSchema(collection string, schema any) {
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
		if !m.createCollection {
			log.Printf("Collection '%s' does not exist", collection)
			return
		}
		err = m.database.CreateCollection(context.Background(), collection)
		if err != nil {
			log.Printf("Error create collection '%s', err -> %s", collection, err.Error())
			return
		}
	}
	m.collections[collection] = schema
}

func schemaToDocument(schema any) (mongoDocument, error) {
	documentSchema := make(mongoDocument)
	jsonBytes, err := bson.Marshal(schema)
	if err != nil {
		return documentSchema, err
	}
	err = bson.Unmarshal(jsonBytes, &documentSchema)
	if err != nil {
		return documentSchema, err
	}
	return documentSchema, nil
}

// Detects schema and collection changes, then applies the necessary migrations.
func (m *Migrator) RunMigrations() {
	var wg sync.WaitGroup
	for collection, schema := range m.collections {
		wg.Add(1)
		go func(collection string, schema any) {
			defer wg.Done()
			var document mongoDocument
			mapSchema, err := schemaToDocument(schema)
			if err != nil {
				log.Println(err)
				return
			}
			col := m.database.Collection(collection)
			if err := col.FindOne(context.Background(), bson.M{}, options.FindOne().SetSort(bson.M{"$natural": -1})).Decode(&document); err != nil {
				if err == mongo.ErrNoDocuments {
					return
				}
				log.Printf("Error fetching document from collection %s: %v\n", collection, err)
				return
			}
			fieldsToAdd := make(map[string]interface{})
			fieldsToRemove := make(map[string]interface{})
			fieldsToUpdateType := make(map[string]interface{})
			compareFields("", mapSchema, document, fieldsToAdd, fieldsToRemove, fieldsToUpdateType)
			m.addFields(fieldsToAdd, col)
			m.updateFieldsType(fieldsToUpdateType, col)
			m.removeFields(fieldsToRemove, col)
		}(collection, schema)
	}
	wg.Wait()
}

func (m *Migrator) saveMigrationLog(message string, collectionName, action string) {
	migration := migration{Collection: collectionName, Migration: message, CreatedAt: bson.Now()}
	if _, err := m.database.Collection(migratorCollectionName).InsertOne(context.Background(), migration); err != nil {
		log.Printf("Error saving migration log when %s\n", action)
	}
}
func (m *Migrator) removeFields(fieldsToRemove mongoDocument, collection *mongo.Collection) {
	collectionName := collection.Name()
	for field := range fieldsToRemove {
		_, err := collection.UpdateMany(context.Background(), bson.M{}, bson.M{"$unset": bson.M{field: ""}})
		if err != nil {
			log.Printf("Error removing field '%s' from collection '%s': %v\n", field, collectionName, err)
		} else {
			message := fmt.Sprintf("Field  '%s' removed from collection '%s'", field, collectionName)
			if m.saveMigration {
				m.saveMigrationLog(message, collectionName, "removing fields")
			}
			log.Printf("%s\n", message)
		}
	}
}

func (m *Migrator) addFields(fieldsToAdd mongoDocument, collection *mongo.Collection) {
	collectionName := collection.Name()
	for field, value := range fieldsToAdd {
		reflectType := reflect.TypeOf(value)
		var data any
		if reflectType.Kind() == reflect.Slice {
			data = []interface{}{}
		} else if reflectType.Kind() == reflect.Array {
			data = reflect.MakeSlice(reflectType, reflectType.Len(), reflectType.Len()+1).Interface()
		} else if reflectType.Kind() == reflect.Map {
			data = value
		} else {
			data = reflect.Zero(reflectType).Interface()
		}
		_, err := collection.UpdateMany(context.Background(), bson.M{}, bson.M{"$set": bson.M{field: data}})
		if err != nil {
			log.Printf("Error adding field '%s' to collection' %s': %v\n", field, collectionName, err)
		} else {
			message := fmt.Sprintf("Field '%s' added to collection '%s'", field, collectionName)
			if m.saveMigration {
				m.saveMigrationLog(message, collectionName, "adding fields")
			}
			log.Printf("%s\n", message)
		}
	}
}

func (m *Migrator) updateFieldsType(fieldsToUpdateType mongoDocument, collection *mongo.Collection) {
	collectionName := collection.Name()
	for field, value := range fieldsToUpdateType {
		reflectType := reflect.TypeOf(value)
		data := reflect.Zero(reflectType).Interface()
		_, err := collection.UpdateMany(context.Background(), bson.M{}, bson.M{"$set": bson.M{field: data}})
		if err != nil {
			log.Printf("Error updating type of field '%s' in collection '%s': %v\n", field, collectionName, err)
		} else {
			message := fmt.Sprintf("Field type of '%s' updated in collection '%s'\n", field, collectionName)
			if m.saveMigration {
				m.saveMigrationLog(message, collectionName, "updating fields type")
			}
			log.Printf("%s\n", message)
		}
	}
}
func compareFields(prefix string, schema, document, fieldsToAdd, fieldsToRemove, fieldsToUpdateType mongoDocument) {
	for field, schemaValue := range schema {
		if field == "id" || field == "_id" || schemaValue == nil {
			continue
		}
		fullField := field
		if prefix != "" {
			fullField = prefix + "." + field
		}
		docValue, exist := document[field]
		if !exist {
			fieldsToAdd[fullField] = schemaValue
			continue
		}
		if docValue == nil {
			docValue = schemaValue
		}
		if _, ok := docValue.(primitive.DateTime); ok {
			if _, ok := schemaValue.(time.Time); ok {
				docValue = schemaValue
			}
		}
		docReflectType := reflect.TypeOf(docValue)
		if docReflectType.Kind() == reflect.Int32 || docReflectType.Kind() == reflect.Int64 {
			docReflectType = reflect.TypeOf(schemaValue)
		}
		schemaReflectType := reflect.TypeOf(schemaValue)
		if schemaReflectType != docReflectType {
			fieldsToUpdateType[fullField] = schemaValue
			continue
		}

		// Check if the field is a struct
		if schemaMap, ok := schemaValue.(map[string]interface{}); ok {
			if docMap, ok := docValue.(map[string]interface{}); ok {
				compareFields(fullField, schemaMap, docMap, fieldsToAdd, fieldsToRemove, fieldsToUpdateType)
			} else {
				fieldsToUpdateType[fullField] = schemaValue
			}
			continue
		}
		// // Check if the fiels is an array
		if schemaArray, ok := schemaValue.([]interface{}); ok {
			if docArray, ok := docValue.([]interface{}); ok {
				compareFieldsArray(fullField, schemaArray, docArray, fieldsToAdd, fieldsToUpdateType)
			} else {
				fieldsToUpdateType[fullField] = schemaValue
			}
			continue
		}
	}
	// Check extra fields
	for field := range document {
		if field == "_id" || field == "id" {
			continue
		}
		fullField := field
		if prefix != "" {
			fullField = prefix + "." + field
		}
		if _, exists := schema[field]; !exists {
			fieldsToRemove[fullField] = struct{}{}
		}
	}
}

func compareFieldsArray(field string, schemaArray, docArray []interface{}, fieldsToAdd, fieldsToUpdateType map[string]interface{}) {
	if len(schemaArray) > 0 && len(docArray) > 0 {
		schemaElem := schemaArray[0]
		docElem := docArray[0]
		if reflect.TypeOf(schemaElem) != reflect.TypeOf(docElem) {
			fieldsToUpdateType[field] = schemaArray
			return
		}
		if schemaMap, ok := schemaElem.(map[string]interface{}); ok {
			if docMap, ok := docElem.(map[string]interface{}); ok {
				compareFields(field+"[]", schemaMap, docMap, fieldsToAdd, nil, fieldsToUpdateType)
			} else {
				fieldsToUpdateType[field] = schemaArray
			}
		}
	}
}
