package ioc_test

import (
	"context"
	"fmt"

	"github.com/luminyra/ioc"
)

type Config struct{ DSN string }

type DB struct{ dsn string }

func (db *DB) Close() error {
	fmt.Println("closing", db.dsn)
	return nil
}

func Example() {
	c := ioc.New()
	c.Supply(&Config{DSN: "postgres://localhost/app"})
	c.Provide(func(c *ioc.Container) (*DB, error) {
		cfg, err := c.Resolve[*Config]()
		if err != nil {
			return nil, err
		}
		return &DB{dsn: cfg.DSN}, nil
	})

	db := c.MustResolve[*DB]()
	fmt.Println("resolved", db.dsn)

	if err := c.Close(context.Background()); err != nil {
		fmt.Println(err)
	}
	// Output:
	// resolved postgres://localhost/app
	// closing postgres://localhost/app
}

func ExampleKey() {
	primary := ioc.Key[*Config]("config.primary")
	replica := ioc.Key[*Config]("config.replica")

	c := ioc.New()
	primary.Supply(c, &Config{DSN: "primary"})
	replica.Supply(c, &Config{DSN: "replica"})

	cfg, err := replica.Resolve(c) // cfg is a *Config, no type assertion
	fmt.Println(cfg.DSN, err)
	// Output: replica <nil>
}
