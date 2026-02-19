package sync

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ohmylock/glenv/pkg/classifier"
	"github.com/ohmylock/glenv/pkg/envfile"
	"github.com/ohmylock/glenv/pkg/gitlab"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClient implements the gitlabClient interface for testing.
type fakeClient struct {
	createFn func(ctx context.Context, projectID string, req gitlab.CreateRequest) (*gitlab.Variable, error)
	updateFn func(ctx context.Context, projectID string, req gitlab.CreateRequest) (*gitlab.Variable, error)
	deleteFn func(ctx context.Context, projectID, key, envScope string) error
	calls    atomic.Int32
}

func (f *fakeClient) CreateVariable(ctx context.Context, projectID string, req gitlab.CreateRequest) (*gitlab.Variable, error) {
	f.calls.Add(1)
	if f.createFn != nil {
		return f.createFn(ctx, projectID, req)
	}
	return &gitlab.Variable{Key: req.Key, Value: req.Value}, nil
}

func (f *fakeClient) UpdateVariable(ctx context.Context, projectID string, req gitlab.CreateRequest) (*gitlab.Variable, error) {
	f.calls.Add(1)
	if f.updateFn != nil {
		return f.updateFn(ctx, projectID, req)
	}
	return &gitlab.Variable{Key: req.Key, Value: req.Value}, nil
}

func (f *fakeClient) DeleteVariable(ctx context.Context, projectID, key, envScope string) error {
	f.calls.Add(1)
	if f.deleteFn != nil {
		return f.deleteFn(ctx, projectID, key, envScope)
	}
	return nil
}

func newTestEngine(client gitlabClient, opts Options) *Engine {
	cl := classifier.New(classifier.Rules{})
	return NewEngine(client, cl, opts, "proj-1")
}

// --- Diff tests ---

func TestDiff_CreateNew(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{})

	local := []envfile.Variable{{Key: "NEW_VAR", Value: "hello"}}
	remote := []gitlab.Variable{}

	diff := engine.Diff(context.Background(), local, remote, "*")

	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ChangeCreate, diff.Changes[0].Kind)
	assert.Equal(t, "NEW_VAR", diff.Changes[0].Key)
	assert.Equal(t, "hello", diff.Changes[0].NewValue)
}

func TestDiff_UpdateChanged(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{})

	local := []envfile.Variable{{Key: "FOO", Value: "new_value"}}
	remote := []gitlab.Variable{{Key: "FOO", Value: "old_value", EnvironmentScope: "*"}}

	diff := engine.Diff(context.Background(), local, remote, "*")

	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ChangeUpdate, diff.Changes[0].Kind)
	assert.Equal(t, "FOO", diff.Changes[0].Key)
	assert.Equal(t, "old_value", diff.Changes[0].OldValue)
	assert.Equal(t, "new_value", diff.Changes[0].NewValue)
}

func TestDiff_Unchanged(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{})

	local := []envfile.Variable{{Key: "SAME", Value: "value"}}
	remote := []gitlab.Variable{{Key: "SAME", Value: "value", VariableType: "env_var", EnvironmentScope: "*"}}

	diff := engine.Diff(context.Background(), local, remote, "*")

	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ChangeUnchanged, diff.Changes[0].Kind)
	assert.Equal(t, "SAME", diff.Changes[0].Key)
}

func TestDiff_DeleteMissing_Enabled(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{DeleteMissing: true})

	local := []envfile.Variable{{Key: "LOCAL", Value: "v"}}
	remote := []gitlab.Variable{
		{Key: "LOCAL", Value: "v", VariableType: "env_var", EnvironmentScope: "*"},
		{Key: "REMOTE_ONLY", Value: "x", EnvironmentScope: "*"},
	}

	diff := engine.Diff(context.Background(), local, remote, "*")

	require.Len(t, diff.Changes, 2)
	var kinds []ChangeKind
	for _, ch := range diff.Changes {
		kinds = append(kinds, ch.Kind)
	}
	assert.Contains(t, kinds, ChangeUnchanged)
	assert.Contains(t, kinds, ChangeDelete)
}

func TestDiff_DeleteMissing_Disabled(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{DeleteMissing: false})

	local := []envfile.Variable{}
	remote := []gitlab.Variable{{Key: "REMOTE_ONLY", Value: "x", EnvironmentScope: "*"}}

	diff := engine.Diff(context.Background(), local, remote, "*")

	// Nothing to do — no deletes when disabled.
	assert.Empty(t, diff.Changes)
}

