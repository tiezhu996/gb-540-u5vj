package service

import (
	"errors"
	"strings"
	"testing"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"time"
)

func moveProposalToReviewed(t *testing.T, svc *CadastralService, proposal model.BoundaryProposal, author, reviewer testActorPair) model.BoundaryProposal {
	t.Helper()
	current := proposal
	for _, to := range []constants.ProposalState{constants.ProposalValidated, constants.ProposalSubmitted, constants.ProposalReviewed} {
		actor := author.actor
		if to == constants.ProposalReviewed {
			actor = reviewer.actor
		}
		moved, err := svc.TransitionProposal(current.ID, dto.ProposalTransitionRequest{To: string(to), Version: current.Version}, actor)
		if err != nil {
			t.Fatalf("transition proposal to %s: %v", to, err)
		}
		current = moved
	}
	return current
}

type testActorPair struct{ actor Actor }

func TestEvidenceChallengeBlocksAcceptanceUntilAnswered(t *testing.T) {
	svc, store := newCadastralTestService(t)
	author := testActorPair{testActor(301, constants.RoleSurveyor, "challenge-author")}
	reviewer := testActorPair{testActor(302, constants.RoleReviewer, "challenge-reviewer")}
	parcel := createTestParcel(t, svc, "P-CHALLENGE", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), author.actor)
	observation, err := svc.ImportObservation(dto.ImportObservationRequest{
		ParcelID: parcel.ID, ObservationCode: "OBS-CHALLENGE", PointGeoJSON: `{"type":"Point","coordinates":[5,5]}`,
		ObservedAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC), Method: "rtk_gnss", HorizontalAccuracyM: 0.03, SourceChecksum: "checksum-challenge",
	}, author.actor)
	if err != nil {
		t.Fatalf("import challenged observation: %v", err)
	}
	proposal, err := svc.CreateProposal(dto.CreateProposalRequest{
		ParcelID: parcel.ID, BaseVersion: parcel.BoundaryVersion, ProposedGeoJSON: serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`),
		ObservationIDs: []uint{observation.ID}, SnapToleranceM: 0.1, Rationale: "cited observation anchors the boundary",
	}, author.actor)
	if err != nil {
		t.Fatalf("create proposal with observation: %v", err)
	}
	reviewed := moveProposalToReviewed(t, svc, proposal, author, reviewer)

	challenge, err := svc.CreateEvidenceChallenge(proposal.ID, dto.CreateChallengeRequest{
		ObservationIDs: []uint{observation.ID}, Question: "该控制点的水平精度是否足以支撑改线？请补充观测时段与校核方式。",
	}, "challenge-idem-key-1", reviewer.actor)
	if err != nil {
		t.Fatalf("raise evidence challenge: %v", err)
	}
	if challenge.ChallengeState != constants.ChallengePending || challenge.ChallengeCode != "EC-1-1" {
		t.Fatalf("created challenge = %#v, want pending EC-1-1", challenge)
	}
	if challenge.RaisedBy != reviewer.actor.ID || challenge.RespondedBy != nil {
		t.Fatalf("challenge attribution = %#v, want reviewer and no respondent", challenge)
	}

	// Acceptance stays closed while the challenge is pending.
	_, err = svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalAccepted), Version: reviewed.Version}, reviewer.actor)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("accept with pending challenge error = %v, want 409 %s", err, CodeConflict)
	}
	if err == nil || !strings.Contains(appErr.Error(), "EC-1-1") {
		t.Fatalf("acceptance error = %v, want it to cite challenge code EC-1-1", err)
	}

	// A surveyor who is not the author cannot answer; the author can.
	_, err = svc.RespondEvidenceChallenge(challenge.ID, dto.ChallengeResponseRequest{Response: "not mine"}, testActor(303, constants.RoleSurveyor, "other-surveyor"))
	if !errors.As(err, &appErr) || appErr.Code != CodeForbidden {
		t.Fatalf("non-author response error = %v, want 403", err)
	}
	answered, err := svc.RespondEvidenceChallenge(challenge.ID, dto.ChallengeResponseRequest{Response: "已补充：该点使用 RTK 固定解，观测 30 分钟并有两次闭合校核。"}, author.actor)
	if err != nil {
		t.Fatalf("author responds to challenge: %v", err)
	}
	if answered.ChallengeState != constants.ChallengeAnswered || answered.RespondedBy == nil || *answered.RespondedBy != author.actor.ID {
		t.Fatalf("answered challenge = %#v, want answered by author", answered)
	}

	// The original question and observation references stay archived.
	persisted, err := store.Challenges.Get(challenge.ID)
	if err != nil {
		t.Fatalf("reload challenge: %v", err)
	}
	if persisted.Question == "" || persisted.ObservationIDs == "" || persisted.Response == "" {
		t.Fatalf("archived challenge lost evidence: %#v", persisted)
	}

	// Answering twice does not create a second pending record.
	_, err = svc.RespondEvidenceChallenge(challenge.ID, dto.ChallengeResponseRequest{Response: "重复补充"}, author.actor)
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict {
		t.Fatalf("duplicate response error = %v, want 409", err)
	}
	var pendingCount int64
	if err := store.DB.Model(&model.EvidenceChallenge{}).Where("challenge_state = ?", string(constants.ChallengePending)).Count(&pendingCount).Error; err != nil {
		t.Fatalf("count pending challenges: %v", err)
	}
	if pendingCount != 0 {
		t.Fatalf("pending challenge count = %d, want 0", pendingCount)
	}

	// Acceptance reopens after every cited question is answered.
	accepted, err := svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalAccepted), Version: reviewed.Version}, reviewer.actor)
	if err != nil {
		t.Fatalf("accept after challenge answered: %v", err)
	}
	if accepted.ProposalState != constants.ProposalAccepted {
		t.Fatalf("accepted proposal = %#v", accepted)
	}
}

// TestEvidenceChallengeCreateIsIdempotentForRepeatedRequests verifies that
// repeated clicks on the raise action replay one stored challenge record.

func TestEvidenceChallengeCreateIsIdempotentForRepeatedRequests(t *testing.T) {
	svc, store := newCadastralTestService(t)
	author := testActorPair{testActor(311, constants.RoleGISAnalyst, "idem-author")}
	reviewer := testActorPair{testActor(312, constants.RoleReviewer, "idem-reviewer")}
	parcel := createTestParcel(t, svc, "P-IDEM", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), author.actor)
	firstObservation, err := svc.ImportObservation(dto.ImportObservationRequest{
		ParcelID: parcel.ID, ObservationCode: "OBS-IDEM-1", PointGeoJSON: `{"type":"Point","coordinates":[1,1]}`,
		ObservedAt: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC), Method: "total_station", HorizontalAccuracyM: 0.02, SourceChecksum: "checksum-idem-1",
	}, author.actor)
	if err != nil {
		t.Fatalf("import observation one: %v", err)
	}
	proposal, err := svc.CreateProposal(dto.CreateProposalRequest{
		ParcelID: parcel.ID, BaseVersion: parcel.BoundaryVersion, ProposedGeoJSON: serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`),
		ObservationIDs: []uint{firstObservation.ID}, SnapToleranceM: 0.2, Rationale: "idempotency fixture",
	}, author.actor)
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	moveProposalToReviewed(t, svc, proposal, author, reviewer)

	request := dto.CreateChallengeRequest{ObservationIDs: []uint{firstObservation.ID}, Question: "请说明该观测的闭合差。"}
	first, err := svc.CreateEvidenceChallenge(proposal.ID, request, "idem-challenge-key", reviewer.actor)
	if err != nil {
		t.Fatalf("first challenge: %v", err)
	}
	replay, err := svc.CreateEvidenceChallenge(proposal.ID, request, "idem-challenge-key", testActor(312, constants.RoleReviewer, "idem-reviewer-replay"))
	if err != nil {
		t.Fatalf("replayed challenge: %v", err)
	}
	if replay.ID != first.ID || replay.ChallengeCode != first.ChallengeCode {
		t.Fatalf("replay = %#v, first = %#v, want same record", replay, first)
	}
	var count int64
	if err := store.DB.Model(&model.EvidenceChallenge{}).Count(&count).Error; err != nil {
		t.Fatalf("count challenges: %v", err)
	}
	if count != 1 {
		t.Fatalf("challenge count = %d, want 1 after duplicate click", count)
	}

	// Reusing the key with a different question is a conflict.
	_, err = svc.CreateEvidenceChallenge(proposal.ID, dto.CreateChallengeRequest{ObservationIDs: []uint{firstObservation.ID}, Question: "另一条不同的疑问？"}, "idem-challenge-key", reviewer.actor)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("key reuse with different request error = %v, want 409", err)
	}
}

