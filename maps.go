package mongrator

import (
	"errors"
	"time"

	"github.com/iancoleman/orderedmap"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type orderedMap = *orderedmap.OrderedMap
type mongratorFields struct {
	Collection string     `bson:"collection"`
	Fields     orderedMap `bson:"fields"`
	CreatedAt  time.Time  `bson:"created_at"`
}

func (m *mongratorFields) UnmarshalBSON(data []byte) error {
	var raw bson.M
	if err := bson.Unmarshal(data, &raw); err != nil {
		return err
	}
	collection, ok := raw["collection"].(string)
	if !ok {
		return errors.New("collection field not found")
	}
	m.Collection = collection
	fields := orderedmap.New()
	if rawFields, ok := raw["fields"].(bson.D); ok {
		for _, v := range rawFields {
			fields.Set(v.Key, v.Value)
		}
	}
	m.Fields = fields
	m.Fields.Sort(func(a, b *orderedmap.Pair) bool {
		return len(a.Key()) < len(b.Key())
	})
	createdAt, ok := raw["created_at"].(bson.DateTime)
	if !ok {
		return errors.New("error parsing created_at field")
	}
	m.CreatedAt = createdAt.Time()
	return nil
}

func (o *mongratorFields) MarshalBSON() ([]byte, error) {
	if o == nil {
		o = &mongratorFields{Fields: orderedmap.New()}
	}
	raw := bson.M{}
	raw["collection"] = o.Collection
	fields := bson.M{}
	for _, key := range o.Fields.Keys() {
		value, ok := o.Fields.Get(key)
		if !ok {
			continue
		}
		fields[key] = value
	}
	raw["fields"] = fields
	raw["created_at"] = o.CreatedAt
	bytes, err := bson.Marshal(raw)
	return bytes, err
}
