package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
	"gorm.io/gorm"
)

var errEvidenceInquiryPending = errors.New("evidence inquiry already pending")
var errEvidenceInquiryReplay = errors.New("evidence inquiry idempotency replay")

func InquiryNumber(id uint) string {
	return fmt.Sprintf("EI-%06d", id)
}

func (s *CadastralService) CreateEvidenceInquiry(proposalID uint, req dto.CreateEvidenceInquiryRequest, idempotencyKey string, actor Actor) (dto.EvidenceInquiryView, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 128 {
		return dto.EvidenceInquiryView{}, invalid("Idempotency-Key is required and must be 1-128 characters", nil)
	}
	question := strings.TrimSpace(req.Question)
	if len([]rune(question)) < 4 {
		return dto.EvidenceInquiryView{}, invalid("the inquiry question must clearly describe the evidence concern", nil)
	}
	proposal, err := s.store.Proposals.Get(proposalID)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.EvidenceInquiryView{}, notFound("proposal")
	}
	if err != nil {
		return dto.EvidenceInquiryView{}, internal("get proposal failed", err)
	}
	if err := authorizeEvidenceInquiryCreation(proposal, actor); err != nil {
		return dto.EvidenceInquiryView{}, err
	}
	if proposal.ProposalState != constants.ProposalSubmitted && proposal.ProposalState != constants.ProposalReviewed {
		return dto.EvidenceInquiryView{}, conflict("evidence questions are available after the proposal is submitted for review", nil)
	}
	if replay, found, replayErr := s.store.Inquiries.FindByCreatorAndKey(actor.ID, idempotencyKey); replayErr != nil {
		return dto.EvidenceInquiryView{}, internal("find evidence inquiry request failed", replayErr)
	} else if found {
		if replay.ProposalID != proposalID {
			return dto.EvidenceInquiryView{}, conflict("Idempotency-Key has already been used for another proposal", nil)
		}
		return s.evidenceInquiryView(replay)
	}
	if existing, found, pendingErr := s.store.Inquiries.FindPendingByProposal(proposalID); pendingErr != nil {
		return dto.EvidenceInquiryView{}, internal("check pending evidence inquiry failed", pendingErr)
	} else if found {
		view, viewErr := s.evidenceInquiryView(existing)
		if viewErr != nil {
			return dto.EvidenceInquiryView{}, viewErr
		}
		return view, conflict(fmt.Sprintf("%s is already awaiting a response", view.InquiryNumber), nil)
	}

	observationIDs, err := proposalEvidenceObservationIDs(proposal)
	if err != nil {
		return dto.EvidenceInquiryView{}, internal("read proposal evidence failed", err)
	}
	selectedIDs, err := validateSelectedObservations(observationIDs, req.ObservationIDs)
	if err != nil {
		return dto.EvidenceInquiryView{}, err
	}
	observations, err := s.observationSnapshots(proposal.ParcelID, selectedIDs)
	if err != nil {
		return dto.EvidenceInquiryView{}, err
	}
	idsJSON, err := json.Marshal(selectedIDs)
	if err != nil {
		return dto.EvidenceInquiryView{}, internal("encode evidence inquiry observations failed", err)
	}
	snapshotJSON, err := json.Marshal(observations)
	if err != nil {
		return dto.EvidenceInquiryView{}, internal("encode observation snapshots failed", err)
	}
	item := model.EvidenceInquiry{
		ProposalID:           proposalID,
		ObservationIDs:       string(idsJSON),
		ObservationSnapshots: string(snapshotJSON),
		Question:             question,
		Status:               constants.EvidenceInquiryPending,
		Version:              1,
		CreatedBy:            actor.ID,
		IdempotencyKey:       idempotencyKey,
	}
	err = s.store.Transaction(func(tx *repository.Store) error {
		if createErr := tx.Inquiries.Create(&item); createErr != nil {
			if replay, found, replayErr := tx.Inquiries.FindByCreatorAndKey(actor.ID, idempotencyKey); replayErr != nil {
				return internal("locate idempotent evidence inquiry failed", replayErr)
			} else if found {
				item = replay
				return errEvidenceInquiryReplay
			}
			pending, found, pendingErr := tx.Inquiries.FindPendingByProposal(proposalID)
			if pendingErr == nil && found {
				item = pending
				return errEvidenceInquiryPending
			}
			if pendingErr != nil {
				return internal("locate pending evidence inquiry failed", pendingErr)
			}
			if errors.Is(createErr, gorm.ErrDuplicatedKey) {
				return conflict("Idempotency-Key has already been used", createErr)
			}
			return createErr
		}
		return tx.Audits.Create(audit(actor, "evidence_inquiry.created", "EvidenceInquiry", item.ID, &item.ProposalID, "{}", snapshot(item)))
	})
	if errors.Is(err, errEvidenceInquiryReplay) {
		return s.evidenceInquiryView(item)
	}
	if errors.Is(err, errEvidenceInquiryPending) {
		view, viewErr := s.evidenceInquiryView(item)
		if viewErr != nil {
			return dto.EvidenceInquiryView{}, conflict("an evidence inquiry is already awaiting a response", nil)
		}
		message := fmt.Sprintf("%s is already awaiting a response", view.InquiryNumber)
		return view, conflict(message, nil)
	}
	if err != nil {
		return dto.EvidenceInquiryView{}, wrapCadastral(err, "create evidence inquiry failed")
	}
	return s.evidenceInquiryView(item)
}

