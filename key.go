package ioc

import (
	"fmt"
	"reflect"
)

// Key identifies a service of type T by name. Use it to register several
// services of the same type:
//
//	var Replica = ioc.Key[*sql.DB]("db.replica")
//
// Container's generic methods use the default key of T, DefaultKey[T]().
type Key[T any] string

// DefaultKey returns the key that Container's generic methods use for T: the
// name of the type, such as "*sql.DB".
func DefaultKey[T any]() Key[T] {
	return Key[T](reflect.TypeFor[T]().String())
}

// Provide registers build as the provider of the service under k. The service
// is built on first resolution and then reused. A build error is returned to
// the caller, and build runs again on the next resolution. Provide panics when
// k is already registered.
func (k Key[T]) Provide(c *Container, build func(*Container) (T, error)) {
	c.reg.add(string(k), &service{build: func(c *Container) (any, error) { return build(c) }})
}

// Supply registers value as the already built service under k. Supply panics
// when k is already registered.
func (k Key[T]) Supply(c *Container, value T) {
	c.reg.add(string(k), &service{
		build:   func(*Container) (any, error) { return value, nil },
		isBuilt: true,
		value:   value,
	})
	c.reg.mu.Lock()
	c.reg.order = append(c.reg.order, string(k))
	c.reg.mu.Unlock()
}

// Resolve returns the service under k, building it and its dependencies on
// first use. Errors wrap ErrNotFound, ErrCycle, ErrTypeMismatch or the
// provider's error.
func (k Key[T]) Resolve(c *Container) (T, error) {
	var zero T
	v, err := c.resolve(string(k))
	if err != nil {
		return zero, err
	}
	if v == nil {
		return zero, nil
	}
	typed, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("%w: service %q is %T, not %s", ErrTypeMismatch, string(k), v, reflect.TypeFor[T]())
	}
	return typed, nil
}

// MustResolve is like Resolve but panics on error. Use it where a missing
// service is a programming error, such as in main.
func (k Key[T]) MustResolve(c *Container) T {
	v, err := k.Resolve(c)
	if err != nil {
		panic(err)
	}
	return v
}
