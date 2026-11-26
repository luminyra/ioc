// Package ioc is a type-safe dependency injection container for Go 1.27.
//
// # Resolving by type
//
// A service is addressed by its type through the generic methods of
// [Container]: [Container.Provide] registers a lazily built service,
// [Container.Supply] registers a ready value, and [Container.Resolve] returns
// it, building its dependencies first. A provider receives the Container to
// resolve its own dependencies:
//
//	c := ioc.New()
//	c.Supply(cfg)
//	c.Provide(func(c *ioc.Container) (*DB, error) {
//		cfg, err := c.Resolve[*Config]()
//		if err != nil {
//			return nil, err
//		}
//		return OpenDB(cfg)
//	})
//	db := c.MustResolve[*DB]()
//
// # Several services of one type
//
// A [Key] names a service and carries its type, so a named lookup always
// returns the type it was declared with:
//
//	var Replica = ioc.Key[*DB]("db.replica")
//
//	Replica.Provide(c, openReplica)
//	replica, err := Replica.Resolve(c)
//
// # Lifecycle
//
// Each service is a singleton within its Container and is built on first
// resolution. [Container.Close] closes the built services in reverse build
// order, through [Closer], a Shutdown(context.Context) error method such as
// [net/http.Server.Shutdown], or [io.Closer], so a service is closed before
// the services it depends on. [Container.Check] reports the health of the
// services that implement [Checker].
//
// # Concurrency
//
// A Container is safe for concurrent use, and concurrent resolutions of a
// service build it once. A dependency cycle is reported with [ErrCycle], both
// when a resolution chain revisits a service and when chains resolving
// concurrently would wait for each other's services: a chain checks for such
// a cycle before it waits, instead of deadlocking.
//
// The package is inspired by samber/do and uber-go/dig.
package ioc
