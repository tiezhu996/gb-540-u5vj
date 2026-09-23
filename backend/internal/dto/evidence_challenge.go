package dto

type ChallengeQuery struct {
	ProposalID *uint
	State      string
	Page       int
	PageSize   int
}

type CreateChallengeRequest struct {
	ObservationIDs []uint `json:"observation_ids" validate:"required,min=1,max=100,dive,gt=0"`
	Question       string `json:"question" validate:"required,min=1,max=2000"`
}

type ChallengeResponseRequest struct {
	Response string `json:"response" validate:"required,min=1,max=2000"`
}
