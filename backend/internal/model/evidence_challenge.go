package model

import (
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
)

// EvidenceChallenge is a reviewer question about specific survey observations
// cited by a boundary proposal. Pending challenges block proposal acceptance;
// answered challenges and their original observations stay archived.
type EvidenceChallenge struct {
	ID             uint                     `gorm:"primaryKey" json:"id"`
	ProposalID     uint                     `gorm:"not null;uniqueIndex:idx_challenge_proposal_seq;index" json:"proposal_id"`
	Sequence       uint                     `gorm:"not null;uniqueIndex:idx_challenge_proposal_seq" json:"sequence"`
	ChallengeCode  string                   `gorm:"size:48;not null;uniqueIndex" json:"challenge_code"`
	ObservationIDs string                   `gorm:"type:text;not null" json:"observation_ids"`
	Question       string                   `gorm:"size:2000;not null" json:"question"`
	Response       string                   `gorm:"size:2000" json:"response"`
	ChallengeState constants.ChallengeState `gorm:"size:24;not null;index" json:"challenge_state"`
	RaisedBy       uint                     `gorm:"not null;uniqueIndex:idx_challenge_actor_key" json:"raised_by"`
	RespondedBy    *uint                    `json:"responded_by"`
	RespondedAt    *time.Time               `json:"responded_at"`
	IdempotencyKey string                   `gorm:"size:128;uniqueIndex:idx_challenge_actor_key" json:"-"`
	RequestHash    string                   `gorm:"size:128;not null" json:"-"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
}