func (s *CadastralService) ListEvidenceInquiries(q dto.EvidenceInquiryQuery, actor Actor) ([]dto.EvidenceInquiryView, dto.Pagination, error) {
	normalizePage(&q.Page, &q.PageSize)
	if q.Status != "" && !constants.EvidenceInquiryState(q.Status).Valid() {
		return nil, dto.Pagination{}, invalid("unknown evidence inquiry status", nil)
	}
	if actor.Role == constants.RoleSurveyor || actor.Role == constants.RoleGISAnalyst {
		q.AuthorID = &actor.ID
		if q.ProposalID != nil {
			proposal, err := s.store.Proposals.Get(*q.ProposalID)
			if errors.Is(err, repository.ErrNotFound) {
				return nil, dto.Pagination{}, notFound("proposal")
			}
			if err != nil {
				return nil, dto.Pagination{}, internal("get proposal failed", err)
			}
			if proposal.CreatedBy != actor.ID {
				return nil, dto.Pagination{}, &AppError{CodeForbidden, http.StatusForbidden, "only the proposal author may read its evidence inquiries", nil}
			}
		}
	}
	items, total, err := s.store.Inquiries.List(q)
	if err != nil {
		return nil, dto.Pagination{}, internal("list evidence inquiries failed", err)
	}
	return s.evidenceInquiryViews(items), dto.Pagination{Page: q.Page, PageSize: q.PageSize, Total: total}, nil
}

func (s *CadastralService) GetEvidenceInquiry(id uint, actor Actor) (dto.EvidenceInquiryView, error) {
	item, err := s.store.Inquiries.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.EvidenceInquiryView{}, notFound("evidence inquiry")
	}
	if err != nil {
		return dto.EvidenceInquiryView{}, internal("get evidence inquiry failed", err)
	}
	if actor.Role == constants.RoleSurveyor || actor.Role == constants.RoleGISAnalyst {
		proposal, proposalErr := s.store.Proposals.Get(item.ProposalID)
		if proposalErr != nil {
			return dto.EvidenceInquiryView{}, internal("get proposal failed", proposalErr)
		}
		if proposal.CreatedBy != actor.ID {
			return dto.EvidenceInquiryView{}, &AppError{CodeForbidden, http.StatusForbidden, "only the proposal author may read this evidence inquiry", nil}
		}
	}
	return s.evidenceInquiryView(item)
}