func TestDiff_MultipleChanges(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{DeleteMissing: true})

	local := []envfile.Variable{
		{Key: "NEW_VAR", Value: "created"},
		{Key: "CHANGED", Value: "new_val"},
		{Key: "SAME", Value: "unchanged"},
	}
	remote := []gitlab.Variable{
		{Key: "CHANGED", Value: "old_val", VariableType: "env_var", EnvironmentScope: "*"},
		{Key: "SAME", Value: "unchanged", VariableType: "env_var", EnvironmentScope: "*"},
		{Key: "STALE", Value: "to_delete", VariableType: "env_var", EnvironmentScope: "*"},
	}

	diff := engine.Diff(context.Background(), local, remote, "*")

	require.Len(t, diff.Changes, 4)

	kindMap := make(map[string]ChangeKind)
	for _, ch := range diff.Changes {
		kindMap[ch.Key] = ch.Kind
	}
	assert.Equal(t, ChangeCreate, kindMap["NEW_VAR"])
	assert.Equal(t, ChangeUpdate, kindMap["CHANGED"])
	assert.Equal(t, ChangeUnchanged, kindMap["SAME"])
	assert.Equal(t, ChangeDelete, kindMap["STALE"])
}

// --- Apply tests ---

func TestApply_DryRun(t *testing.T) {
	fake := &fakeClient{}
	engine := newTestEngine(fake, Options{Workers: 2, DryRun: true})

	diff := DiffResult{Changes: []Change{
		{Kind: ChangeCreate, Key: "A", NewValue: "1"},
		{Kind: ChangeUpdate, Key: "B", OldValue: "old", NewValue: "new"},
		{Kind: ChangeDelete, Key: "C"},
	}}

	report := engine.Apply(context.Background(), diff)

	// Dry-run: no API calls, but create/update/delete counts are still reported.
	assert.Equal(t, int32(0), fake.calls.Load(), "dry-run must not make API calls")
	assert.Equal(t, 0, report.APICalls, "dry-run must report 0 API calls")
	assert.Equal(t, 1, report.Created)
	assert.Equal(t, 1, report.Updated)
	assert.Equal(t, 1, report.Deleted)
	assert.Equal(t, 0, report.Failed)
}

func TestApply_Concurrent(t *testing.T) {
	fake := &fakeClient{}
	engine := newTestEngine(fake, Options{Workers: 5})

	changes := make([]Change, 20)
	for i := range changes {
		changes[i] = Change{Kind: ChangeCreate, Key: fmt.Sprintf("VAR_%d", i), NewValue: "v"}
	}
	diff := DiffResult{Changes: changes}

	report := engine.Apply(context.Background(), diff)

	assert.Equal(t, 20, report.Created)
	assert.Equal(t, 0, report.Failed)
	assert.Equal(t, int32(20), fake.calls.Load())
}

