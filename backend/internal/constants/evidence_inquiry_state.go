package constants

type EvidenceInquiryState string

const (
	EvidenceInquiryPending  EvidenceInquiryState = "pending"
	EvidenceInquiryAnswered EvidenceInquiryState = "answered"
)

func (s EvidenceInquiryState) Valid() bool {
	switch s {
	case EvidenceInquiryPending, EvidenceInquiryAnswered:
		return true
	default:
		return false
	}
}
