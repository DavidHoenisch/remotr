package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func TestDiagnosticStoreValidatesIdentifiersAndPropagatesCreateFailures(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	sentinel := errors.New("diagnostic persistence unavailable")

	store := NewFromQueries(&fakeQuerier{})
	if _, err := store.CreateDiagnosticRequest(t.Context(), "bad endpoint id", "operator", diagnostics.Spec{}); err == nil {
		t.Fatal("invalid endpoint ID was accepted")
	}
	if _, _, err := store.GetDiagnosticRequest(t.Context(), "not-a-uuid"); err == nil {
		t.Fatal("invalid request ID was accepted by GetDiagnosticRequest")
	}
	if _, _, err := store.PendingDiagnosticForEndpoint(t.Context(), "bad endpoint id"); err == nil {
		t.Fatal("invalid endpoint ID was accepted by PendingDiagnosticForEndpoint")
	}
	if err := store.MarkDiagnosticDispatched(t.Context(), "not-a-uuid"); err == nil {
		t.Fatal("invalid request ID was accepted by MarkDiagnosticDispatched")
	}
	if err := store.MarkDiagnosticRunning(t.Context(), "not-a-uuid"); err == nil {
		t.Fatal("invalid request ID was accepted by MarkDiagnosticRunning")
	}

	endpoint := db.Endpoint{ID: endpointID, Fleet: "test"}
	tests := []struct {
		name    string
		querier *fakeQuerier
		spec    diagnostics.Spec
		wantErr error
	}{
		{
			name: "active request",
			querier: &fakeQuerier{
				byID: map[string]db.Endpoint{endpointID: endpoint}, diagnosticActive: true,
			},
			wantErr: diagnostics.ErrActiveRequest,
		},
		{
			name: "active lookup failure",
			querier: &fakeQuerier{
				byID: map[string]db.Endpoint{endpointID: endpoint}, diagnosticActiveErr: sentinel,
			},
			wantErr: sentinel,
		},
		{
			name: "unencodable specification",
			querier: &fakeQuerier{
				byID: map[string]db.Endpoint{endpointID: endpoint},
			},
			spec: diagnostics.Spec{Since: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name: "insert failure",
			querier: &fakeQuerier{
				byID: map[string]db.Endpoint{endpointID: endpoint}, diagnosticInsertErr: sentinel,
			},
			wantErr: sentinel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFromQueries(tt.querier).CreateDiagnosticRequest(t.Context(), endpointID, "operator", tt.spec)
			if err == nil {
				t.Fatal("CreateDiagnosticRequest returned no error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	row := diagnosticContractRow()
	created, err := NewFromQueries(&fakeQuerier{
		byID: map[string]db.Endpoint{endpointID: endpoint}, diagnosticInsertRow: row,
	}).CreateDiagnosticRequest(t.Context(), endpointID, "operator", diagnostics.Spec{})
	if err != nil || created.ID != uuid.UUID(row.ID.Bytes).String() {
		t.Fatalf("created request = %+v, err=%v", created, err)
	}
}

func TestDiagnosticStoreDistinguishesMissingRowsCorruptionAndPersistenceFailures(t *testing.T) {
	const (
		endpointID = "11111111-1111-1111-1111-111111111111"
		requestID  = "22222222-2222-2222-2222-222222222222"
	)
	sentinel := errors.New("diagnostic persistence unavailable")
	validRow := diagnosticContractRow()
	invalidRow := validRow
	invalidRow.SpecJson = []byte(`{"collectors":`)

	for _, tt := range []struct {
		name    string
		querier *fakeQuerier
		wantOK  bool
		wantErr error
	}{
		{name: "missing", querier: &fakeQuerier{}},
		{name: "query failure", querier: &fakeQuerier{diagnosticGetErr: sentinel}, wantErr: sentinel},
		{name: "corrupt row", querier: &fakeQuerier{diagnosticGetSet: true, diagnosticGetRow: invalidRow}},
		{name: "valid row", querier: &fakeQuerier{diagnosticGetSet: true, diagnosticGetRow: validRow}, wantOK: true},
	} {
		t.Run("get "+tt.name, func(t *testing.T) {
			req, ok, err := NewFromQueries(tt.querier).GetDiagnosticRequest(t.Context(), requestID)
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tt.wantOK)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && tt.name != "corrupt row" && err != nil {
				t.Fatal(err)
			}
			if tt.name == "corrupt row" && err == nil {
				t.Fatal("corrupt persisted request was accepted")
			}
			if tt.wantOK && req.EndpointID != endpointID {
				t.Fatalf("request = %+v", req)
			}
		})
	}

	for _, tt := range []struct {
		name    string
		querier *fakeQuerier
		wantOK  bool
		wantErr error
	}{
		{name: "missing", querier: &fakeQuerier{}},
		{name: "query failure", querier: &fakeQuerier{diagnosticActiveErr: sentinel}, wantErr: sentinel},
		{name: "corrupt row", querier: &fakeQuerier{diagnosticActive: true, diagnosticGetRow: invalidRow}},
		{name: "valid row", querier: &fakeQuerier{diagnosticActive: true, diagnosticGetRow: validRow}, wantOK: true},
	} {
		t.Run("pending "+tt.name, func(t *testing.T) {
			req, ok, err := NewFromQueries(tt.querier).PendingDiagnosticForEndpoint(t.Context(), endpointID)
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tt.wantOK)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && tt.name != "corrupt row" && err != nil {
				t.Fatal(err)
			}
			if tt.name == "corrupt row" && err == nil {
				t.Fatal("corrupt persisted request was accepted")
			}
			if tt.wantOK && req.EndpointID != endpointID {
				t.Fatalf("request = %+v", req)
			}
		})
	}
}

func TestDiagnosticStoreStateTransitionsAndExpiryPreservePersistenceOutcomes(t *testing.T) {
	const requestID = "22222222-2222-2222-2222-222222222222"
	sentinel := errors.New("diagnostic persistence unavailable")

	for _, tt := range []struct {
		name string
		call func(*Store) error
		fq   *fakeQuerier
		want error
	}{
		{name: "dispatch missing is idempotent", call: func(s *Store) error { return s.MarkDiagnosticDispatched(t.Context(), requestID) }, fq: &fakeQuerier{diagnosticDispatchErr: pgx.ErrNoRows}},
		{name: "dispatch failure", call: func(s *Store) error { return s.MarkDiagnosticDispatched(t.Context(), requestID) }, fq: &fakeQuerier{diagnosticDispatchErr: sentinel}, want: sentinel},
		{name: "running missing is idempotent", call: func(s *Store) error { return s.MarkDiagnosticRunning(t.Context(), requestID) }, fq: &fakeQuerier{diagnosticRunningErr: pgx.ErrNoRows}},
		{name: "running failure", call: func(s *Store) error { return s.MarkDiagnosticRunning(t.Context(), requestID) }, fq: &fakeQuerier{diagnosticRunningErr: sentinel}, want: sentinel},
		{name: "expiry failure", call: func(s *Store) error { return s.ExpireDiagnosticRequests(t.Context()) }, fq: &fakeQuerier{diagnosticExpireErr: sentinel}, want: sentinel},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(NewFromQueries(tt.fq))
			if tt.want == nil && err != nil {
				t.Fatal(err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}

	if _, err := NewFromQueries(&fakeQuerier{diagnosticDeleteErr: sentinel}).DeleteExpiredDiagnosticRequests(t.Context()); !errors.Is(err, sentinel) {
		t.Fatalf("delete error = %v, want %v", err, sentinel)
	}
	invalid := diagnosticContractRow()
	invalid.SpecJson = []byte(`{"collectors":`)
	if _, err := NewFromQueries(&fakeQuerier{diagnosticDeleteRows: []db.DiagnosticRequest{invalid}}).DeleteExpiredDiagnosticRequests(t.Context()); err == nil {
		t.Fatal("corrupt expired request was returned")
	}
	rows := []db.DiagnosticRequest{diagnosticContractRow(), diagnosticContractRow()}
	requests, err := NewFromQueries(&fakeQuerier{diagnosticDeleteRows: rows}).DeleteExpiredDiagnosticRequests(t.Context())
	if err != nil || len(requests) != len(rows) {
		t.Fatalf("deleted requests = %+v, err=%v", requests, err)
	}
}

func diagnosticContractRow() db.DiagnosticRequest {
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	return db.DiagnosticRequest{
		ID: pgtype.UUID{Bytes: id, Valid: true}, EndpointID: "11111111-1111-1111-1111-111111111111",
		RequestedBy: "operator", Status: diagnostics.StatusPending, SpecJson: []byte(`{}`),
		S3Key: "diagnostics/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222.tar.gz",
	}
}
