package ioc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
)

var (
	// ErrNotFound reports that no service is registered under a key.
	ErrNotFound = errors.New("ioc: service not found")

	// ErrCycle reports that building a service requires itself, on one
	// resolution chain or across chains resolving concurrently.
	ErrCycle = errors.New("ioc: dependency cycle")

	// ErrTypeMismatch reports that a service is not of the requested type.
	ErrTypeMismatch = errors.New("ioc: type mismatch")
)

// Checker is implemented by services that report their health on
// [Container.Check].
type Checker interface {
	Check(ctx context.Context) error
}

// Closer is implemented by services that release resources on
// [Container.Close]. Services with Shutdown([context.Context]) error, such as
// *[net/http.Server], or with [io.Closer]'s Close() error are closed as well.
type Closer interface {
	Close(ctx context.Context) error
}

type shutdowner interface {
	Shutdown(ctx context.Context) error
}

// Container holds services and builds them lazily. Create it with New. A
// provider receives a *Container to resolve its own dependencies; that value
// tracks the resolution path for cycle detection and must not be kept after
// the provider returns.
type Container struct {
	reg   *registry
	path  []string // names being built on this resolution chain
	chain *chain   // nil until the chain builds or waits for a service
}

type registry struct {
	mu       sync.Mutex
	services map[string]*service
	order    []string // names in build completion order
}

type service struct {
	name    string
	mu      sync.Mutex // held while the service is checked or built
	build   func(*Container) (any, error)
	isBuilt bool
	value   any
	owner   *chain // the chain building the service, protected by registry.mu
}

// chain is one resolution chain: a root Resolve call and the providers it
// runs. Chains that resolve concurrently can wait for services that other
// chains are building; the registry keeps these waits acyclic.
type chain struct {
	waitingFor *service // protected by registry.mu
}

// New returns an empty Container.
func New() *Container {
	return &Container{reg: &registry{services: make(map[string]*service)}}
}

// Provide registers build as the provider of the default service of type T.
// See [Key.Provide].
func (c *Container) Provide[T any](build func(*Container) (T, error)) {
	DefaultKey[T]().Provide(c, build)
}

// Supply registers value as the default service of type T. See [Key.Supply].
func (c *Container) Supply[T any](value T) {
	DefaultKey[T]().Supply(c, value)
}

// Resolve returns the default service of type T, as in c.Resolve[*sql.DB]().
// See [Key.Resolve].
func (c *Container) Resolve[T any]() (T, error) {
	return DefaultKey[T]().Resolve(c)
}

// MustResolve is like Resolve but panics on error.
func (c *Container) MustResolve[T any]() T {
	return DefaultKey[T]().MustResolve(c)
}

func (r *registry) add(name string, s *service) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.services[name]; ok {
		panic(fmt.Sprintf("ioc: service %q already registered", name))
	}
	s.name = name
	r.services[name] = s
}

// wait records that ch is about to wait for the lock of s. If the chain
// building s is itself waiting, directly or through other chains, for a
// service that ch is building, the chains would wait for each other forever;
// wait reports that cycle as ErrCycle instead. path is the resolution path of
// ch.
func (r *registry) wait(ch *chain, s *service, path []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.waitsFor(s, ch) {
		names := append(slices.Clip(path), s.name)
		for owner := s.owner; owner != ch; owner = owner.waitingFor.owner {
			names = append(names, owner.waitingFor.name)
		}
		return fmt.Errorf("%w: %s", ErrCycle, strings.Join(names, " -> "))
	}
	ch.waitingFor = s
	return nil
}

// waitsFor reports whether the chain building s is ch, or waits for it
// through the chains building the services it waits for. Every wait goes
// through wait, which refuses to close a cycle, so the walk ends; it is
// bounded by the number of services all the same. r.mu must be held.
func (r *registry) waitsFor(s *service, ch *chain) bool {
	owner := s.owner
	for range len(r.services) + 1 {
		switch {
		case owner == ch:
			return true
		case owner == nil || owner.waitingFor == nil:
			return false
		}
		owner = owner.waitingFor.owner
	}
	return false
}

