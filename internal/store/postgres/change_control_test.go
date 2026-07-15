package postgres

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func TestStorePersistsChangeControlStateWithRevisionCheck(t *testing.T) {
	queries := &recordingChangeControlQueries{}
	store := NewFromChangeControlQueries(queries)
	var _ changecontrol.StateStore = store

	payload := []byte(`{"version":1,"requests":{}}`)
	revision, err := store.SaveChangeControlState(context.Background(), 0, payload)
	if err != nil {
		t.Fatal(err)
	}
	if revision != 1 {
		t.Fatalf("revision = %d, want 1", revision)
	}
	got, gotRevision, err := store.LoadChangeControlState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotRevision != 1 || !bytes.Equal(got, payload) {
		t.Fatalf("loaded revision=%d payload=%s", gotRevision, got)
	}
	if _, err := store.SaveChangeControlState(context.Background(), 0, []byte(`{"version":1}`)); err == nil {
		t.Fatal("stale revision update succeeded")
	}
}

type recordingChangeControlQueries struct {
	payload  []byte
	revision int64
}

func (q *recordingChangeControlQueries) LoadChangeControlState(context.Context) (db.LoadChangeControlStateRow, error) {
	if q.revision == 0 {
		return db.LoadChangeControlStateRow{}, pgx.ErrNoRows
	}
	return db.LoadChangeControlStateRow{StateJson: append([]byte(nil), q.payload...), Revision: q.revision}, nil
}

func (q *recordingChangeControlQueries) SaveChangeControlState(_ context.Context, params db.SaveChangeControlStateParams) (int64, error) {
	if params.ExpectedRevision != q.revision {
		return 0, fmt.Errorf("stale revision")
	}
	q.revision++
	q.payload = append([]byte(nil), params.StateJson...)
	return q.revision, nil
}
