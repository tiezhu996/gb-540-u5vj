package repository

import (
	"errors"
	"fmt"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"gorm.io/gorm"
)

// EvidenceInquiryRepository stores reviewer questions about proposal evidence.
type EvidenceInquiryRepository struct{ db *gorm.DB }

func EnsureEvidenceInquiryIndexes(db *gorm.DB) error {
	statement := `CREATE UNIQUE INDEX IF NOT EXISTS idx_evidence_inquiries_one_pending
		ON evidence_inquiries(proposal_id) WHERE status = 'pending'`
	if err := db.Exec(statement).Error; err != nil {
		return fmt.Errorf("create one-pending evidence inquiry index: %w", err)
	}
	return nil
}

func (r *EvidenceInquiryRepository) Create(item *model.EvidenceInquiry) error {
	if err := r.db.Create(item).Error; err != nil {
		return fmt.Errorf("create evidence inquiry: %w", err)
	}
	return nil
}

func (r *EvidenceInquiryRepository) Get(id uint) (model.EvidenceInquiry, error) {
	var item model.EvidenceInquiry
	if err := r.db.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, ErrNotFound
		}
		return item, fmt.Errorf("get evidence inquiry: %w", err)
	}
	return item, nil
}

func (r *EvidenceInquiryRepository) List(q dto.EvidenceInquiryQuery) ([]model.EvidenceInquiry, int64, error) {
	db := r.db.Model(&model.EvidenceInquiry{})
	if q.ProposalID != nil {
		db = db.Where("evidence_inquiries.proposal_id = ?", *q.ProposalID)
	}
	if q.Status != "" {
		db = db.Where("evidence_inquiries.status = ?", q.Status)
	}
	if q.AuthorID != nil {
		db = db.Joins("JOIN boundary_proposals ON boundary_proposals.id = evidence_inquiries.proposal_id").
			Where("boundary_proposals.created_by = ?", *q.AuthorID)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count evidence inquiries: %w", err)
	}
	var items []model.EvidenceInquiry
	if err := db.Order("evidence_inquiries.created_at DESC, evidence_inquiries.id DESC").
		Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list evidence inquiries: %w", err)
	}
	return items, total, nil
}

func (r *EvidenceInquiryRepository) FindPendingByProposal(proposalID uint) (model.EvidenceInquiry, bool, error) {
	var item model.EvidenceInquiry
	err := r.db.Where("proposal_id = ? AND status = ?", proposalID, constants.EvidenceInquiryPending).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, false, nil
	}
	if err != nil {
		return item, false, fmt.Errorf("find pending evidence inquiry: %w", err)
	}
	return item, true, nil
}

func (r *EvidenceInquiryRepository) CountPendingByProposal(proposalID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.EvidenceInquiry{}).Where("proposal_id = ? AND status = ?", proposalID, constants.EvidenceInquiryPending).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count pending evidence inquiries: %w", err)
	}
	return count, nil
}

func (r *EvidenceInquiryRepository) FindByCreatorAndKey(creatorID uint, key string) (model.EvidenceInquiry, bool, error) {
	var item model.EvidenceInquiry
	err := r.db.Where("created_by = ? AND idempotency_key = ?", creatorID, key).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, false, nil
	}
	if err != nil {
		return item, false, fmt.Errorf("find evidence inquiry idempotency record: %w", err)
	}
	return item, true, nil
}

func (r *EvidenceInquiryRepository) Answer(id, version uint, actorID uint, response string, respondedAt time.Time, updates map[string]any) error {
	if updates == nil {
		updates = map[string]any{}
	}
	updates["response"] = response
	updates["status"] = constants.EvidenceInquiryAnswered
	updates["responded_by"] = actorID
	updates["responded_at"] = respondedAt
	updates["version"] = gorm.Expr("version + 1")
	result := r.db.Model(&model.EvidenceInquiry{}).
		Where("id = ? AND version = ? AND status = ?", id, version, constants.EvidenceInquiryPending).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("answer evidence inquiry: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("evidence inquiry state or version changed: %w", gorm.ErrInvalidTransaction)
	}
	return nil
}
