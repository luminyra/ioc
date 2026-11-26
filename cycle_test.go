package ioc

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// resolveTogether resolves keys concurrently, one goroutine each, after every
// provider has started, so that each goroutine holds its own service while it
// resolves the next one.
func resolveTogether(t *testing.T, names ...string) []error {
	t.Helper()
	c := New()
	var started sync.WaitGroup
	started.Add(len(names))
	for i, name := range names {
		next := Key[int](names[(i+1)%len(names)])
		var first sync.Once // a provider runs again after a failed build
		Key[int](name).Provide(c, func(c *Container) (int, error) {
			first.Do(func() {
				started.Done()
				started.Wait()
			})
			return next.Resolve(c)
		})
	}
	errs := make([]error, len(names))
	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i, name := range names {
			wg.Go(func() { _, errs[i] = Key[int](name).Resolve(c) })
		}
		wg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent resolutions of a cycle deadlocked")
	}
	return errs
}

func TestCycleAcrossConcurrentResolutions(t *testing.T) {
	for _, names := range [][]string{{"a", "b"}, {"a", "b", "c"}} {
		t.Run(strings.Join(names, ""), func(t *testing.T) {
			for range 50 {
				for i, err := range resolveTogether(t, names...) {
					if !errors.Is(err, ErrCycle) {
						t.Fatalf("resolving %s: %v, want ErrCycle", names[i], err)
					}
				}
			}
		})
	}
}

func TestCrossChainCycleMessage(t *testing.T) {
	// The chain that detects the cycle names it from its own path; the other
	// chain then finds the same cycle on its own path.
	want := map[string]bool{"a -> b -> a": true, "b -> a -> b": true}
	for _, err := range resolveTogether(t, "a", "b") {
		_, path, _ := strings.Cut(err.Error(), ErrCycle.Error()+": ")
		if !want[path] {
			t.Fatalf("cycle error %q, want one of %v", err, want)
		}
	}
}

func TestConcurrentSharedDependenciesAreNotCycles(t *testing.T) {
	// A diamond resolved from every node at once, many times: chains wait
	// for each other, but never in a cycle.
	for range 200 {
		c := New()
		leaf := Key[int]("leaf")
		leaf.Provide(c, func(*Container) (int, error) {
			time.Sleep(time.Microsecond)
			return 1, nil
		})
		mids := []Key[int]{"left", "right"}
		for _, mid := range mids {
			mid.Provide(c, func(c *Container) (int, error) { return leaf.Resolve(c) })
		}
		top := Key[int]("top")
		top.Provide(c, func(c *Container) (int, error) {
			l, err := mids[0].Resolve(c)
			if err != nil {
				return 0, err
			}
			r, err := mids[1].Resolve(c)
			return l + r, err
		})
		var wg sync.WaitGroup
		for _, k := range []Key[int]{top, mids[1], mids[0], leaf, top} {
			wg.Go(func() {
				if _, err := k.Resolve(c); err != nil {
					t.Error(fmt.Errorf("resolving %s: %w", string(k), err))
				}
			})
		}
		wg.Wait()
	}
}
