// Command lifecycle registers several services of one type with typed keys,
// checks their health, and shows the errors the container reports for cycles
// and type mismatches.
package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/luminyra/ioc"
)

type Cache struct {
	addr        string
	isConnected bool
}

// Check implements ioc.Checker, so the container's Check reports on it.
func (c *Cache) Check(context.Context) error {
	if !c.isConnected {
		return fmt.Errorf("%s: connection lost", c.addr)
	}
	return nil
}

// region: keys
// A Key carries its service type: Primary.Resolve returns a *Cache with no
// type assertion.
var (
	Primary = ioc.Key[*Cache]("cache.primary")
	Replica = ioc.Key[*Cache]("cache.replica")
)

// endregion

func main() {
	c := ioc.New()

	// region: supply
	Primary.Supply(c, &Cache{addr: "10.0.0.1:6379", isConnected: true})
	Replica.Supply(c, &Cache{addr: "10.0.0.2:6379", isConnected: false})
	// endregion

	// region: health
	health := c.Check(context.Background())
	for _, name := range slices.Sorted(maps.Keys(health)) {
		status := "ok"
		if err := health[name]; err != nil {
			status = err.Error()
		}
		fmt.Printf("%s: %s\n", name, status)
	}
	// endregion

	// region: errors
	// A dependency cycle is detected instead of deadlocking or overflowing
	// the stack.
	a, b := ioc.Key[string]("a"), ioc.Key[string]("b")
	a.Provide(c, b.Resolve)
	b.Provide(c, a.Resolve)

	_, err := a.Resolve(c)
	fmt.Println(errors.Is(err, ioc.ErrCycle), err)

	// Resolving a name with a key of another type reports a type mismatch.
	_, err = ioc.Key[string]("cache.primary").Resolve(c)
	fmt.Println(errors.Is(err, ioc.ErrTypeMismatch), err)
	// endregion
}