// claim records that ch holds the lock it waited for, and that it builds s
// unless s is nil.
func (r *registry) claim(ch *chain, s *service) {
	r.mu.Lock()
	ch.waitingFor = nil
	if s != nil {
		s.owner = ch
	}
	r.mu.Unlock()
}

// release records that s is no longer being built.
func (r *registry) release(s *service) {
	r.mu.Lock()
	s.owner = nil
	r.mu.Unlock()
}

func (r *registry) lookup(name string) *service {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.services[name]
}

func (c *Container) resolve(name string) (any, error) {
	if slices.Contains(c.path, name) {
		return nil, fmt.Errorf("%w: %s -> %s", ErrCycle, strings.Join(c.path, " -> "), name)
	}
	s := c.reg.lookup(name)
	if s == nil {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
	}

	// The lock is free unless another chain is checking or building s, so
	// resolving a built service never touches the waits between chains.
	ch, waited := c.chain, false
	if !s.mu.TryLock() {
		if ch == nil {
			ch = &chain{}
		}
		if err := c.reg.wait(ch, s, c.path); err != nil {
			return nil, err
		}
		s.mu.Lock()
		waited = true
	}
	defer s.mu.Unlock()
	if s.isBuilt {
		if waited {
			c.reg.claim(ch, nil)
		}
		return s.value, nil
	}
	if ch == nil {
		ch = &chain{}
	}
	c.reg.claim(ch, s)
	defer c.reg.release(s)
	v, err := s.build(&Container{reg: c.reg, path: append(slices.Clip(c.path), name), chain: ch})
	if err != nil {
		if errors.Is(err, ErrCycle) {
			return nil, err
		}
		return nil, fmt.Errorf("ioc: build %q: %w", name, err)
	}
	s.value, s.isBuilt = v, true
	c.reg.mu.Lock()
	c.reg.order = append(c.reg.order, name)
	c.reg.mu.Unlock()
	return v, nil
}

// Registered returns the names of all registered services, sorted.
func (c *Container) Registered() []string {
	c.reg.mu.Lock()
	defer c.reg.mu.Unlock()
	return slices.Sorted(maps.Keys(c.reg.services))
}

// Built returns the names of the built services in build order.
func (c *Container) Built() []string {
	c.reg.mu.Lock()
	defer c.reg.mu.Unlock()
	return slices.Clone(c.reg.order)
}

// Check calls Check on every built service that implements Checker and
// returns the results by service name.
func (c *Container) Check(ctx context.Context) map[string]error {
	result := make(map[string]error)
	for name, v := range c.builtServices() {
		if checker, ok := v.(Checker); ok {
			result[name] = checker.Check(ctx)
		}
	}
	return result
}

// Close releases the built services in reverse build order, so a service is
// closed before the services it depends on. Every service is then reset, so
// a later Resolve builds it again. Errors are joined and wrapped with the
// service name. Close makes a Container itself a Closer.
func (c *Container) Close(ctx context.Context) error {
	c.reg.mu.Lock()
	order := c.reg.order
	c.reg.order = nil
	c.reg.mu.Unlock()

	var errs []error
	for _, name := range slices.Backward(order) {
		s := c.reg.lookup(name)
		s.mu.Lock()
		v := s.value
		s.value, s.isBuilt = nil, false
		s.mu.Unlock()

		if err := closeService(ctx, v); err != nil {
			errs = append(errs, fmt.Errorf("ioc: close %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

func closeService(ctx context.Context, v any) error {
	switch svc := v.(type) {
	case Closer:
		return svc.Close(ctx)
	case shutdowner:
		return svc.Shutdown(ctx)
	case io.Closer:
		return svc.Close()
	}
	return nil
}

// builtServices returns a snapshot of the built services by name.
func (c *Container) builtServices() map[string]any {
	names := c.Built()
	result := make(map[string]any, len(names))
	for _, name := range names {
		s := c.reg.lookup(name)
		s.mu.Lock()
		if s.isBuilt {
			result[name] = s.value
		}
		s.mu.Unlock()
	}
	return result
}
