<img align="right" src="https://images.luminyra.dev/luminyra/github/ioc/logo.webp" alt="An amber joint aligned with its matching indigo socket." width="96" height="96">

# ioc

[![Go Reference](https://pkg.go.dev/badge/github.com/luminyra/ioc.svg)](https://pkg.go.dev/github.com/luminyra/ioc)
[![CI](https://github.com/luminyra/ioc/actions/workflows/ci.yml/badge.svg)](https://github.com/luminyra/ioc/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/luminyra/ioc)](https://goreportcard.com/report/github.com/luminyra/ioc)
[![Go 1.27+](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/doc/devel/release)

ioc assembles your application for you. You tell it how to build each part, such as the config, the database pool, the repositories and the HTTP server, and it builds them in the right order, only when they are needed, and shuts them down in reverse order when the program exits.

Parts are looked up by their Go type, `c.Resolve[*sql.DB]()`, so asking for something that does not fit is a compile error rather than a runtime surprise.

- **Resolve by type.** Go 1.27 generic methods on the container; no reflection-based injection, no struct tags, no string lookups.
- **Typed keys for named parts.** `ioc.Key[*sql.DB]("db.replica")` names one of several values of the same type and still returns that type.
- **Ordered startup and shutdown.** Services are built lazily on first use and closed in reverse build order through `Close`, `Shutdown(ctx)` or `io.Closer`.
- **Safe by default.** Concurrent requests build a service once, and dependency cycles are reported with the full path instead of deadlocking.
- **Fast lookups.** Resolving a built service takes about 14 ns and does not allocate.

<p align="center">
  <img src="https://images.luminyra.dev/luminyra/github/ioc/cover.webp" alt="An amber joint and a blue block hover above matching sockets." width="640">
</p>

## Contents

- [When to use it](#when-to-use-it) · [Install](#install) · [Getting started](#getting-started)
- [Common tasks](#common-tasks) · [How it works](#how-it-works) · [Performance](#performance) · [API at a glance](#api-at-a-glance)

## When to use it

ioc pays off in services with many parts and a real lifecycle: connection pools, clients, background workers and servers that must start in dependency order and close cleanly.

For a small program, wiring constructors by hand in `main` is simpler and perfectly idiomatic; keep doing that until the wiring becomes a chore. If you prefer all wiring checked and generated at compile time, consider [google/wire](https://github.com/google/wire).

## Install

```sh
go get github.com/luminyra/ioc
```

ioc requires Go 1.27 or later and has no dependencies.

## Getting started

Copy the program into `main.go` and run it with `go run .`:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/luminyra/ioc"
)

type Config struct{ DSN string }

type DB struct{ dsn string }

// Close is called by the container when it shuts down.
func (db *DB) Close() error {
	fmt.Println("closing database")
	return nil
}

type UserService struct{ db *DB }

func newDB(c *ioc.Container) (*DB, error) {
	cfg, err := c.Resolve[*Config]()
	if err != nil {
		return nil, err
	}
	fmt.Println("connecting to", cfg.DSN)
	return &DB{dsn: cfg.DSN}, nil
}

func newUserService(c *ioc.Container) (*UserService, error) {
	db, err := c.Resolve[*DB]()
	if err != nil {
		return nil, err
	}
	return &UserService{db: db}, nil
}

func main() {
	c := ioc.New()
	c.Supply(&Config{DSN: "postgres://localhost/app"}) // a ready value
	c.Provide(newDB)                                    // built on first use
	c.Provide(newUserService)

	// Asking for the service builds it and everything it depends on.
	users := c.MustResolve[*UserService]()
	fmt.Println("user service ready:", users.db != nil)

	// Close shuts services down in reverse build order.
	if err := c.Close(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

```text
connecting to postgres://localhost/app
user service ready: true
closing database
```

A provider is an ordinary function `func(*ioc.Container) (T, error)`: it resolves what it needs from the container it receives and returns the service. `Provide` infers the service type from that return type.

<p align="center">
  <a href="https://images.luminyra.dev/luminyra/github/ioc/demo.webp"><img src="https://images.luminyra.dev/luminyra/github/ioc/demo.webp" alt="The first UserService lookup builds DB and then UserService. The second lookup reuses the instance. Close calls the database close method in this example." width="760"></a>
</p>

## Common tasks

### Register values and providers

```go
c.Supply(cfg)            // a value you already have
c.Provide(newDB)         // func(*ioc.Container) (*DB, error), built on first Resolve
c.Provide(newHTTPServer) // dependencies are resolved inside the provider
```

Registering the same type twice panics, because it is always a wiring mistake.

### Resolve services

```go
server, err := c.Resolve[*http.Server]() // returns an error if anything fails to build
logger := c.MustResolve[*slog.Logger]()  // panics instead; fine in main
```

Each service is built once per container and reused afterwards. If a provider returns an error, nothing is cached and the next `Resolve` tries again.

### Register several services of the same type

```go
var (
	Primary = ioc.Key[*sql.DB]("db.primary")
	Replica = ioc.Key[*sql.DB]("db.replica")
)

Primary.Provide(c, openPrimary)
Replica.Provide(c, openReplica)

replica, err := Replica.Resolve(c) // a *sql.DB, no type assertion
```

### Shut down cleanly

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := c.Close(ctx); err != nil {
	log.Printf("shutdown: %v", err) // every service is still attempted; errors are joined
}
```

A service is closed if it has `Close(ctx) error` (`ioc.Closer`), `Shutdown(ctx) error` like `*http.Server`, or `Close() error` (`io.Closer`). After `Close`, services are rebuilt on the next `Resolve`.

### Report health

```go
for name, err := range c.Check(ctx) { // services that implement ioc.Checker
	if err != nil {
		log.Printf("%s is unhealthy: %v", name, err)
	}
}
```

### Handle wiring errors

```go
_, err := c.Resolve[*UserService]()
switch {
case errors.Is(err, ioc.ErrNotFound):     // nothing registered for a type or key
case errors.Is(err, ioc.ErrCycle):        // a service depends on itself, e.g. "a -> b -> a"
case errors.Is(err, ioc.ErrTypeMismatch): // a key was registered with another type
}
```

### Swap parts in tests

Build a fresh container in each test and supply fakes where production code provides real services:

```go
c := ioc.New()
c.Supply[UserStore](fakeStore{}) // the interface your service resolves
c.Provide(newUserService)
```

## How it works

- `Provide` and `Supply` register under `DefaultKey[T]()`, the type's name such as `"*sql.DB"`; a `Key[T]` registers under its own name. Lookups are by key, and the stored value is checked against `T`.
- A provider receives a `*Container` that records the resolution path, which is how cycles are detected. Do not keep that value after the provider returns.
- A `Container` is safe for concurrent use; concurrent resolutions of the same service build it once. Resolving a cyclic graph from several goroutines at once may deadlock, so keep the graph acyclic.
- `Registered` and `Built` list service names, which helps when debugging wiring.

## Performance

Measured with the benchmarks in this repository (`make bench`; Go 1.27.4, linux/arm64, 5 CPUs, median of 6 runs). Run them on your hardware for numbers that matter to you.

| Operation | Time | Allocations |
| --- | --- | --- |
| `c.Resolve[*Config]()` on a built service | 14 ns | 0 |
| `key.Resolve(c)` on a built service | 11 ns | 0 |

Resolving is a map lookup under a lock, cheap enough to call per request. Resolving once at startup and passing the service along is still the clearest design.

## API at a glance

| Task | API |
| --- | --- |
| Create | `New` |
| Register | `Container.Provide`, `Container.Supply`, `Key.Provide`, `Key.Supply` |
| Resolve | `Container.Resolve`, `Container.MustResolve`, `Key.Resolve`, `Key.MustResolve`, `DefaultKey` |
| Lifecycle | `Container.Close`, `Container.Check`, `Closer`, `Checker` |
| Introspect | `Container.Registered`, `Container.Built` |
| Errors | `ErrNotFound`, `ErrCycle`, `ErrTypeMismatch` |

The full reference with runnable examples is on [pkg.go.dev](https://pkg.go.dev/github.com/luminyra/ioc). Complete programs live in [`examples/`](examples): [`basic`](examples/basic/main.go) and [`lifecycle`](examples/lifecycle/main.go).

## Versioning

ioc follows [semantic versioning](https://semver.org). Until v1.0.0, a minor release may change the API; every change is listed in the [changelog](CHANGELOG.md). It supports the two most recent Go releases, starting with Go 1.27.

## Contributing

Questions, bug reports and pull requests are welcome. See the [contributing guide](https://github.com/luminyra/.github/blob/main/CONTRIBUTING.md), and report security issues as described in the [security policy](https://github.com/luminyra/.github/blob/main/SECURITY.md).

ioc was inspired by [samber/do](https://github.com/samber/do) and [uber-go/dig](https://github.com/uber-go/dig). It is released under the [MIT License](LICENSE).
