package mongrator

import (
	"fmt"
	"testing"

	"github.com/iancoleman/orderedmap"
	"github.com/stretchr/testify/assert"
)

type user struct {
	Name     string   `bson:"name"`
	Age      int      `bson:"age"`
	Allowed  bool     `bson:"allowed"`
	Address  address  `bson:"address"`
	Cars     []string `bson:"cars"`
	TodoList []todo   `bson:"todo_list"`
}
type address struct {
	Number  int8   `bson:"number"`
	Neib    string `bson:"neib"`
	City    string `bson:"city"`
	Country string `bson:"country"`
}

type todo struct {
	Name        string   `bson:"name"`
	IsCompleted bool     `bson:"is_completed"`
	References  []string `bson:"references"`
}

func TestMongrator(t *testing.T) {
	t.Run("Testing extract fields and type", func(t *testing.T) {
		expected := map[string]string{
			"name":                       "string",
			"age":                        "int",
			"allowed":                    "bool",
			"address":                    "struct",
			"address.number":             "int8",
			"address.neib":               "string",
			"address.city":               "string",
			"address.country":            "string",
			"cars":                       "slice",
			"todo_list":                  "slice",
			"todo_list.$[].name":         "string",
			"todo_list.$[].is_completed": "bool",
			"todo_list.$[].references":   "slice",
		}
		fielsd := orderedmap.New()
		extractSchemaFields("", user{}, fielsd)
		for _, fieldKey := range fielsd.Keys() {
			fieldTypeValue, ok := fielsd.Get(fieldKey)
			assert.Equal(t, true, ok, fmt.Sprintf("failed to get field '%s'", fieldKey))
			fieldType := fieldTypeValue.(string)
			expectedType, ok := expected[fieldKey]
			assert.Equal(t, true, ok, fmt.Sprintf("field '%s' is not expected", fieldKey))
			assert.Equal(t, expectedType, fieldType, fmt.Sprintf("expec type '%s' got '%s'", expectedType, fieldType))
		}
	})
}
