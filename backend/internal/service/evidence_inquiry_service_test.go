package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
)

func createInquiryTestProposal(t *testing.T, svc *CadastralService, author Actor) (model.LandParcel, model.BoundaryProposal, model.SurveyObservation) {
	t.Helper()
	parcel := createTestParcel(t, svc, "P-INQUIRY", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), author)
	observation, err := svc.ImportObservation(dto.ImportObservationRequest{
		ParcelID: parcel.ID, ObservationCode: "OBS-INQUIRY", PointGeoJSON: `{"type":"Point","coordinates":[1.5,1.5]}`,
		ObservedAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), Method: "rtk_gps", HorizontalAccuracyM: 0.08, SourceChecksum: "checksum-inquiry",
	}, author)
	if err != nil {
		t.Fatalf("import inquiry observation: %v", err)
	}
	proposal, err := svc.CreateProposal(dto.CreateProposalRequest{
		ParcelID: parcel.ID, BaseVersion: parcel.BoundaryVersion, ProposedGeoJSON: serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`),
		ObservationIDs: []uint{observation.ID}, SnapToleranceM: 0.1, Rationale: "move the line based on the imported point",
	}, author)
	if err != nil {
		t.Fatalf("create inquiry proposal: %v", err)
	}
	return parcel, proposal, observation
}

func moveProposalToReviewed(t *testing.T, svc *CadastralService, proposal model.BoundaryProposal, authorID, reviewerID uint) model.BoundaryProposal {
	t.Helper()
	author := testActor(authorID, constants.RoleSurveyor, "inquiry-author-flow")
	validated, err := svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalValidated), Version: proposal.Version}, author)
	if err != nil {
		t.Fatalf("validate proposal: %v", err)
	}
	submitted, err := svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalSubmitted), Version: validated.Version}, author)
	if err != nil {
		t.Fatalf("submit proposal: %v", err)
	}
	reviewed, err := svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalReviewed), Version: submitted.Version}, testActor(reviewerID, constants.RoleReviewer, "inquiry-review-flow"))
	if err != nil {
		t.Fatalf("review proposal: %v", err)
	}
	return reviewed
}

func TestEvidenceInquiryLifecycleBlocksAcceptanceUntilAnswered(t *testing.T) {
	svc, store := newCadastralTestService(t)
	author := testActor(601, constants.RoleSurveyor, "inquiry-author")
	reviewer := testActor(602, constants.RoleReviewer, "inquiry-reviewer")
	_, proposal, observation := createInquiryTestProposal(t, svc, author)
	reviewed := moveProposalToReviewed(t, svc, proposal, author.ID, reviewer.ID)

	request := dto.CreateEvidenceInquiryRequest{
		Question:       "该点位精度和观测时间如何支撑北侧改线？",
		ObservationIDs: []uint{observation.ID},
	}
	inquiry, err := svc.CreateEvidenceInquiry(reviewed.ID, request, "inquiry-idempotency-key", reviewer)
	if err != nil {
		t.Fatalf("create evidence inquiry: %v", err)
	}
	if inquiry.Status != "pending" || inquiry.InquiryNumber != InquiryNumber(inquiry.ID) || len(inquiry.Observations) != 1 {
		t.Fatalf("created inquiry = %#v, want pending numbered inquiry with observation snapshot", inquiry)
	}
	if inquiry.Observations[0].ObservationCode != observation.ObservationCode || inquiry.Observations[0].PointGeoJSON != observation.PointGeoJSON {
		t.Fatalf("snapshot = %#v, want original observation evidence", inquiry.Observations[0])
	}
	replayed, err := svc.CreateEvidenceInquiry(reviewed.ID, request, "inquiry-idempotency-key", reviewer)
	if err != nil {
		t.Fatalf("replay evidence inquiry: %v", err)
	}
	if replayed.ID != inquiry.ID {
		t.Fatalf("replayed inquiry ID = %d, want %d", replayed.ID, inquiry.ID)
	}
	_, err = svc.CreateEvidenceInquiry(reviewed.ID, dto.CreateEvidenceInquiryRequest{
		Question:       "重复点击不应再产生待回应记录",
		ObservationIDs: []uint{observation.ID},
	}, "different-key", reviewer)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 || !strings.Contains(appErr.Message, inquiry.InquiryNumber) {
		t.Fatalf("second pending inquiry error = %v, want 409 citing %s", err, inquiry.InquiryNumber)
	}
	var pendingCount int64
	if err := store.DB.Model(&model.EvidenceInquiry{}).Count(&pendingCount).Error; err != nil {
		t.Fatalf("count inquiries: %v", err)
	}
	if pendingCount != 1 {
		t.Fatalf("inquiry count = %d, want one pending record after repeated clicks", pendingCount)
	}

	_, err = svc.TransitionProposal(reviewed.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalAccepted), Version: reviewed.Version}, reviewer)
	if !errors.As(err, &appErr) || appErr.Status != 409 || !strings.Contains(appErr.Message, inquiry.InquiryNumber) {
		t.Fatalf("acceptance with pending inquiry error = %v, want 409 citing %s", err, inquiry.InquiryNumber)
	}
	answered, err := svc.AnswerEvidenceInquiry(inquiry.ID, dto.AnswerEvidenceInquiryRequest{
		Response: "已补充控制点闭合差与重复观测结果，改线距离在标称精度两倍以内。", Version: inquiry.Version,
	}, author)
	if err != nil {
		t.Fatalf("answer evidence inquiry: %v", err)
	}
	if answered.Status != "answered" || answered.Response == nil || answered.RespondedBy == nil || *answered.RespondedBy != author.ID {
		t.Fatalf("answered inquiry = %#v, want author response persisted", answered)
	}
	accepted, err := svc.TransitionProposal(reviewed.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalAccepted), Version: reviewed.Version}, reviewer)
	if err != nil {
		t.Fatalf("accept proposal after answer: %v", err)
	}
	if accepted.ProposalState != constants.ProposalAccepted {
		t.Fatalf("proposal state = %s, want accepted", accepted.ProposalState)
	}
	history, _, err := svc.ListEvidenceInquiries(dto.EvidenceInquiryQuery{ProposalID: &reviewed.ID}, reviewer)
	if err != nil {
		t.Fatalf("list inquiry history: %v", err)
	}
	if len(history) != 1 || history[0].Status != "answered" {
		t.Fatalf("history = %#v, want retained answered inquiry", history)
	}
}

func TestEvidenceInquiryRejectsUnauthorizedActorsAndForeignEvidence(t *testing.T) {
	svc, _ := newCadastralTestService(t)
	author := testActor(701, constants.RoleSurveyor, "inquiry-author-auth")
	reviewer := testActor(702, constants.RoleReviewer, "inquiry-reviewer-auth")
	_, proposal, observation := createInquiryTestProposal(t, svc, author)
	submitted := moveProposalToReviewed(t, svc, proposal, author.ID, reviewer.ID)
	request := dto.CreateEvidenceInquiryRequest{Question: "请说明该观测如何支撑改线。", ObservationIDs: []uint{observation.ID}}

	_, err := svc.CreateEvidenceInquiry(submitted.ID, request, "author-key", author)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Fatalf("author inquiry error = %v, want 403", err)
	}
	otherReviewer := testActor(701, constants.RoleReviewer, "same-id-reviewer")
	_, err = svc.CreateEvidenceInquiry(submitted.ID, request, "self-key", otherReviewer)
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Fatalf("self inquiry error = %v, want 403", err)
	}
	_, err = svc.CreateEvidenceInquiry(submitted.ID, dto.CreateEvidenceInquiryRequest{
		Question:       "该观测不属于本提案。",
		ObservationIDs: []uint{999999},
	}, "foreign-evidence-key", reviewer)
	if !errors.As(err, &appErr) || appErr.Status != 400 {
		t.Fatalf("foreign observation inquiry error = %v, want 400", err)
	}
	inquiry, err := svc.CreateEvidenceInquiry(submitted.ID, request, "authorized-key", reviewer)
	if err != nil {
		t.Fatalf("authorized inquiry: %v", err)
	}
	_, err = svc.AnswerEvidenceInquiry(inquiry.ID, dto.AnswerEvidenceInquiryRequest{Response: "其他作者无权回答。", Version: inquiry.Version}, testActor(703, constants.RoleGISAnalyst, "other-author"))
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Fatalf("foreign author answer error = %v, want 403", err)
	}
}
