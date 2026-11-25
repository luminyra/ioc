package ioc

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
)

type config struct{ dsn string }

type db struct {
	cfg      *config
	log      *[]string
	isClosed bool
}

// Close implements Closer.
func (d *db) Close(context.Context) error {
	*d.log = append(*d.log, "db")
	d.isClosed = true
	return nil
}

// Check implements Checker.
func (d *db) Check(context.Context) error {
	if d.isClosed {
		return errors.New("closed")
	}
	return nil
}

type repo struct {
	db  *db
	log *[]string
}

// Close implements io.Closer.
func (r *repo) Close() error {
	*r.log = append(*r.log, "repo")
	return errors.New("repo close failed")
}

type server struct{ log *[]string }

// Shutdown has the shape of (*http.Server).Shutdown.
func (s *server) Shutdown(context.Context) error {
	*s.log = append(*s.log, "server")
	return nil
}

var (
	_ Checker = (*db)(nil)
	_ Closer  = (*db)(nil)
	_ Closer  = (*Container)(nil)
)

func newGraph(log *[]string) *Container {
	c := New()
	c.Supply(&config{dsn: "mem"})
	c.Provide(func(c *Container) (*db, error) {
		cfg, err := c.Resolve[*config]()
		if err != nil {
			return nil, err
		}
		return &db{cfg: cfg, log: log}, nil
	})
	c.Provide(func(c *Container) (*repo, error) {
		d, err := c.Resolve[*db]()
		if err != nil {
			return nil, err
		}
		return &repo{db: d, log: log}, nil
	})
	c.Provide(func(c *Container) (*server, error) {
		if _, err := c.Resolve[*repo](); err != nil {
			return nil, err
		}
		return &server{log: log}, nil
	})
	return c
}

func TestResolveBuildsLazilyOnce(t *testing.T) {
	var log []string
	c := newGraph(&log)
	if got := c.Built(); !slices.Equal(got, []string{"*ioc.config"}) {
		t.Fatalf("before resolve = %v", got)
	}
	r1 := c.MustResolve[*repo]()
	r2 := c.MustResolve[*repo]()
	if r1 != r2 || r1.db.cfg.dsn != "mem" {
		t.Fatal("services are not singletons")
	}
	if got := c.Built(); !slices.Equal(got, []string{"*ioc.config", "*ioc.db", "*ioc.repo"}) {
		t.Fatalf("build order = %v", got)
	}
	if got := c.Registered(); !slices.Equal(got, []string{"*ioc.config", "*ioc.db", "*ioc.repo", "*ioc.server"}) {
		t.Fatalf("registered = %v", got)
	}
}

func TestCloseReverseOrderAndReset(t *testing.T) {
	var log []string
	c := newGraph(&log)
	c.MustResolve[*server]()
	first := c.MustResolve[*repo]()

	if health := c.Check(t.Context()); len(health) != 1 || health["*ioc.db"] != nil {
		t.Fatalf("Check = %v", health)
	}
	err := c.Close(t.Context())
	if !slices.Equal(log, []string{"server", "repo", "db"}) {
		t.Fatalf("close order = %v", log)
	}
	if err == nil || err.Error() != `ioc: close "*ioc.repo": repo close failed` {
		t.Fatalf("Close error = %v", err)
	}
	if second := c.MustResolve[*repo](); second == first {
		t.Fatal("service was not rebuilt after Close")
	}
	if c.MustResolve[*config]().dsn != "mem" {
		t.Fatal("supplied value lost after Close")
	}
}

func TestErrors(t *testing.T) {
	c := New()
	if _, err := c.Resolve[*db](); !errors.Is(err, ErrNotFound) {
		t.Errorf("not found error = %v", err)
	}

	Key[int]("port").Supply(c, 8080)
	if _, err := Key[string]("port").Resolve(c); !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("type mismatch error = %v", err)
	}

	boom := errors.New("boom")
	var attempts atomic.Int32
	flaky := Key[int]("flaky")
	flaky.Provide(c, func(*Container) (int, error) {
		if attempts.Add(1) == 1 {
			return 0, boom
		}
		return 1, nil
	})
	if _, err := flaky.Resolve(c); !errors.Is(err, boom) || err.Error() != `ioc: build "flaky": boom` {
		t.Errorf("provider error = %v", err)
	}
	if v, err := flaky.Resolve(c); v != 1 || err != nil {
		t.Errorf("retry after provider error = %d, %v", v, err)
	}

	Key[error]("nil-interface").Supply(c, nil)
	if v, err := Key[error]("nil-interface").Resolve(c); v != nil || err != nil {
		t.Errorf("nil service = %v, %v", v, err)
	}
}

func TestMustResolvePanics(t *testing.T) {
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, ErrNotFound) {
			t.Fatalf("MustResolve panic = %v", err)
		}
	}()
	New().MustResolve[*db]()
}

func TestCycle(t *testing.T) {
	a, b := Key[int]("a"), Key[int]("b")
	c := New()
	a.Provide(c, b.Resolve)
	b.Provide(c, a.Resolve)

	_, err := a.Resolve(c)
	if !errors.Is(err, ErrCycle) || err.Error() != "ioc: dependency cycle: a -> b -> a" {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestDuplicateRegistrationPanics(t *testing.T) {
	c := New()
	c.Supply(1)
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate Supply did not panic")
		}
	}()
	c.Provide(func(*Container) (int, error) { return 2, nil })
}

func TestConcurrentResolve(t *testing.T) {
	c := New()
	var builds atomic.Int32
	c.Provide(func(*Container) (*config, error) {
		builds.Add(1)
		return &config{}, nil
	})
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { c.MustResolve[*config]() })
	}
	wg.Wait()
	if builds.Load() != 1 {
		t.Fatalf("builds = %d, want 1", builds.Load())
	}
}
