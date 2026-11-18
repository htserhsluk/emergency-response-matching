//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"github.com/htserhsluk/emergency-response-matching/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type workflowLab struct {
	router       *coordinator.Router
	urls         [3]string
	stores       [3]*storage.Store
	pools        [3]*pgxpool.Pool
	loseReserve  atomic.Bool
	stallReserve atomic.Bool
	slowIntents  atomic.Bool
}

func newWorkflowLab(t *testing.T) *workflowLab {
	t.Helper()
	l := &workflowLab{}
	for i := range 3 {
		s, p := resourceFixture(t)
		l.stores[i] = s
		l.pools[i] = p
		handler := httpapp.NewServer(s, "workflow-secret-at-least-32-characters", true)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/internal/commands/intent" && l.slowIntents.Load() {
				time.Sleep(120 * time.Millisecond)
			}
			if i == 2 && r.URL.Path == "/internal/commands/reserve" && l.stallReserve.Load() {
				io.Copy(io.Discard, r.Body)
				select {
				case <-r.Context().Done():
				case <-time.After(6 * time.Second):
				}
				http.Error(w, "stalled dependency", 503)
				return
			}
			if r.URL.Path == "/internal/commands/reserve" && l.loseReserve.CompareAndSwap(true, false) {
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, r)
				http.Error(w, "response lost after commit", 503)
				return
			}
			handler.ServeHTTP(w, r)
		}))
		t.Cleanup(server.Close)
		l.urls[i] = server.URL
	}
	l.restart()
	return l
}

func TestSlowDefinitiveCandidatesPersistWithinPage(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	r, d := l.request(t, 0, "slow-candidate-search")
	for i := 0; i < 20; i++ {
		supply := l.resource(t, 1, fmt.Sprintf("rejected-again-%d", i))
		c := contracts.IntentCommand{AssignmentID: contracts.EntityID("assignment", d.Event.EventID+":"+supply.ID), RequestID: r.ID, ResourceID: supply.ID, Generation: 1, DistanceMeters: 0}
		_, err := l.pools[0].Exec(ctx, `INSERT INTO assignment_attempts(assignment_id,request_id,resource_id,generation,distance_meters,payload_hash,phase) VALUES($1,$2,$3,1,0,$4,'REJECTED')`, c.AssignmentID, r.ID, supply.ID, contracts.PayloadHash(c))
		if err != nil {
			t.Fatal(err)
		}
	}
	winner, err := l.router.RegisterResource(ctx, keyOnShard("register-resource", 2, "after-slow-rejections"), resourceBody(0.001, 0, true))
	if err != nil {
		t.Fatal(err)
	}
	l.slowIntents.Store(true)
	state, err := l.router.HandleDelivery(ctx, d)
	if err != nil || !state.Ack {
		t.Fatal(state, err)
	}
	var after string
	var remaining int
	if err = l.pools[0].QueryRow(ctx, `SELECT search_after_id,retry_limit-expected_retry FROM processed_events WHERE request_id=$1`, r.ID).Scan(&after, &remaining); err != nil {
		t.Fatal(err)
	}
	if after == "" || remaining != 3 {
		t.Fatalf("no bounded candidate progress: cursor=%q budget=%d", after, remaining)
	}
	for attempt := 0; attempt < 6; attempt++ {
		if err = l.router.ReconcileOnce(ctx); err != nil {
			t.Fatal(err)
		}
		got, e := l.router.Get(ctx, r.ID)
		if e != nil {
			t.Fatal(e)
		}
		if got.AssignedResourceID == winner.ID {
			return
		}
	}
	t.Fatal("bounded continuations never reached eligible resource")
}

