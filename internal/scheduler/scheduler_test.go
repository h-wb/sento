package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/store"
)

func testScheduler(t *testing.T, st *store.Store, tasks ...*task) *Scheduler {
	t.Helper()
	return &Scheduler{store: st, tasks: tasks, startupGrace: 10 * time.Millisecond}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	conn, dialect := dbtest.Open(t)
	return store.New(conn, dialect)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func status(s *Scheduler, slug string) TaskStatus {
	for _, ts := range s.Tasks() {
		if ts.Slug == slug {
			return ts
		}
	}
	return TaskStatus{}
}

func TestFirstDelayResumesTheCadence(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	s := &Scheduler{startupGrace: 30 * time.Second}
	for name, tc := range map[string]struct {
		lastRun time.Time
		want    time.Duration
	}{
		"never ran: a full interval":       {time.Time{}, 12 * time.Hour},
		"ran 11h ago: due in 1h":           {now.Add(-11 * time.Hour), time.Hour},
		"overdue: after the grace period":  {now.Add(-3 * 24 * time.Hour), 30 * time.Second},
		"just ran: almost a full interval": {now.Add(-time.Minute), 12*time.Hour - time.Minute},
		"clock went backwards: capped":     {now.Add(48 * time.Hour), 12 * time.Hour},
	} {
		task := &task{interval: 12 * time.Hour, lastRun: tc.lastRun}
		if got := s.firstDelay(task, now); got != tc.want {
			t.Errorf("%s: firstDelay = %v, want %v", name, got, tc.want)
		}
	}
}

// The reason last runs are persisted: an instance restarted more often
// than a task's interval used to never run that task.
func TestOverdueTaskRunsSoonAfterARestart(t *testing.T) {
	st := openStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First life: the sweep runs once (manually), then the process "dies".
	var runs atomic.Int32
	tick := func(context.Context) error { runs.Add(1); return nil }
	first := testScheduler(t, st, &task{slug: "volume-sweep", interval: time.Hour, tick: tick})
	if err := first.Run("volume-sweep"); err != nil {
		t.Fatal(err)
	}
	// The in-memory status is updated before the asynchronous save. Wait
	// for persistence so that save cannot overwrite the simulated past below.
	waitFor(t, "the manual run to persist", func() bool {
		return !first.loadState(ctx)["volume-sweep"].IsZero()
	})

	// Second life, with a clock far enough ahead that the sweep is overdue:
	// rewrite the persisted run into the past.
	if err := st.PutSetting(ctx, stateKey, []byte(`{"volume-sweep":"2020-01-01T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	second := testScheduler(t, st, &task{slug: "volume-sweep", interval: time.Hour, tick: tick})
	second.Start(ctx)
	if got := status(second, "volume-sweep").LastRun; got != "2020-01-01T00:00:00Z" {
		t.Fatalf("restored lastRun = %q, want the persisted one", got)
	}
	waitFor(t, "the overdue run", func() bool { return runs.Load() == 2 })

	// And a never-run task still waits out its interval.
	never := testScheduler(t, st, &task{slug: "other", interval: time.Hour, tick: tick})
	never.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	if runs.Load() != 2 {
		t.Fatalf("runs = %d: a task that never ran must not fire at startup", runs.Load())
	}
}

func TestRunPersistsAcrossSchedulers(t *testing.T) {
	st := openStore(t)
	a := testScheduler(t, st,
		&task{slug: "a", interval: time.Hour, tick: func(context.Context) error { return nil }},
		&task{slug: "b", interval: time.Hour, tick: func(context.Context) error { return errors.New("boom") }})
	for _, slug := range []string{"a", "b"} {
		if err := a.Run(slug); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "both runs", func() bool {
		return status(a, "a").LastRun != "" && status(a, "b").LastError == "boom"
	})
	waitFor(t, "both runs persisted", func() bool { return len(a.loadState(context.Background())) == 2 })
	if err := a.Run("nope"); err == nil {
		t.Fatal("want an error for an unknown task")
	}
}

// A manual run asked for while the task is mid-tick runs once that tick
// ends: it may carry a request (a queued library organize) the running
// tick did not see.
func TestManualRunDuringTickRunsAfterIt(t *testing.T) {
	release := make(chan struct{})
	var runs atomic.Int32
	s := testScheduler(t, nil, &task{slug: "slow", tick: func(context.Context) error {
		if runs.Add(1) == 1 {
			<-release
		}
		return nil
	}})
	if err := s.Run("slow"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the first tick", func() bool { return runs.Load() == 1 })
	// Asked for in this goroutine, so all three are held before the tick is
	// let go: through Run they would start on their own, and one arriving
	// after the follow-up run had ended would start a third.
	for range 3 {
		s.runTask(context.Background(), s.tasks[0], true)
	}
	s.tasks[0].mu.Lock()
	held := s.tasks[0].again
	s.tasks[0].mu.Unlock()
	if !held {
		t.Fatal("a run asked for mid-tick was not held")
	}
	close(release)
	waitFor(t, "one follow-up run", func() bool { return runs.Load() == 2 && !status(s, "slow").Running })
	time.Sleep(20 * time.Millisecond)
	if runs.Load() != 2 {
		t.Fatalf("runs = %d, want 2", runs.Load())
	}
}

// "Crash-isolated" has to include panics: one in a tick goroutine would
// otherwise kill the whole server.
func TestPanickingTaskIsContained(t *testing.T) {
	s := testScheduler(t, nil, &task{slug: "bad", interval: time.Hour, tick: func(context.Context) error {
		panic("tick blew up")
	}})
	if err := s.Run("bad"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the panic to be recorded", func() bool { return status(s, "bad").LastError != "" })
	if ts := status(s, "bad"); ts.Running {
		t.Fatalf("task still marked running after a panic: %+v", ts)
	}
}

// A series on hiatus resumes without notice: it keeps the daily refresh
// and the chapter search that a finished series does not get.
func TestActiveSeries(t *testing.T) {
	for status, want := range map[string]bool{
		"releasing": true, "hiatus": true, "completed": false, "cancelled": false, "upcoming": false, "": false,
	} {
		if got := active(store.Series{Status: status}); got != want {
			t.Errorf("active(%q) = %v, want %v", status, got, want)
		}
	}
}
