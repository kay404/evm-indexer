package indexer

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CursorStore tracks the last processed block per handler.
type CursorStore interface {
	// GetCursor returns the last processed block for the given handler name.
	GetCursor(ctx context.Context, name string) (block uint64, exists bool, err error)

	// UpsertCursor sets the last processed block for the given handler name.
	UpsertCursor(ctx context.Context, name string, block uint64) error
}

// scan_cursor table model — framework manages this table.
type scanCursor struct {
	Name                   string    `gorm:"column:name;primaryKey"`
	LastSafeBlockProcessed int64     `gorm:"column:last_safe_block_processed;not null"`
	CreatedAt              time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt              time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (scanCursor) TableName() string { return "scan_cursor" }

// PostgresCursorStore is the default CursorStore backed by PostgreSQL.
type PostgresCursorStore struct {
	db *gorm.DB
}

// NewPostgresCursorStore creates a cursor store using the given GORM database.
func NewPostgresCursorStore(db *gorm.DB) *PostgresCursorStore {
	return &PostgresCursorStore{db: db}
}

func (s *PostgresCursorStore) GetCursor(ctx context.Context, name string) (uint64, bool, error) {
	var rec scanCursor
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return uint64(rec.LastSafeBlockProcessed), true, nil
}

func (s *PostgresCursorStore) UpsertCursor(ctx context.Context, name string, block uint64) error {
	now := time.Now().UTC()
	rec := &scanCursor{
		Name:                   name,
		LastSafeBlockProcessed: int64(block),
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}},
			DoUpdates: clause.Assignments(map[string]any{
				"last_safe_block_processed": rec.LastSafeBlockProcessed,
				"updated_at":               rec.UpdatedAt,
			}),
		}).
		Create(rec).Error
}