func TestAssignmentTimeoutPersistsBackoffWithoutStarvingReconciliation(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	l.stallReserve.Store(true)
	for i := range 3 {
		resource := l.resource(t, 2, fmt.Sprintf("slow-resource-%d", i))
		request, d := l.request(t, 0, fmt.Sprintf("slow-request-%d", i))
		if _, err := l.stores[0].Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"}); err != nil {
			t.Fatal(err)
		}
		if _, err := l.stores[0].BeginIntent(ctx, contracts.IntentCommand{AssignmentID: fmt.Sprintf("%032x", 100+i), RequestID: request.ID, ResourceID: resource.ID, Generation: 1}); err != nil {
			t.Fatal(err)
		}
	}
	resource := l.resource(t, 1, "healthy-resource")
	request, d := l.request(t, 0, "healthy-request")
	if _, err := l.stores[0].Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.stores[0].BeginIntent(ctx, contracts.IntentCommand{AssignmentID: fmt.Sprintf("%032x", 200), RequestID: request.ID, ResourceID: resource.ID, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	// This tests bounded dependency stalls, not wall-clock scheduling. A VM
	// clock correction must not turn prepared, runnable fixtures into future work.
	if _, err := l.pools[0].Exec(ctx, `UPDATE processed_events SET due_at='2000-01-01 UTC'`); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pending, pendingErr := l.router.PendingWork(ctx)
	if pendingErr != nil || len(pending) != 4 {
		t.Fatalf("prepared runnable workflows=%d error=%v", len(pending), pendingErr)
	}
	reconcileErr := l.router.ReconcileOnce(bounded)
	got, err := l.router.Get(ctx, request.ID)
	if err != nil || got.Status != "ASSIGNED" {
		rows, queryErr := l.pools[0].Query(ctx, `SELECT request_id,state,expected_retry,due_at,clock_timestamp() FROM processed_events ORDER BY request_id`)
		if queryErr == nil {
			defer rows.Close()
			for rows.Next() {
				values, _ := rows.Values()
				t.Logf("workflow at failure: %v", values)
			}
		}
		t.Fatalf("healthy work starved %+v read=%v reconcile=%v deadline=%v", got, err, reconcileErr, bounded.Err())
	}
	var retried int
	if err = l.pools[0].QueryRow(ctx, `SELECT count(*) FROM processed_events WHERE expected_retry=1`).Scan(&retried); err != nil || retried != 3 {
		t.Fatalf("timeouts bypassed backoff %d %v", retried, err)
	}
}

func TestCoordinatorRejectsUnsafeAssignmentCommands(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	request, d := l.request(t, 0, "command-boundary")
	for _, kind := range []string{"wrong-type", "outside-radius"} {
		input := resourceBody(0, 0, true)
		if kind == "wrong-type" {
			input.Type = "FOOD"
		} else {
			input = resourceBody(10, 10, true)
		}
		resource, err := l.router.RegisterResource(ctx, keyOnShard("register-resource", 2, kind), input)
		if err != nil {
			t.Fatal(err)
		}
		c := contracts.IntentCommand{AssignmentID: contracts.EntityID("test-intent", kind), RequestID: request.ID, ResourceID: resource.ID, Generation: 1}
		if _, err = l.router.BeginIntent(ctx, c); err == nil {
			t.Fatalf("unsafe %s candidate accepted", kind)
		}
	}
	resource := l.resource(t, 1, "valid-candidate")
	intent := contracts.IntentCommand{AssignmentID: fmt.Sprintf("%032x", 201), RequestID: request.ID, ResourceID: resource.ID, Generation: 1}
	if _, err := l.router.Reserve(ctx, contracts.ReserveCommand{AssignmentID: intent.AssignmentID, RequestID: request.ID, ResourceID: resource.ID}); err == nil {
		t.Fatal("reservation without prior durable intent accepted")
	}
	if _, err := l.router.BeginIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"finalize", "reject", "unmatched"} {
		if _, err := l.router.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: action, AssignmentID: intent.AssignmentID}); err == nil {
			t.Fatalf("unguarded %s accepted", action)
		}
	}
	got, _ := l.router.Get(ctx, request.ID)
	if got.Status != "ASSIGNING" {
		t.Fatal("unsafe command changed request", got)
	}
}

func TestMatchingExpandsRadiusAndRejectsWrongType(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	wrong := resourceBody(0, 0, true)
	wrong.Type = "FOOD"
	if _, err := l.router.RegisterResource(ctx, keyOnShard("register-resource", 1, "wrong-nearby"), wrong); err != nil {
		t.Fatal(err)
	}
	supply, err := l.router.RegisterResource(ctx, keyOnShard("register-resource", 2, "six-km"), resourceBody(0.06, 0, true))
	if err != nil {
		t.Fatal(err)
	}
	_, d := l.request(t, 0, "radius-request")
	state, err := l.router.HandleDelivery(ctx, d)
	if err != nil || state.Request.AssignedResourceID != supply.ID || state.Intent == nil || state.Intent.DistanceMeters < 5000 || state.Intent.DistanceMeters > 10000 {
		t.Fatalf("radius/type matching %+v %v", state, err)
	}
	_, second := l.request(t, 0, "exhausted-radius")
	distant, err := l.router.RegisterResource(ctx, keyOnShard("register-resource", 2, "distant"), resourceBody(14, 75.5, true))
	if err != nil {
		t.Fatal(err)
	}
	state, err = l.router.HandleDelivery(ctx, second)
	if err != nil || state.Request.AssignedResourceID != distant.ID {
		t.Fatalf("distant resource not matched %+v %v", state, err)
	}
}
func (l *workflowLab) restart() {
	l.router = coordinator.NewRouter(l.urls, "workflow-secret-at-least-32-characters")
}

