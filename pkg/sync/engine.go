package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ohmylock/glenv/pkg/classifier"
	"github.com/ohmylock/glenv/pkg/envfile"
	"github.com/ohmylock/glenv/pkg/gitlab"
)

// ChangeKind identifies the type of diff change.
type ChangeKind string

const (
	ChangeCreate    ChangeKind = "create"
	ChangeUpdate    ChangeKind = "update"
	ChangeDelete    ChangeKind = "delete"
	ChangeUnchanged ChangeKind = "unchanged"
	ChangeSkipped   ChangeKind = "skipped"
)

// Change describes a single diff entry.
type Change struct {
	Kind           ChangeKind
	Key            string
	OldValue       string
	NewValue       string
	Classification string // human-readable tags, e.g. "masked", "protected", "file"
	SkipReason     string
	// Internal: used by Apply to pass classification data to the API call.
	varType     string
	masked      bool
	protected   bool
	raw         bool
	envScope    string
}

// DiffResult holds the complete set of changes between local and remote.
type DiffResult struct {
	Changes []Change
}

// Result is produced by a worker after attempting to apply one Change.
type Result struct {
	Change Change
	Error  error
}

// SyncReport summarises the outcome of an Apply run.
type SyncReport struct {
	Created   int
	Updated   int
	Deleted   int
	Unchanged int
	Skipped   int
	Failed    int
	Duration  time.Duration
	APICalls  int
	Errors    []error
}

// Options controls Engine behaviour.
type Options struct {
	Workers       int
	DryRun        bool
	DeleteMissing bool
}

// gitlabClient is the subset of the gitlab.Client API used by the engine.
// It is defined as an interface to allow test fakes.
type gitlabClient interface {
	CreateVariable(ctx context.Context, projectID string, req gitlab.CreateRequest) (*gitlab.Variable, error)
	UpdateVariable(ctx context.Context, projectID string, req gitlab.CreateRequest) (*gitlab.Variable, error)
	DeleteVariable(ctx context.Context, projectID, key, envScope string) error
}

// Engine orchestrates diff and apply operations.
type Engine struct {
	client     gitlabClient
	classifier *classifier.Classifier
	opts       Options
	projectID  string
}

// NewEngine creates a new Engine.
func NewEngine(client gitlabClient, cl *classifier.Classifier, opts Options, projectID string) *Engine {
	if opts.Workers <= 0 {
		opts.Workers = 5
	}
	return &Engine{
		client:     client,
		classifier: cl,
		opts:       opts,
		projectID:  projectID,
	}
}

// Diff computes the set of changes needed to bring remote in sync with local.
// envScope is passed as the environment_scope when creating/updating variables.
func (e *Engine) Diff(ctx context.Context, local []envfile.Variable, remote []gitlab.Variable, envScope string) DiffResult {
	// Client-side scope filtering: GitLab API does not reliably honour the
	// filter[environment_scope] query parameter on the LIST endpoint
	// (see https://gitlab.com/gitlab-org/gitlab/-/issues/343169), so we
	// filter the response ourselves before building the index.
	remote = gitlab.FilterByScope(remote, envScope)

	// Index remote by key for O(1) lookup.
	// After filtering, remote contains only variables matching the target scope
	// (exact match) or the wildcard "*". When both exist for the same key,
	// prefer the exact-scope entry so that scopeMatch and value comparison
	// operate on the precise variable rather than the wildcard one.
	remoteMap := make(map[string]gitlab.Variable, len(remote))
	for _, v := range remote {
		existing, ok := remoteMap[v.Key]
		if !ok || existing.EnvironmentScope == "*" {
			remoteMap[v.Key] = v
		}
	}

	localKeys := make(map[string]struct{}, len(local))
	var changes []Change

	for _, lv := range local {
		localKeys[lv.Key] = struct{}{}
		cl := e.classifier.Classify(lv.Key, lv.Value, envScope)

		classLabel := buildClassLabel(cl)

		rv, exists := remoteMap[lv.Key]
		// scopeMatch checks if the remote variable matches the target environment scope.
		// A match requires: remote exists AND (remote scope == target scope OR remote scope is "*").
		scopeMatch := exists && (rv.EnvironmentScope == envScope || rv.EnvironmentScope == "*")

		switch {
		case !scopeMatch:
			changes = append(changes, Change{
				Kind:           ChangeCreate,
				Key:            lv.Key,
				NewValue:       lv.Value,
				Classification: classLabel,
				varType:        cl.VarType,
				masked:         cl.Masked,
				protected:      cl.Protected,
				envScope:       envScope,
			})
		case rv.Value != lv.Value || rv.VariableType != cl.VarType || rv.Masked != cl.Masked || rv.Protected != cl.Protected:
			changes = append(changes, Change{
				Kind:           ChangeUpdate,
				Key:            lv.Key,
				OldValue:       rv.Value,
				NewValue:       lv.Value,
				Classification: classLabel,
				varType:        cl.VarType,
				masked:         cl.Masked,
				protected:      cl.Protected,
				raw:            rv.Raw,
				envScope:       rv.EnvironmentScope,
			})
		default:
			changes = append(changes, Change{
				Kind:           ChangeUnchanged,
				Key:            lv.Key,
				OldValue:       rv.Value,
				NewValue:       lv.Value,
				Classification: classLabel,
				envScope:       rv.EnvironmentScope,
			})
		}
	}

	// Remote-only vars: delete if DeleteMissing is enabled.
	if e.opts.DeleteMissing {
		for _, rv := range remote {
			if _, inLocal := localKeys[rv.Key]; !inLocal {
				changes = append(changes, Change{
					Kind:     ChangeDelete,
					Key:      rv.Key,
					OldValue: rv.Value,
					envScope: rv.EnvironmentScope,
				})
			}
		}
	}

	return DiffResult{Changes: changes}
}

