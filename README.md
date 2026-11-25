# ioc

[![Go Reference](https://pkg.go.dev/badge/github.com/luminyra/ioc.svg)](https://pkg.go.dev/github.com/luminyra/ioc)
[![CI](https://github.com/luminyra/ioc/actions/workflows/ci.yml/badge.svg)](https://github.com/luminyra/ioc/actions/workflows/ci.yml)

A type-safe dependency injection container for Go 1.27: services are resolved by type with generic methods, or by typed keys.

```go
c := ioc.New()
c.Supply(cfg)
c.Provide(func(c *ioc.Container) (*DB, error) {
	cfg, err := c.Resolve[*Config]()
	if err != nil {
		return nil, err
	}
	return OpenDB(cfg)
})

db := c.MustResolve[*DB]()
defer c.Close(context.Background())
```

## Install

```sh
go get github.com/luminyra/ioc
```

Requires Go 1.27 or later. The module has no dependencies.

## Highlights

- **Resolve by type.** `c.Resolve[*DB]()` is a Go 1.27 generic method: no reflection-based injection, no string names, no type assertions.
- **Typed keys for named services.** `ioc.Key[*DB]("db.replica")` names a service and carries its type, so a named lookup always returns that type.
- **Lazy singletons.** Providers run on first resolution and their results are reused. A failed build is retried on the next resolution.
- **Lifecycle.** `Close` releases built services in reverse build order through `Close(ctx)`, `Shutdown(ctx)` (as on `*http.Server`) or `io.Closer`; `Check` reports the health of services that implement `Checker`.
- **Safe by default.** Concurrent resolutions build a service once, and dependency cycles are reported as `ErrCycle` with the full path.

## Usage

```go
var (
	Primary = ioc.Key[*DB]("db.primary")
	Replica = ioc.Key[*DB]("db.replica")
)

Primary.Provide(c, openPrimary) // func(*ioc.Container) (*DB, error)
Replica.Provide(c, openReplica)

replica, err := Replica.Resolve(c) // replica is a *DB
if err != nil {
	return err
}

for name, err := range c.Check(ctx) {
	if err != nil {
		log.Printf("%s unhealthy: %v", name, err)
	}
}
```

## Examples

Runnable programs live in [`examples/`](examples):

| Example | Shows |
| --- | --- |
| [`basic`](examples/basic/main.go) | providers, lazy resolution, closing in reverse order |
| [`lifecycle`](examples/lifecycle/main.go) | typed keys, health checks, cycle and type errors |

```sh
go run ./examples/basic
```

## Design

ioc is inspired by [samber/do](https://github.com/samber/do) and [uber-go/dig](https://github.com/uber-go/dig). It targets Go 1.27 only: generic methods on the container and typed keys replace string-based lookups, so every resolution is checked at compile time.

## License

[MIT](LICENSE)