func TestGlobalSearchExhaustionAndNewGeneration(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	r, d := l.request(t, 0, "global-empty")
	state, err := l.router.HandleDelivery(ctx, d)
	if err != nil || state.Request.Status != "UNMATCHED" || state.Progress.RadiusIndex != len(contracts.SearchRadii) {
		t.Fatal("incomplete global exhaustion", state, err)
	}
	// A resource near the antipode must be covered by the final radius.
	supply, err := l.router.RegisterResource(ctx, keyOnShard("register-resource", 2, "antipodal"), resourceBody(0, 179.99, true))
	if err != nil {
		t.Fatal(err)
	}
	next, err := l.router.ScheduleMatch(ctx, r.ID, "restart-global-search-0001")
	if err != nil || next.Generation != 2 {
		t.Fatal(next, err)
	}
	var payload []byte
	err = l.pools[0].QueryRow(ctx, `SELECT payload,delivery_id,retry_number,destination FROM outbox_events WHERE request_id=$1 AND generation=2`, r.ID).Scan(&payload, &d.DeliveryID, &d.RetryNumber, &d.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(payload, &d.Event); err != nil {
		t.Fatal(err)
	}
	state, err = l.stores[0].Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"})
	if err != nil || state.Progress.RadiusIndex != 0 {
		t.Fatal("new generation did not reset", state, err)
	}
	state, err = l.router.HandleDelivery(ctx, d)
	if err != nil || state.Request.AssignedResourceID != supply.ID {
		t.Fatal("antipodal resource omitted", state, err)
	}
}
func keyOnShard(kind string, shard int, tag string) string {
	for n := 0; ; n++ {
		key := fmt.Sprintf("workflow-%s-%d", tag, n)
		got, _ := routing.ShardFor(contracts.EntityID(kind, key))
		if got == shard {
			return key
		}
	}
}
func (l *workflowLab) request(t *testing.T, shard int, tag string) (contracts.Request, contracts.Delivery) {
	t.Helper()
	r, err := l.router.Create(context.Background(), keyOnShard("create-request", shard, tag), body())
	if err != nil {
		t.Fatal(err)
	}
	var d contracts.Delivery
	var payload []byte
	err = l.pools[shard].QueryRow(context.Background(), `SELECT payload,delivery_id,retry_number,destination FROM outbox_events WHERE request_id=$1`, r.ID).Scan(&payload, &d.DeliveryID, &d.RetryNumber, &d.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(payload, &d.Event); err != nil {
		t.Fatal(err)
	}
	return r, d
}
func (l *workflowLab) resource(t *testing.T, shard int, tag string) contracts.Resource {
	t.Helper()
	r, err := l.router.RegisterResource(context.Background(), keyOnShard("register-resource", shard, tag), contracts.RegisterResource{Type: "WATER", Location: body().Location, Availability: resourceBody(0, 0, true).Availability})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestTwoRequestsOneResourceAndDuplicateManualRace(t *testing.T) {
	l := newWorkflowLab(t)
	resource := l.resource(t, 2, "supply")
	a, da := l.request(t, 0, "first")
	b, db := l.request(t, 1, "second")
	// Prepare the two initial attempts as already due. Retry/future-attempt
	// scheduling is covered separately; this case asserts concurrent ownership.
	for i, d := range []contracts.Delivery{da, db} {
		if _, err := l.stores[i].Workflow(context.Background(), contracts.WorkflowCommand{Delivery: d, Action: "inspect"}); err != nil {
			t.Fatal(err)
		}
		if _, err := l.pools[i].Exec(context.Background(), `UPDATE processed_events SET due_at='2000-01-01 UTC'`); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := da
			if i%2 == 1 {
				d = db
			}
			state, err := l.router.HandleDelivery(context.Background(), d)
			if (err != nil && !errors.Is(err, contracts.ErrUnavailable)) || (err == nil && !state.Ack) {
				t.Errorf("handle %+v %v", state, err)
			}
			if _, err = l.router.ScheduleMatch(context.Background(), d.Event.RequestID, fmt.Sprintf("manual-race-%016d", i)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	// Ack may mean a durable retry, not immediate assignment. Under load a
	// bounded attempt can expire legitimately; exercise the real due-aware
	// reconciler instead of assuming every concurrent delivery finishes inline.
	settle, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var ar, br contracts.Request
	for {
		var errA, errB error
		ar, errA = l.router.Get(settle, a.ID)
		br, errB = l.router.Get(settle, b.ID)
		if errA != nil || errB != nil {
			t.Fatalf("read contenders: %v %v", errA, errB)
		}
		if ar.Status == "ASSIGNED" || br.Status == "ASSIGNED" {
			break
		}
		if err := l.router.ReconcileOnce(settle); err != nil && !errors.Is(err, contracts.ErrUnavailable) {
			t.Fatal(err)
		}
		if settle.Err() != nil {
			t.Fatalf("contenders did not converge: %+v %+v", ar, br)
		}
		time.Sleep(100 * time.Millisecond)
	}
	assigned := 0
	for _, r := range []contracts.Request{ar, br} {
		if r.Status == "ASSIGNED" {
			assigned++
			if r.AssignedResourceID != resource.ID {
				t.Fatal(r)
			}
		}
	}
	if assigned != 1 {
		t.Fatalf("owners %+v %+v", ar, br)
	}
}

func TestFutureDueIntentDefersAndResumesSameReservation(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	resource := l.resource(t, 2, "clock-step-resource")
	request, d := l.request(t, 0, "clock-step-request")
	if _, err := l.stores[0].Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"}); err != nil {
		t.Fatal(err)
	}
	intent := contracts.IntentCommand{AssignmentID: fmt.Sprintf("%032x", 300), RequestID: request.ID, ResourceID: resource.ID, Generation: 1}
	if _, err := l.stores[0].BeginIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	// Emulate persisted due times lying ahead of the current database clock.
	if _, err := l.pools[0].Exec(ctx, `UPDATE processed_events SET due_at=clock_timestamp()+interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.router.HandleDelivery(ctx, d); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatalf("future work executed: %v", err)
	}
	if work, err := l.router.PendingWork(ctx); err != nil || len(work) != 0 {
		t.Fatalf("future work selected: %+v %v", work, err)
	}
	if err := l.router.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := l.router.GetResource(ctx, resource.ID); err != nil || got.Status != "AVAILABLE" {
		t.Fatalf("future intent reserved resource: %+v %v", got, err)
	}
	if _, err := l.pools[0].Exec(ctx, `UPDATE processed_events SET due_at='2000-01-01 UTC'`); err != nil {
		t.Fatal(err)
	}
	if err := l.router.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := l.router.Get(ctx, request.ID); err != nil || got.Status != "ASSIGNED" || got.AssignedResourceID != resource.ID {
		t.Fatalf("due intent did not resume: %+v %v", got, err)
	}
}
func TestAmbiguousReservationRecovery(t *testing.T) {
	l := newWorkflowLab(t)
	resource := l.resource(t, 2, "ambiguous-supply")
	request, d := l.request(t, 0, "ambiguous-request")
	l.loseReserve.Store(true)
	state, err := l.router.HandleDelivery(context.Background(), d)
	if err != nil || !state.Ack || state.ExpectedRetry != 1 {
		t.Fatalf("scheduled uncertain attempt %+v %v", state, err)
	}
	got, _ := l.router.Get(context.Background(), request.ID)
	if got.Status != "ASSIGNING" {
		t.Fatal(got)
	}
	allocated, _ := l.router.GetResource(context.Background(), resource.ID)
	if allocated.AssignedRequestID != request.ID {
		t.Fatal(allocated)
	}
	l.resource(t, 1, "second-candidate")
	if _, err = l.pools[0].Exec(context.Background(), `UPDATE processed_events SET due_at=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	l.restart()
	if err = l.router.ReconcileOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = l.router.Get(context.Background(), request.ID)
	if err != nil || got.Status != "ASSIGNED" || got.AssignedResourceID != resource.ID {
		t.Fatalf("reconcile changed candidate %+v %v", got, err)
	}
}

func TestPausedWorkflowStopsReconciliationAndRedriveResumesOriginalIntent(t *testing.T) {
	l := newWorkflowLab(t)
	resource := l.resource(t, 2, "paused-supply")
	request, d := l.request(t, 0, "paused-request")
	l.loseReserve.Store(true)
	if _, err := l.router.HandleDelivery(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	d.RetryNumber = 1
	d.DeliveryID = contracts.DeliveryID(d.Event.EventID, 1, "request.match")
	if _, err := l.pools[0].Exec(context.Background(), `UPDATE processed_events SET due_at=clock_timestamp()+interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	// A pending retry cannot execute early, but an operator can pause it now.
	if _, err := l.router.HandleDelivery(context.Background(), d); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatalf("future retry executed: %v", err)
	}
	state, err := l.router.Workflow(context.Background(), contracts.WorkflowCommand{Delivery: d, Action: "pause", Reason: "operator fixture"})
	if err != nil || state.State != "PAUSED" {
		t.Fatal(state, err)
	}
	l.restart()
	if err = l.router.ReconcileOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := l.router.Get(context.Background(), request.ID)
	if got.Status != "ASSIGNING" {
		t.Fatal("reconciler ignored pause", got)
	}
	if _, err = l.router.Workflow(context.Background(), contracts.WorkflowCommand{Delivery: d, Action: "redrive"}); err != nil {
		t.Fatal(err)
	}
	if err = l.router.ReconcileOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = l.router.Get(context.Background(), request.ID)
	if err != nil || got.Status != "ASSIGNED" || got.AssignedResourceID != resource.ID {
		t.Fatal(got, err)
	}
}
