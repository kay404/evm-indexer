package indexer

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CursorStore tracks the last processed block (and its canonical hash) per handler.
//
// blockHash is used for reorg detection. When the engine is not configured to verify
// reorgs (cfg.VerifyCursorHash == false), the engine passes common.Hash{} on writes
// and ignores hash on reads — backends should accept and return the zero value without
// special-casing.
type CursorStore interface {
	// GetCursor returns the last processed block and its recorded hash.
	// exists is false when the handler has no cursor yet; block and blockHash are zero in that case.
	GetCursor(ctx context.Context, name string) (block uint64, blockHash common.Hash, exists bool, err error)

	// UpsertCursor sets the last processed block and its hash for the given handler.
	// Pass common.Hash{} when hash tracking is disabled.
	UpsertCursor(ctx context.Context, name string, block uint64, blockHash common.Hash) error
}

// scan_cursor table model — framework manages this table.
type scanCursor struct {
	Name                   string    `gorm:"column:name;primaryKey"`
	LastSafeBlockProcessed int64     `gorm:"column:last_safe_block_processed;not null"`
	LastBlockHash          string    `gorm:"column:last_block_hash;not null;default:''"`
	CreatedAt              time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt              time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (scanCursor) TableName() string { return "scan_cursor" }

// GormCursorStore is the default CursorStore backed by any GORM-supported database.
type GormCursorStore struct {
	db *gorm.DB
}

// NewGormCursorStore creates a cursor store using the given GORM database.
func NewGormCursorStore(db *gorm.DB) *GormCursorStore {
	return &GormCursorStore{db: db}
}

func (s *GormCursorStore) GetCursor(ctx context.Context, name string) (uint64, common.Hash, bool, error) {
	var rec scanCursor
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, common.Hash{}, false, nil
		}
		return 0, common.Hash{}, false, err
	}
	var hash common.Hash
	if rec.LastBlockHash != "" {
		hash = common.HexToHash(rec.LastBlockHash)
	}
	return uint64(rec.LastSafeBlockProcessed), hash, true, nil
}

func (s *GormCursorStore) UpsertCursor(ctx context.Context, name string, block uint64, blockHash common.Hash) error {
	now := time.Now().UTC()
	hashStr := ""
	if blockHash != (common.Hash{}) {
		hashStr = blockHash.Hex()
	}
	rec := &scanCursor{
		Name:                   name,
		LastSafeBlockProcessed: int64(block),
		LastBlockHash:          hashStr,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}},
			DoUpdates: clause.Assignments(map[string]any{
				"last_safe_block_processed": rec.LastSafeBlockProcessed,
				"last_block_hash":           rec.LastBlockHash,
				"updated_at":                rec.UpdatedAt,
			}),
		}).
		Create(rec).Error
}
