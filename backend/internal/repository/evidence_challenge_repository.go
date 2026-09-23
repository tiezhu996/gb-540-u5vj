package repository

import (
	"errors"
	"fmt"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"gorm.io/gorm"
)

// EvidenceChallengeRepository persists reviewer questions about proposal
// observations and the author responses that resolve them.
type EvidenceChallengeRepository struct{ db *gorm.DB }

func (r *EvidenceChallengeRepository) Create(item *model.EvidenceChallenge) error {
	if err := r.db.Create(item).Error; err != nil {
		return fmt.Errorf("create evidence challenge: %w", err)
	}
	return nil
}

func (r *EvidenceChallengeRepository) Get(id uint) (model.EvidenceChallenge, error) {
	var item model.EvidenceChallenge
	if err := r.db.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, ErrNotFound
		}
		return item, fmt.Errorf("get evidence challenge: %w", err)
	}
	return item, nil
}

func (r *EvidenceChallengeRepository) GetByActorKey(actorID uint, key string) (model.EvidenceChallenge, error) {
	var item model.EvidenceChallenge
	if err := r.db.Where("raised_by = ? AND idempotency_key = ?", actorID, key).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, ErrNotFound
		}
		return item, fmt.Errorf("find challenge idempotency key: %w", err)
	}
	return item, nil
}

func (r *EvidenceChallengeRepository) List(q dto.ChallengeQuery) ([]model.EvidenceChallenge, int64, error) {
	db := r.db.Model(&model.EvidenceChallenge{})
	if q.ProposalID != nil {
		db = db.Where("proposal_id = ?", *q.ProposalID)
	}
	if q.State != "" {
		db = db.Where("challenge_state = ?", q.State)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count evidence challenges: %w", err)
	}
	var items []model.EvidenceChallenge
	if err := db.Order("proposal_id ASC, sequence ASC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list evidence challenges: %w", err)
	}
	return items, total, nil
}

// NextSequence returns the next per-proposal challenge sequence inside the
// caller's transaction so concurrent raises cannot collide.
func (r *EvidenceChallengeRepository) NextSequence(proposalID uint) (uint, error) {
	var count int64
	if err := r.db.Model(&model.EvidenceChallenge{}).Where("proposal_id = ?", proposalID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count proposal challenges: %w", err)
	}
	return uint(count) + 1, nil
}

// Respond applies the author answer with a conditional update: a challenge
// can only be answered once, and only while still pending.
func (r *EvidenceChallengeRepository) Respond(id uint, response string, respondedBy uint, respondedAt time.Time) error {
	result := r.db.Model(&model.EvidenceChallenge{}).
		Where("id = ? AND challenge_state = ?", id, "pending").
		Updates(map[string]any{
			"response":        response,
			"challenge_state": "answered",
			"responded_by":    respondedBy,
			"responded_at":    respondedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("respond evidence challenge: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("evidence challenge already answered: %w", gorm.ErrInvalidTransaction)
	}
	return nil
}
