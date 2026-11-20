package runner

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// EnvSetup is the preparation of one task shell environment.
type EnvSetup struct {
	// Tasks are the run's tasks that use the environment, sorted.
	Tasks []string
	// Cached is set when the environment came from the nix-tasks cache
	// rather than nix print-dev-env.
	Cached   bool
	Duration time.Duration
	Err      error
}

// SetupReporter is implemented by reporters that show environment
// preparation as a step of its own. For them, every environment the run
// needs is prepared before any task starts, instead of by the first task
// that needs it, so preparation output never mixes with task output. A
// broken environment still only fails the tasks that use it.
type SetupReporter interface {
	SetupFinished(envs []EnvSetup, duration time.Duration)
}

// prepareEnvs loads the environment of every task in order that runs a
// script, Jobs at a time.
func (e *Executor) prepareEnvs(ctx context.Context, graph *TaskGraph, order []string) []EnvSetup {
	var setups []EnvSetup
	byDrv := map[string][]string{}
	for _, name := range order {
		task := graph.tasks[name]
		if task.Type == config.TaskTypeBuild || taskScript(task) == "" {
			continue
		}
		drv := task.ShellDrvPath
		if drv == "" {
			var err error
			if drv, err = e.devEnvs.drvPath(ctx, name); err != nil {
				setups = append(setups, EnvSetup{Tasks: []string{name}, Err: err})
				continue
			}
		}
		byDrv[drv] = append(byDrv[drv], name)
	}

	drvs := make([]string, 0, len(byDrv))
	for drv := range byDrv {
		drvs = append(drvs, drv)
	}
	loaded := make([]EnvSetup, len(drvs))
	sem := make(chan struct{}, e.options.Jobs)
	var wg sync.WaitGroup
	for i, drv := range drvs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			entry := e.devEnvs.entry(ctx, drv, io.Discard)
			tasks := slices.Clone(byDrv[drv])
			slices.Sort(tasks)
			loaded[i] = EnvSetup{Tasks: tasks, Cached: entry.cached, Duration: entry.duration, Err: entry.err}
		}()
	}
	wg.Wait()

	setups = append(setups, loaded...)
	slices.SortFunc(setups, func(a, b EnvSetup) int { return strings.Compare(a.Tasks[0], b.Tasks[0]) })
	return setups
}