func TestEvidenceChallengeRejectsInvalidActorsAndEvidence(t *testing.T) {
	svc, _ := newCadastralTestService(t)
	author := testActorPair{testActor(321, constants.RoleSurveyor, "invalid-author")}
	reviewer := testActorPair{testActor(322, constants.RoleReviewer, "invalid-reviewer")}
	parcel := createTestParcel(t, svc, "P-INVALID", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), author.actor)
	cited, err := svc.ImportObservation(dto.ImportObservationRequest{
		ParcelID: parcel.ID, ObservationCode: "OBS-CITED", PointGeoJSON: `{"type":"Point","coordinates":[2,2]}`,
		ObservedAt: time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC), Method: "total_station", HorizontalAccuracyM: 0.02, SourceChecksum: "checksum-cited",
	}, author.actor)
	if err != nil {
		t.Fatalf("import cited observation: %v", err)
	}
	other := createTestParcel(t, svc, "P-OTHER", serviceTestPolygon(`[20,0],[30,0],[30,10],[20,10],[20,0]`), author.actor)
	foreign, err := svc.ImportObservation(dto.ImportObservationRequest{
		ParcelID: other.ID, ObservationCode: "OBS-FOREIGN", PointGeoJSON: `{"type":"Point","coordinates":[21,1]}`,
		ObservedAt: time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC), Method: "total_station", HorizontalAccuracyM: 0.02, SourceChecksum: "checksum-foreign",
	}, author.actor)
	if err != nil {
		t.Fatalf("import foreign observation: %v", err)
	}
	proposal, err := svc.CreateProposal(dto.CreateProposalRequest{
		ParcelID: parcel.ID, BaseVersion: parcel.BoundaryVersion, ProposedGeoJSON: serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`),
		ObservationIDs: []uint{cited.ID}, SnapToleranceM: 0.2, Rationale: "validation fixture",
	}, author.actor)
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	moveProposalToReviewed(t, svc, proposal, author, reviewer)

	// Only reviewer/admin can challenge.
	_, err = svc.CreateEvidenceChallenge(proposal.ID, dto.CreateChallengeRequest{ObservationIDs: []uint{cited.ID}, Question: "疑问？"}, "key-surveyor", testActor(323, constants.RoleSurveyor, "surveyor-challenge"))
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeForbidden {
		t.Fatalf("surveyor challenge error = %v, want 403", err)
	}
	// Reviewer cannot challenge observations the proposal does not cite.
	_, err = svc.CreateEvidenceChallenge(proposal.ID, dto.CreateChallengeRequest{ObservationIDs: []uint{foreign.ID}, Question: "外地块观测？"}, "key-foreign", reviewer.actor)
	if !errors.As(err, &appErr) || appErr.Code != CodeInvalidInput {
		t.Fatalf("foreign observation challenge error = %v, want invalid input", err)
	}
	// Challenges are only available at the review desk.
	draft, err := svc.CreateProposal(dto.CreateProposalRequest{
		ParcelID: parcel.ID, BaseVersion: parcel.BoundaryVersion, ProposedGeoJSON: serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`),
		ObservationIDs: []uint{cited.ID}, SnapToleranceM: 0.2, Rationale: "draft fixture",
	}, author.actor)
	if err != nil {
		t.Fatalf("create draft proposal: %v", err)
	}
	_, err = svc.CreateEvidenceChallenge(draft.ID, dto.CreateChallengeRequest{ObservationIDs: []uint{cited.ID}, Question: "草拟阶段质询？"}, "key-draft", reviewer.actor)
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict {
		t.Fatalf("draft challenge error = %v, want 409", err)
	}
}