func TestApply_ContextCancel(t *testing.T) {
	// Use a channel to block workers until we cancel.
	// Each call to createFn blocks until ctx is cancelled, then returns error.
	var started atomic.Int32

	fake := &fakeClient{
		createFn: func(ctx context.Context, _ string, _ gitlab.CreateRequest) (*gitlab.Variable, error) {
			started.Add(1)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	// 2 workers, 20 tasks — workers will be permanently blocked in the createFn.
	engine := newTestEngine(fake, Options{Workers: 2})

	changes := make([]Change, 20)
	for i := range changes {
		changes[i] = Change{Kind: ChangeCreate, Key: fmt.Sprintf("VAR_%d", i), NewValue: "v"}
	}
	diff := DiffResult{Changes: changes}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan SyncReport, 1)
	go func() {
		done <- engine.Apply(ctx, diff)
	}()

	// Wait until both workers are blocked in createFn, then cancel.
	require.Eventually(t, func() bool {
		return started.Load() >= 2
	}, time.Second, time.Millisecond)
	cancel()

	report := <-done

	// The 2 in-flight tasks return ctx error, the remaining 18 are skipped at
	// the pre-check. All end up in Failed.
	assert.Greater(t, report.Failed, 0, "should have failures from cancellation")
	assert.Less(t, report.Created, 20, "should be a partial report")
}

func TestApply_ErrorHandling(t *testing.T) {
	errKey := "BAD_VAR"
	fake := &fakeClient{
		createFn: func(ctx context.Context, _ string, req gitlab.CreateRequest) (*gitlab.Variable, error) {
			if req.Key == errKey {
				return nil, fmt.Errorf("API error: variable invalid")
			}
			return &gitlab.Variable{Key: req.Key, Value: req.Value}, nil
		},
	}
	engine := newTestEngine(fake, Options{Workers: 3})

	diff := DiffResult{Changes: []Change{
		{Kind: ChangeCreate, Key: "GOOD_1", NewValue: "v1"},
		{Kind: ChangeCreate, Key: errKey, NewValue: "bad"},
		{Kind: ChangeCreate, Key: "GOOD_2", NewValue: "v2"},
	}}

	report := engine.Apply(context.Background(), diff)

	assert.Equal(t, 2, report.Created)
	assert.Equal(t, 1, report.Failed)
	assert.Len(t, report.Errors, 1)
}

func TestApply_UnchangedAndSkipped(t *testing.T) {
	fake := &fakeClient{}
	engine := newTestEngine(fake, Options{Workers: 2})

	diff := DiffResult{Changes: []Change{
		{Kind: ChangeUnchanged, Key: "SAME"},
		{Kind: ChangeSkipped, Key: "SKIP", SkipReason: "placeholder"},
	}}

	report := engine.Apply(context.Background(), diff)

	assert.Equal(t, 0, report.Created)
	assert.Equal(t, 0, report.Failed)
	assert.Equal(t, 1, report.Unchanged)
	assert.Equal(t, 1, report.Skipped)
	assert.Equal(t, int32(0), fake.calls.Load())
}

func TestApplyWithCallback(t *testing.T) {
	fake := &fakeClient{}
	engine := newTestEngine(fake, Options{Workers: 2})

	diff := DiffResult{Changes: []Change{
		{Kind: ChangeCreate, Key: "A", NewValue: "1"},
		{Kind: ChangeCreate, Key: "B", NewValue: "2"},
	}}

	var results []Result
	report := engine.ApplyWithCallback(context.Background(), diff, func(r Result) {
		results = append(results, r)
	})

	assert.Equal(t, 2, report.Created)
	require.Len(t, results, 2)

	keys := map[string]bool{results[0].Change.Key: true, results[1].Change.Key: true}
	assert.True(t, keys["A"] && keys["B"], "callback must receive results for both keys")
	for _, r := range results {
		assert.Nil(t, r.Error, "callback results must have no error")
		assert.Equal(t, ChangeCreate, r.Change.Kind)
	}
}

func TestApply_CreateUpdateDelete(t *testing.T) {
	var creates, updates, deletes atomic.Int32
	fake := &fakeClient{
		createFn: func(ctx context.Context, _ string, req gitlab.CreateRequest) (*gitlab.Variable, error) {
			creates.Add(1)
			return &gitlab.Variable{Key: req.Key, Value: req.Value}, nil
		},
		updateFn: func(ctx context.Context, _ string, req gitlab.CreateRequest) (*gitlab.Variable, error) {
			updates.Add(1)
			return &gitlab.Variable{Key: req.Key, Value: req.Value}, nil
		},
		deleteFn: func(ctx context.Context, _ string, key, _ string) error {
			deletes.Add(1)
			return nil
		},
	}
	engine := newTestEngine(fake, Options{Workers: 3})

	diff := DiffResult{Changes: []Change{
		{Kind: ChangeCreate, Key: "NEW_VAR", NewValue: "v1"},
		{Kind: ChangeUpdate, Key: "CHANGED_VAR", OldValue: "old", NewValue: "new"},
		{Kind: ChangeDelete, Key: "OLD_VAR"},
	}}

	report := engine.Apply(context.Background(), diff)

	assert.Equal(t, int32(1), creates.Load(), "should call CreateVariable once")
	assert.Equal(t, int32(1), updates.Load(), "should call UpdateVariable once")
	assert.Equal(t, int32(1), deletes.Load(), "should call DeleteVariable once")
	assert.Equal(t, 1, report.Created)
	assert.Equal(t, 1, report.Updated)
	assert.Equal(t, 1, report.Deleted)
	assert.Equal(t, 0, report.Failed)
	assert.Equal(t, 3, report.APICalls)
}

func TestApply_EmptyDiff(t *testing.T) {
	fake := &fakeClient{}
	engine := newTestEngine(fake, Options{Workers: 3})

	diff := DiffResult{Changes: []Change{}}

	report := engine.Apply(context.Background(), diff)

	assert.Equal(t, int32(0), fake.calls.Load(), "no API calls for empty diff")
	assert.Equal(t, 0, report.Created)
	assert.Equal(t, 0, report.Updated)
	assert.Equal(t, 0, report.Deleted)
	assert.Equal(t, 0, report.Failed)
	assert.Equal(t, 0, report.APICalls)
}

func TestDiff_MetadataChange(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{})

	// Same value, but remote has wrong type — should trigger update.
	local := []envfile.Variable{{Key: "API_KEY", Value: "supersecretvalue123"}}
	remote := []gitlab.Variable{{Key: "API_KEY", Value: "supersecretvalue123", VariableType: "env_var", Masked: false, EnvironmentScope: "*"}}

	diff := engine.Diff(context.Background(), local, remote, "*")

	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ChangeUpdate, diff.Changes[0].Kind, "metadata mismatch should trigger update")
	assert.Contains(t, diff.Changes[0].Classification, "masked")
}

