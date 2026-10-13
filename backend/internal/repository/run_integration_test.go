//go:build integration

package repository

import (
	"reflect"
	"sync"
	"testing"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateRunCreatesQueuedRun(t *testing.T) {
	pool, models := setupTestDB(t)
	ctx := integrationContext(t)
	input := validCreateRunInput()
	deliveryID := "delivery-1"
	input.DeliveryID = &deliveryID

	got, err := models.Runs.CreateRun(ctx, input)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if got.ID <= 0 || got.CreatedAt.IsZero() {
		t.Fatalf("expected generated ID and creation time, got %+v", got)
	}
	if got.Repository != input.Repository || got.Branch != input.Branch || got.CommitSHA != input.CommitSHA {
		t.Fatalf("run fields do not match input: got %+v, input %+v", got, input)
	}
	if got.Status != domain.RunStatusQueued || got.ActiveAttemptID != nil || got.CancelRequestedAt != nil {
		t.Fatalf("unexpected initial run state: %+v", got)
	}

	var stored domain.Run
	var storedDeliveryID *string
	err = pool.QueryRow(ctx, `
		SELECT id, repository, branch, commit_sha, status,
		       active_attempt_id, cancel_requested_at, created_at, webhook_delivery_id
		FROM runs WHERE id = $1`, got.ID).Scan(
		&stored.ID, &stored.Repository, &stored.Branch, &stored.CommitSHA, &stored.Status,
		&stored.ActiveAttemptID, &stored.CancelRequestedAt, &stored.CreatedAt, &storedDeliveryID,
	)
	if err != nil {
		t.Fatalf("read stored run: %v", err)
	}
	assertSameRun(t, got, stored)
	if storedDeliveryID == nil || *storedDeliveryID != deliveryID {
		t.Fatalf("stored delivery ID = %v, want %q", storedDeliveryID, deliveryID)
	}
	assertRunCount(t, pool, 1)
}

func TestCreateRunWithoutDeliveryIDCreatesDistinctRuns(t *testing.T) {
	pool, models := setupTestDB(t)
	ctx := integrationContext(t)
	input := validCreateRunInput()

	first, err := models.Runs.CreateRun(ctx, input)
	if err != nil {
		t.Fatalf("first CreateRun: %v", err)
	}
	second, err := models.Runs.CreateRun(ctx, input)
	if err != nil {
		t.Fatalf("second CreateRun: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("manual runs share ID %d", first.ID)
	}
	assertRunCount(t, pool, 2)
	var withoutDelivery int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM runs WHERE webhook_delivery_id IS NULL").Scan(&withoutDelivery); err != nil {
		t.Fatal(err)
	}
	if withoutDelivery != 2 {
		t.Fatalf("runs with NULL delivery ID = %d, want 2", withoutDelivery)
	}
}

func TestCreateRunDuplicateDeliveryReturnsExistingRun(t *testing.T) {
	pool, models := setupTestDB(t)
	ctx := integrationContext(t)
	input := validCreateRunInput()
	deliveryID := "duplicate-delivery"
	input.DeliveryID = &deliveryID

	first, err := models.Runs.CreateRun(ctx, input)
	if err != nil {
		t.Fatalf("first CreateRun: %v", err)
	}
	second, err := models.Runs.CreateRun(ctx, input)
	if err != nil {
		t.Fatalf("duplicate CreateRun: %v", err)
	}
	assertSameRun(t, second, first)
	assertRunCount(t, pool, 1)
}

func TestCreateRunDuplicateDeliveryPreservesExistingState(t *testing.T) {
	pool, models := setupTestDB(t)
	ctx := integrationContext(t)
	input := validCreateRunInput()
	deliveryID := "cancelled-delivery"
	input.DeliveryID = &deliveryID

	original, err := models.Runs.CreateRun(ctx, input)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	err = pool.QueryRow(ctx, `
		UPDATE runs SET status = 'cancelled', cancel_requested_at = now()
		WHERE id = $1 RETURNING cancel_requested_at`, original.ID).Scan(&original.CancelRequestedAt)
	if err != nil {
		t.Fatalf("cancel stored run: %v", err)
	}
	original.Status = domain.RunStatusCancelled

	// Repeated deliveries must not overwrite the original request or its state.
	input.Branch = "another-branch"
	input.CommitSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	got, err := models.Runs.CreateRun(ctx, input)
	if err != nil {
		t.Fatalf("duplicate CreateRun: %v", err)
	}
	assertSameRun(t, got, original)
	assertRunCount(t, pool, 1)
	var status domain.RunStatus
	if err := pool.QueryRow(ctx, "SELECT status FROM runs WHERE id = $1", original.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != domain.RunStatusCancelled {
		t.Fatalf("stored status = %q, want cancelled", status)
	}
}

func TestCreateRunDifferentDeliveriesCreateDistinctRuns(t *testing.T) {
	pool, models := setupTestDB(t)
	ctx := integrationContext(t)
	input := validCreateRunInput()
	firstDelivery, secondDelivery := "delivery-1", "delivery-2"
	input.DeliveryID = &firstDelivery
	first, err := models.Runs.CreateRun(ctx, input)
	if err != nil {
		t.Fatalf("first CreateRun: %v", err)
	}
	input.DeliveryID = &secondDelivery
	second, err := models.Runs.CreateRun(ctx, input)
	if err != nil {
		t.Fatalf("second CreateRun: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("different deliveries share run ID %d", first.ID)
	}
	assertRunCount(t, pool, 2)
}

func TestCreateRunConcurrentDuplicateDeliveries(t *testing.T) {
	pool, models := setupTestDB(t)
	ctx := integrationContext(t)
	input := validCreateRunInput()
	deliveryID := "concurrent-delivery"
	input.DeliveryID = &deliveryID

	const callers = 8
	type result struct {
		run domain.Run
		err error
	}
	start := make(chan struct{})
	results := make(chan result, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			run, err := models.Runs.CreateRun(ctx, input)
			results <- result{run: run, err: err}
		}()
	}
	ready.Wait()
	close(start)

	// Collect all results before asserting so no query outlives test cleanup.
	got := make([]result, callers)
	for i := range got {
		got[i] = <-results
	}
	for i, result := range got {
		if result.err != nil {
			t.Fatalf("caller %d: %v", i, result.err)
		}
	}
	for _, result := range got {
		assertSameRun(t, result.run, got[0].run)
	}
	assertRunCount(t, pool, 1)
	var matching int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM runs WHERE webhook_delivery_id = $1", deliveryID).Scan(&matching); err != nil {
		t.Fatal(err)
	}
	if matching != 1 {
		t.Fatalf("runs for delivery = %d, want 1", matching)
	}
}

func validCreateRunInput() CreateRunInput {
	return CreateRunInput{
		Repository: "https://github.com/example/project",
		Branch:     "main",
		CommitSHA:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
}

func assertSameRun(t *testing.T, got, want domain.Run) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("run = %+v, want %+v", got, want)
	}
}

func assertRunCount(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(integrationContext(t), "SELECT count(*) FROM runs").Scan(&got); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if got != want {
		t.Fatalf("run count = %d, want %d", got, want)
	}
}
