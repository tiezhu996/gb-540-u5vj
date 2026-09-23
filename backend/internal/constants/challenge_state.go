package constants

// ChallengeState tracks whether an evidence challenge still blocks proposal
// acceptance. Answered challenges are retained verbatim as review evidence.
type ChallengeState string

const (
	ChallengePending  ChallengeState = "pending"
	ChallengeAnswered ChallengeState = "answered"
)

func (s ChallengeState) Valid() bool {
	return s == ChallengePending || s == ChallengeAnswered
}
