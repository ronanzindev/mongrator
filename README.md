# Mongrator

**Mongrator** is a Go package that automatically runs MongoDB schema migrations at application startup.

## Prerequisites

- Use `bson` tags in your struct fields to define schema mappings.
- Schemas must be defined as Go structs (not other types).

## Limitations

- **Pointer Support:** This package does not handle pointers in schemas. If your schema uses pointers, Mongrator might not work as expected.

## Installation

To install Mongrator, run the following command:

```bash
go get github.com/RonanzinDev/mongrator
```

### Usage
```golang
package main

import (
	"context"
	"time"

	"github.com/RonanzinDev/mongrator"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type User struct {
	Name     string    `json:"name" bson:"name"`
	Addrees  Addrees   `json:"address" bson:"address"`
	Contacts []Contact `json:"contacts" bson:"contacts,omitempty"`
}
type Addrees struct {
	Street string `json:"street" bson:"street"`
	Number int    `json:"number" bson:"number"`
}

type Contact struct {
	Email  string  `json:"email" bson:"email"`
	Number float64 `json:"number" bson:"number"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10) // Adjust the context as needed
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		panic(err)
	}
	db := client.Database("YOUR_DATABASE")
	migrator := mongrator.New(db)
	migrator.RegisterSchema("users", User{})
	migrator.RunMigrations()
}
```

### Options

You can define default type values:

```go
mongrator.DefaultBooleanType(true),
mongrator.DefaultIntType(1),
mongrator.DefaultInt32Type(2),
mongrator.DefaultIntType(3),
mongrator.DefaultFloat32Type(4.5),
mongrator.DefaultFloat64Type(5.5),
mongrator.DefaultStringType("completed"),
mongrator.DefaultTimeType(time.Now()),
```

### Contributing
If you encounter any issues, feel free to open an [Issue](https://github.com/RonanzinDev/mongrator/issues/new/choose) :)