func (s *CadastralService) AnswerEvidenceInquiry(id uint, req dto.AnswerEvidenceInquiryRequest, actor Actor) (dto.EvidenceInquiryView, error) {
	item, err := s.store.Inquiries.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.EvidenceInquiryView{}, notFound("evidence inquiry")
	}
	if err != nil {
		return dto.EvidenceInquiryView{}, internal("get evidence inquiry failed", err)
	}
	if item.Status != constants.EvidenceInquiryPending {
		return dto.EvidenceInquiryView{}, conflict("this evidence inquiry has already been answered", nil)
	}
	proposal, err := s.store.Proposals.Get(item.ProposalID)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.EvidenceInquiryView{}, notFound("proposal")
	}
	if err != nil {
		return dto.EvidenceInquiryView{}, internal("get proposal failed", err)
	}
	if actor.ID != proposal.CreatedBy && actor.Role != constants.RoleAdmin {
		return dto.EvidenceInquiryView{}, &AppError{CodeForbidden, http.StatusForbidden, "only the proposal author or an administrator may answer this evidence inquiry", nil}
	}
	response := strings.TrimSpace(req.Response)
	if len([]rune(response)) < 4 {
		return dto.EvidenceInquiryView{}, invalid("the response must include the supporting explanation", nil)
	}
	now := time.Now()
	err = s.store.Transaction(func(tx *repository.Store) error {
		if answerErr := tx.Inquiries.Answer(id, req.Version, actor.ID, response, now, nil); answerErr != nil {
			return answerErr
		}
		return tx.Audits.Create(audit(actor, "evidence_inquiry.answered", "EvidenceInquiry", id, &item.ProposalID, snapshot(item), snapshot(map[string]any{"status": constants.EvidenceInquiryAnswered, "response": response, "version": item.Version + 1})))
	})
	if err != nil {
		return dto.EvidenceInquiryView{}, conflict("evidence inquiry changed while answering", err)
	}
	persisted, err := s.store.Inquiries.Get(id)
	if err != nil {
		return dto.EvidenceInquiryView{}, internal("reload answered evidence inquiry failed", err)
	}
	return s.evidenceInquiryView(persisted)
}

func (s *CadastralService) observationSnapshots(parcelID uint, ids []uint) ([]model.EvidenceInquiryObservation, error) {
	snapshots := make([]model.EvidenceInquiryObservation, 0, len(ids))
	for _, id := range ids {
		observation, err := s.store.Observations.Get(id)
		if errors.Is(err, repository.ErrNotFound) || err == nil && observation.ParcelID != parcelID {
			return nil, invalid("all selected observations must belong to the proposal parcel", nil)
		}
		if err != nil {
			return nil, internal("load selected observation failed", err)
		}
		snapshots = append(snapshots, model.EvidenceInquiryObservation{
			ID:                  observation.ID,
			ObservationCode:     observation.ObservationCode,
			PointGeoJSON:        observation.PointGeoJSON,
			ObservedAt:          observation.ObservedAt.UTC().Format(time.RFC3339),
			Method:              observation.Method,
			HorizontalAccuracyM: observation.HorizontalAccuracyM,
			ObservationState:    observation.ObservationState,
			QualityNote:         observation.QualityNote,
		})
	}
	return snapshots, nil
}