func TestDiff_ClassificationAttached(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{})

	// API_KEY with long value → should be classified as masked.
	local := []envfile.Variable{{Key: "API_KEY", Value: "supersecretvalue123"}}
	remote := []gitlab.Variable{}

	diff := engine.Diff(context.Background(), local, remote, "*")

	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ChangeCreate, diff.Changes[0].Kind)
	assert.Contains(t, diff.Changes[0].Classification, "masked")
}

func TestDiff_ClassificationProtectedOnly(t *testing.T) {
	// production env + secret key with short value → protected but not masked
	// (masked requires value length >= 8; "abc" is 3 chars)
	cl := classifier.New(classifier.Rules{})
	engine := NewEngine(&fakeClient{}, cl, Options{}, "proj-1")

	local := []envfile.Variable{{Key: "DB_SECRET", Value: "abc"}}
	remote := []gitlab.Variable{}

	diff := engine.Diff(context.Background(), local, remote, "production")

	require.Len(t, diff.Changes, 1)
	assert.Contains(t, diff.Changes[0].Classification, "protected")
	assert.NotContains(t, diff.Changes[0].Classification, "masked")
}

func TestDiff_ScopeMismatch_CreateNew(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{})

	// Local: "production", Remote: same key with "staging" scope.
	// Because the scopes don't match, we should CREATE for production, not UPDATE the staging one.
	local := []envfile.Variable{{Key: "DB_PASS", Value: "prod_secret"}}
	remote := []gitlab.Variable{{Key: "DB_PASS", Value: "staging_secret", EnvironmentScope: "staging"}}

	diff := engine.Diff(context.Background(), local, remote, "production")

	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ChangeCreate, diff.Changes[0].Kind, "mismatched scope should trigger CREATE")
	assert.Equal(t, "production", diff.Changes[0].envScope)
}

func TestDiff_WildcardScope_Update(t *testing.T) {
	engine := newTestEngine(&fakeClient{}, Options{})

	// Local: "production", Remote: same key with wildcard "*" scope.
	// Wildcard matches all scopes, so we should UPDATE, not CREATE.
	local := []envfile.Variable{{Key: "COMMON_VAR", Value: "new_value"}}
	remote := []gitlab.Variable{{Key: "COMMON_VAR", Value: "old_value", EnvironmentScope: "*"}}

	diff := engine.Diff(context.Background(), local, remote, "production")

	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ChangeUpdate, diff.Changes[0].Kind, "wildcard scope should match any target scope")
	// envScope must equal rv.EnvironmentScope ("*") so UpdateVariable uses
	// filter[environment_scope]=* and correctly locates the wildcard variable.
	assert.Equal(t, "*", diff.Changes[0].envScope)
}

func TestApply_WildcardScope_UpdatePassesCorrectScope(t *testing.T) {
	// Verify that when a wildcard-scoped remote variable is updated,
	// UpdateVariable is called with EnvironmentScope="*" (not the target scope).
	// Using "production" as filter would return 404 because the variable has scope "*".
	var capturedScope string
	fake := &fakeClient{
		updateFn: func(_ context.Context, _ string, req gitlab.CreateRequest) (*gitlab.Variable, error) {
			capturedScope = req.EnvironmentScope
			return &gitlab.Variable{Key: req.Key, Value: req.Value}, nil
		},
	}
	engine := newTestEngine(fake, Options{Workers: 1})

	local := []envfile.Variable{{Key: "COMMON_VAR", Value: "new_value"}}
	remote := []gitlab.Variable{{Key: "COMMON_VAR", Value: "old_value", EnvironmentScope: "*"}}

	diff := engine.Diff(context.Background(), local, remote, "production")
	require.Len(t, diff.Changes, 1)
	require.Equal(t, ChangeUpdate, diff.Changes[0].Kind)

	report := engine.Apply(context.Background(), diff)

	require.Equal(t, 0, report.Failed)
	assert.Equal(t, "*", capturedScope, "UpdateVariable must use the remote variable's actual scope as filter")
}
