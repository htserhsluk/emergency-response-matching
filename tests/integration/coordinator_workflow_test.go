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
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = l.router.ReconcileOnce(bounded)
	got, err := l.router.Get(ctx, request.ID)
	if err != nil || got.Status != "ASSIGNED" {
		t.Fatalf("healthy work starved %+v %v", got, err)
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
	if _, err = l.router.RegisterResource(ctx, keyOnShard("register-resource", 2, "too-far"), resourceBody(0.2, 0, true)); err != nil {
		t.Fatal(err)
	}
	state, err = l.router.HandleDelivery(ctx, second)
	if err != nil || state.Request.Status != "UNMATCHED" {
		t.Fatalf("outside radius matched %+v %v", state, err)
	}
}
func (l *workflowLab) restart() {
	l.router = coordinator.NewRouter(l.urls, "workflow-secret-at-least-32-characters")
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
			if err != nil || !state.Ack {
				t.Errorf("handle %+v %v", state, err)
			}
			if _, err = l.router.ScheduleMatch(context.Background(), d.Event.RequestID, fmt.Sprintf("manual-race-%016d", i)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	ar, _ := l.router.Get(context.Background(), a.ID)
	br, _ := l.router.Get(context.Background(), b.ID)
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
