// Command basic registers services in an ioc container, resolves them lazily
// by type with Go 1.27 generic methods, and closes them.
package main

import (
	"context"
	"fmt"

	"github.com/luminyra/ioc"
)

type Config struct {
	DSN string
}

type DB struct {
	dsn string
}

// Close lets the container close the connection on Close (the io.Closer form).
func (db *DB) Close() error {
	fmt.Println("closing database:", db.dsn)
	return nil
}

type UserRepo struct {
	db *DB
}

func main() {
	// region: provide
	c := ioc.New()

	// Ready values are supplied directly.
	c.Supply(&Config{DSN: "postgres://localhost/app"})

	// Other services are registered as providers. They are built on first use
	// and then reused; the *ioc.Container a provider receives resolves its own
	// dependencies.
	c.Provide(func(c *ioc.Container) (*DB, error) {
		cfg, err := c.Resolve[*Config]()
		if err != nil {
			return nil, err
		}
		fmt.Println("connecting to database:", cfg.DSN)
		return &DB{dsn: cfg.DSN}, nil
	})
	c.Provide(func(c *ioc.Container) (*UserRepo, error) {
		db, err := c.Resolve[*DB]()
		if err != nil {
			return nil, err
		}
		return &UserRepo{db: db}, nil
	})
	// endregion

	// region: resolve
	// Resolve is a generic method: the requested type goes in brackets.
	repo := c.MustResolve[*UserRepo]()
	again := c.MustResolve[*UserRepo]()
	fmt.Println("same instance:", repo == again)
	// endregion

	fmt.Println("registered:", c.Registered())
	fmt.Println("built:     ", c.Built())

	// region: close
	// Services are closed in reverse build order, through ioc.Closer,
	// Shutdown(ctx) or io.Closer.
	if err := c.Close(context.Background()); err != nil {
		fmt.Println("close failed:", err)
	}
	// endregion
}
