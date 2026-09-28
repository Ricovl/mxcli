// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#765: `refresh catalog full source` describes every document from
// a pool of goroutines that share one ExecContext.Cache. describeEntity's
// findModule filled the module list lazily with no synchronisation, so one
// worker published its slice while another read the field.

// TestParallelEntityDescribes_NoDataRace runs the catalog's describe path —
// preWarmCache, then captureDescribeParallel from many goroutines on one
// shared cache — over every entity of the Studio Pro-authored PedApp fixture.
// It only fails under `go test -race`, where it reported the two races of the
// issue in getModulesFromCache before the fix.
//
// It drives entities only rather than the whole `refresh catalog full source`:
// page, workflow and microflow describes parse expressions, and concurrent
// ANTLR parsers race inside the runtime's shared DFA cache — a separate
// defect that would make this test flaky without saying anything about the
// module cache.
func TestParallelEntityDescribes_NoDataRace(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	ctx := exec.newExecContext(t.Context())
	preWarmCache(ctx)

	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, dm := range dms {
		for _, e := range dm.Entities {
			names = append(names, h.GetModuleName(dm.ContainerID)+"."+e.Name)
		}
	}
	if len(names) < 4 {
		t.Fatalf("PedApp has %d entities; the test needs several to run in parallel", len(names))
	}

	var wg sync.WaitGroup
	errs := make([]error, len(names))
	for i, name := range names {
		wg.Go(func() {
			_, errs[i] = captureDescribeParallel(ctx, catalog.SourceEntity, name, "")
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("describe entity %s: %v", names[i], err)
		}
	}
}

// TestModuleCache_ConcurrentFillListsOnce is the detector-free half: workers
// that find the module cache empty at the same moment must not each list and
// publish the modules. Unguarded, every worker that passed the nil check
// before the first one stored its result called ListModules itself.
func TestModuleCache_ConcurrentFillListsOnce(t *testing.T) {
	var calls atomic.Int32
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			calls.Add(1)
			time.Sleep(20 * time.Millisecond) // hold the window open
			return []*model.Module{{BaseElement: model.BaseElement{ID: "m1"}, Name: "Shop"}}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withCache(&executorCache{}))

	const workers = 8
	var start, done sync.WaitGroup
	start.Add(1)
	errs := make([]error, workers)
	for i := range workers {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			_, errs[i] = findModule(ctx, "Shop")
		}()
	}
	start.Done()
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("worker %d: findModule: %v", i, err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("ListModules called %d times by %d concurrent lookups on one cache, want 1", n, workers)
	}
}
