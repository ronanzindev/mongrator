package model

import "time"

type User struct {
	Name      string    `bson:"name"`
	Age       int       `bson:"age"`
	Allowed   bool      `bson:"allowed"`
	Address   Address   `bson:"address"`
	Cars      struct{}  `bson:"cars"`
	Todos     []Todo    `bson:"todos"`
	CreatedAt time.Time `bson:"created_at"`
}
type Address struct {
	Number  int8   `bson:"number"`
	Neib    string `bson:"neib"`
	City    string `bson:"city"`
	Country string `bson:"country"`
}

type Todo struct {
	Name        string   `bson:"name"`
	IsCompleted bool     `bson:"is_completed"`
	References  struct{} `bson:"references"`
	Point       []int    `bson:"point"`
}
