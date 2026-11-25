package ioc

import (
	"errors"
	"testing"
)

func TestKeys(t *testing.T) {
	primary := Key[*config]("config.primary")
	replica := Key[*config]("config.replica")

	c := New()
	primary.Supply(c, &config{dsn: "primary"})
	replica.Provide(c, func(c *Container) (*config, error) {
		p, err := primary.Resolve(c)
		if err != nil {
			return nil, err
		}
		return &config{dsn: p.dsn + "-replica"}, nil
	})

	if got := replica.MustResolve(c).dsn; got != "primary-replica" {
		t.Errorf("replica = %q", got)
	}
	if _, err := c.Resolve[*config](); !errors.Is(err, ErrNotFound) {
		t.Errorf("default key must be separate from named keys: %v", err)
	}
	if got := DefaultKey[*config](); got != "*ioc.config" {
		t.Errorf("DefaultKey = %q", got)
	}
}