// Apply executes all changes in diff using a worker pool. It is equivalent to
// ApplyWithCallback with a nil callback.
func (e *Engine) Apply(ctx context.Context, diff DiffResult) SyncReport {
	return e.ApplyWithCallback(ctx, diff, nil)
}

// ApplyWithCallback executes all changes concurrently. For each completed result
// (success or error), cb is called synchronously from the collecting goroutine.
func (e *Engine) ApplyWithCallback(ctx context.Context, diff DiffResult, cb func(Result)) SyncReport {
	start := time.Now()
	report := SyncReport{}

	// Count non-actionable changes upfront — don't send through the worker pool.
	var actionable []Change
	for _, ch := range diff.Changes {
		switch ch.Kind {
		case ChangeUnchanged:
			report.Unchanged++
			if cb != nil {
				cb(Result{Change: ch})
			}
		case ChangeSkipped:
			report.Skipped++
			if cb != nil {
				cb(Result{Change: ch})
			}
		default:
			actionable = append(actionable, ch)
		}
	}

	if len(actionable) == 0 {
		report.Duration = time.Since(start)
		return report
	}

	taskCh := make(chan Change, len(actionable))
	resultCh := make(chan Result, len(actionable))

	// Enqueue only actionable tasks.
	for _, ch := range actionable {
		taskCh <- ch
	}
	close(taskCh)

	// Launch worker pool.
	var wg sync.WaitGroup
	for i := 0; i < e.opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				// Check cancellation before each task to stop early.
				if ctx.Err() != nil {
					resultCh <- Result{Change: task, Error: ctx.Err()}
					continue
				}
				resultCh <- e.applyOne(ctx, task)
			}
		}()
	}

	// Close resultCh once all workers are done.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results.
	for r := range resultCh {
		if cb != nil {
			cb(r)
		}
		if r.Error != nil {
			report.Failed++
			report.Errors = append(report.Errors, r.Error)
			continue
		}
		switch r.Change.Kind {
		case ChangeCreate:
			report.Created++
			if !e.opts.DryRun {
				report.APICalls++
			}
		case ChangeUpdate:
			report.Updated++
			if !e.opts.DryRun {
				report.APICalls++
			}
		case ChangeDelete:
			report.Deleted++
			if !e.opts.DryRun {
				report.APICalls++
			}
		}
	}

	report.Duration = time.Since(start)
	return report
}

// applyOne executes a single Change, routing to the appropriate API call.
func (e *Engine) applyOne(ctx context.Context, task Change) Result {
	switch task.Kind {
	case ChangeUnchanged, ChangeSkipped:
		return Result{Change: task}

	case ChangeCreate:
		if e.opts.DryRun {
			return Result{Change: task}
		}
		req := gitlab.CreateRequest{
			Key:              task.Key,
			Value:            task.NewValue,
			VariableType:     task.varType,
			EnvironmentScope: task.envScope,
			Masked:           task.masked,
			Protected:        task.protected,
		}
		if req.VariableType == "" {
			req.VariableType = "env_var"
		}
		_, err := e.client.CreateVariable(ctx, e.projectID, req)
		if err != nil {
			return Result{Change: task, Error: fmt.Errorf("create %s: %w", task.Key, err)}
		}
		return Result{Change: task}

	case ChangeUpdate:
		if e.opts.DryRun {
			return Result{Change: task}
		}
		req := gitlab.CreateRequest{
			Key:              task.Key,
			Value:            task.NewValue,
			VariableType:     task.varType,
			EnvironmentScope: task.envScope,
			Masked:           task.masked,
			Protected:        task.protected,
			Raw:              task.raw,
		}
		if req.VariableType == "" {
			req.VariableType = "env_var"
		}
		_, err := e.client.UpdateVariable(ctx, e.projectID, req)
		if err != nil {
			return Result{Change: task, Error: fmt.Errorf("update %s: %w", task.Key, err)}
		}
		return Result{Change: task}

	case ChangeDelete:
		if e.opts.DryRun {
			return Result{Change: task}
		}
		err := e.client.DeleteVariable(ctx, e.projectID, task.Key, task.envScope)
		if err != nil {
			return Result{Change: task, Error: fmt.Errorf("delete %s: %w", task.Key, err)}
		}
		return Result{Change: task}

	default:
		return Result{Change: task, Error: fmt.Errorf("unknown change kind: %s", task.Kind)}
	}
}

// buildClassLabel returns a human-readable classification string from a Classification.
func buildClassLabel(cl classifier.Classification) string {
	label := cl.VarType
	if cl.Masked {
		label += ",masked"
	}
	if cl.Protected {
		label += ",protected"
	}
	return label
}