func proposalEvidenceObservationIDs(proposal model.BoundaryProposal) ([]uint, error) {
	var ids []uint
	if strings.TrimSpace(proposal.ObservationIDs) == "" {
		return ids, nil
	}
	if err := json.Unmarshal([]byte(proposal.ObservationIDs), &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func validateSelectedObservations(proposalIDs, selected []uint) ([]uint, error) {
	if len(selected) == 0 {
		return nil, invalid("select at least one survey observation to question", nil)
	}
	selected = append([]uint(nil), selected...)
	slices.Sort(selected)
	selected = slices.Compact(selected)
	allowed := make(map[uint]struct{}, len(proposalIDs))
	for _, id := range proposalIDs {
		allowed[id] = struct{}{}
	}
	for _, id := range selected {
		if _, ok := allowed[id]; !ok {
			return nil, invalid("all selected observations must already be attached to the proposal", nil)
		}
	}
	return selected, nil
}

func authorizeEvidenceInquiryCreation(proposal model.BoundaryProposal, actor Actor) error {
	if actor.Role != constants.RoleReviewer && actor.Role != constants.RoleAdmin {
		return &AppError{CodeForbidden, http.StatusForbidden, "only a reviewer or administrator may request evidence", nil}
	}
	if actor.ID == proposal.CreatedBy {
		return &AppError{CodeForbidden, http.StatusForbidden, "the proposal author cannot request evidence from themselves", nil}
	}
	return nil
}

func (s *CadastralService) evidenceInquiryViews(items []model.EvidenceInquiry) []dto.EvidenceInquiryView {
	ids := make([]uint, 0, len(items)*2)
	for _, item := range items {
		ids = append(ids, item.CreatedBy)
		if item.RespondedBy != nil {
			ids = append(ids, *item.RespondedBy)
		}
	}
	users, err := s.store.Users.FindByIDs(ids)
	if err != nil {
		users = map[uint]model.User{}
	}
	views := make([]dto.EvidenceInquiryView, 0, len(items))
	for _, item := range items {
		views = append(views, evidenceInquiryView(item, users))
	}
	return views
}

func (s *CadastralService) evidenceInquiryView(item model.EvidenceInquiry) (dto.EvidenceInquiryView, error) {
	ids := []uint{item.CreatedBy}
	if item.RespondedBy != nil {
		ids = append(ids, *item.RespondedBy)
	}
	users, err := s.store.Users.FindByIDs(ids)
	if err != nil {
		return dto.EvidenceInquiryView{}, internal("load evidence inquiry actors failed", err)
	}
	return evidenceInquiryView(item, users), nil
}

func evidenceInquiryView(item model.EvidenceInquiry, users map[uint]model.User) dto.EvidenceInquiryView {
	var ids []uint
	if err := json.Unmarshal([]byte(item.ObservationIDs), &ids); err != nil {
		ids = []uint{}
	}
	var snapshots []model.EvidenceInquiryObservation
	if err := json.Unmarshal([]byte(item.ObservationSnapshots), &snapshots); err != nil {
		snapshots = []model.EvidenceInquiryObservation{}
	}
	observations := make([]dto.EvidenceInquiryObservationView, 0, len(snapshots))
	for _, observation := range snapshots {
		observations = append(observations, dto.EvidenceInquiryObservationView(observation))
	}
	var respondedAt *string
	if item.RespondedAt != nil {
		formatted := item.RespondedAt.UTC().Format(time.RFC3339)
		respondedAt = &formatted
	}
	return dto.EvidenceInquiryView{
		ID: item.ID, InquiryNumber: InquiryNumber(item.ID), ProposalID: item.ProposalID, ObservationIDs: ids, Observations: observations,
		Question: item.Question, Response: item.Response, Status: string(item.Status), Version: item.Version,
		CreatedBy: item.CreatedBy, CreatedByName: userDisplayName(users, item.CreatedBy), RespondedBy: item.RespondedBy,
		RespondedByName: optionalUserDisplayName(users, item.RespondedBy), RespondedAt: respondedAt,
		CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func userDisplayName(users map[uint]model.User, id uint) string {
	if user, ok := users[id]; ok {
		return user.DisplayName
	}
	return fmt.Sprintf("user-%d", id)
}

func optionalUserDisplayName(users map[uint]model.User, id *uint) string {
	if id == nil {
		return ""
	}
	return userDisplayName(users, *id)
}
