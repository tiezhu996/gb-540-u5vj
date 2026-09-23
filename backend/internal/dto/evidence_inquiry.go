package dto

type EvidenceInquiryQuery struct {
	ProposalID *uint
	Status     string
	AuthorID   *uint
	Page       int
	PageSize   int
}

type CreateEvidenceInquiryRequest struct {
	Question       string `json:"question" validate:"required,min=4,max=1000"`
	ObservationIDs []uint `json:"observation_ids" validate:"required,min=1,max=100,dive,gt=0"`
}

type AnswerEvidenceInquiryRequest struct {
	Response string `json:"response" validate:"required,min=4,max=2000"`
	Version  uint   `json:"version" validate:"required,gt=0"`
}

type EvidenceInquiryObservationView struct {
	ID                  uint    `json:"id"`
	ObservationCode     string  `json:"observation_code"`
	PointGeoJSON        string  `json:"point_geojson"`
	ObservedAt          string  `json:"observed_at"`
	Method              string  `json:"method"`
	HorizontalAccuracyM float64 `json:"horizontal_accuracy_m"`
	ObservationState    string  `json:"observation_state"`
	QualityNote         string  `json:"quality_note"`
}

type EvidenceInquiryView struct {
	ID              uint                             `json:"id"`
	InquiryNumber   string                           `json:"inquiry_number"`
	ProposalID      uint                             `json:"proposal_id"`
	ObservationIDs  []uint                           `json:"observation_ids"`
	Observations    []EvidenceInquiryObservationView `json:"observations"`
	Question        string                           `json:"question"`
	Response        *string                          `json:"response"`
	Status          string                           `json:"status"`
	Version         uint                             `json:"version"`
	CreatedBy       uint                             `json:"created_by"`
	CreatedByName   string                           `json:"created_by_name"`
	RespondedBy     *uint                            `json:"responded_by"`
	RespondedByName string                           `json:"responded_by_name"`
	RespondedAt     *string                          `json:"responded_at"`
	CreatedAt       string                           `json:"created_at"`
	UpdatedAt       string                           `json:"updated_at"`
}
