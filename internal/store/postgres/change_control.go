package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

type ChangeControlQuerier interface {
	LoadChangeControlState(context.Context) (db.LoadChangeControlStateRow, error)
	SaveChangeControlState(context.Context, db.SaveChangeControlStateParams) (int64, error)
}

var (
	_ ChangeControlQuerier     = (*db.Queries)(nil)
	_ changecontrol.StateStore = (*Store)(nil)
)

func (s *Store) LoadChangeControlState(ctx context.Context) ([]byte, int64, error) {
	if s.changeControlQ == nil {
		return nil, 0, fmt.Errorf("change-control queries are unavailable")
	}
	row, err := s.changeControlQ.LoadChangeControlState(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return append([]byte(nil), row.StateJson...), row.Revision, nil
}

func (s *Store) SaveChangeControlState(ctx context.Context, expectedRevision int64, payload []byte) (int64, error) {
	if s.changeControlQ == nil {
		return 0, fmt.Errorf("change-control queries are unavailable")
	}
	if expectedRevision < 0 {
		return 0, fmt.Errorf("change-control revision must not be negative")
	}
	if len(payload) == 0 {
		return 0, fmt.Errorf("change-control state payload is required")
	}
	revision, err := s.changeControlQ.SaveChangeControlState(ctx, db.SaveChangeControlStateParams{StateJson: payload, ExpectedRevision: expectedRevision})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("change-control state revision conflict")
	}
	if err != nil {
		return 0, err
	}
	return revision, nil
}
