package mongrator

import (
	"time"

	"github.com/RonanzinDev/mongrator/utils"
)

type Config struct {
	Types map[string]any
}
type Option func(*Config)

func DefaultStringType(defaultValue string) Option {
	return func(c *Config) {
		c.Types["string"] = defaultValue
	}
}
func DefaultIntType(defaultValue int) Option {
	return func(c *Config) {
		c.Types["int"] = defaultValue
	}
}
func DefaultInt32Type(defaultValue int32) Option {
	return func(c *Config) {
		c.Types["int32"] = defaultValue
	}
}
func DefaultInt64Type(defaultValue int64) Option {
	return func(c *Config) {
		c.Types["int64"] = defaultValue
	}
}
func DefaultFloat32Type(defaultValue float32) Option {
	return func(c *Config) {
		c.Types["float32"] = defaultValue
	}
}
func DefaultFloat64Type(defaultValue float64) Option {
	return func(c *Config) {
		c.Types["float64"] = defaultValue
	}
}

func DefaultBooleanType(defaultValue bool) Option {
	return func(c *Config) {
		c.Types["bool"] = defaultValue
	}
}
func DefaultTimeType(defaultValue time.Time) Option {
	return func(c *Config) {
		c.Types["time"] = defaultValue
	}
}

func defaultConfig() *Config {
	return &Config{
		Types: utils.DefaultTypesValues,
	}
}
