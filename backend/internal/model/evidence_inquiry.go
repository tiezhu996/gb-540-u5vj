package model

import (
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
)

// EvidenceInquiryObservation is an immutable snapshot of evidence named in a
// review question. Keeping it with the inquiry preserves the original context
// even if the observation is later superseded.
type EvidenceInquiryObservation struct {
	ID                  uint    `json:"id"`
	ObservationCode     string  `json:"observation_code"`
	PointGeoJSON        string  `json:"point_geojson"`
	ObservedAt          string  `json:"observed_at"`
	Method              string  `json:"method"`
	HorizontalAccuracyM float64 `json:"horizontal_accuracy_m"`
	ObservationState    string  `json:"observation_state"`
	QualityNote         string  `json:"quality_note"`
}

// EvidenceInquiry asks a proposal author to explain selected survey evidence
// before a reviewer may accept the proposal.
type EvidenceInquiry struct {
	ID                   uint                           `gorm:"primaryKey" json:"id"`
	ProposalID           uint                           `gorm:"not null;index" json:"proposal_id"`
	ObservationIDs       string                         `gorm:"type:text;not null" json:"observation_ids"`
	ObservationSnapshots string                         `gorm:"column:observation_snapshots;type:text;not null" json:"-"`
	Question             string                         `gorm:"size:1000;not null" json:"question"`
	Response             *string                        `gorm:"size:2000" json:"response"`
	Status               constants.EvidenceInquiryState `gorm:"size:24;not null;default:pending;index" json:"status"`
	Version              uint                           `gorm:"not null;default:1" json:"version"`
	CreatedBy            uint                           `gorm:"not null;uniqueIndex:idx_inquiry_actor_key,priority:1" json:"created_by"`
	RespondedBy          *uint                          `json:"responded_by"`
	RespondedAt          *time.Time                     `json:"responded_at"`
	IdempotencyKey       string                         `gorm:"size:128;not null;index:idx_inquiry_actor_key,priority:2" json:"-"`
	CreatedAt            time.Time                      `json:"created_at"`
	UpdatedAt            time.Time                      `json:"updated_at"`
}
