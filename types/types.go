package types

import "gopkg.in/mgo.v2/bson"

var Types = map[string]any{
	"string":  "",
	"int":     int(0),
	"int32":   int32(0),
	"int64":   int64(0),
	"float32": float32(0.0),
	"float64": float64(0.0),
	"slice":   make([]any, 0),
	"bool":    false,
	"time":    bson.Now(),
}
