package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/geometry"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
)

// CreateEvidenceChallenge lets a reviewer question the survey observations
// cited by a proposal that is at the review desk. Replaying the same actor and
// Idempotency-Key returns the stored challenge instead of creating a second
// pending record.
func (s *CadastralService) CreateEvidenceChallenge(proposalID uint, req dto.CreateChallengeRequest, idempotencyKey string, actor Actor) (model.EvidenceChallenge, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" || len(key) > 128 {
		return model.EvidenceChallenge{}, invalid("Idempotency-Key must contain between 1 and 128 characters", nil)
	}
	proposal, err := s.store.Proposals.Get(proposalID)
	if errors.Is(err, repository.ErrNotFound) {
		return model.EvidenceChallenge{}, notFound("proposal")
	}
	if err != nil {
		return model.EvidenceChallenge{}, internal("load proposal failed", err)
	}
	if proposal.ProposalState != constants.ProposalSubmitted && proposal.ProposalState != constants.ProposalReviewed {
		return model.EvidenceChallenge{}, conflict("evidence challenges can only be raised for submitted or reviewed proposals", nil)
	}
	if actor.Role != constants.RoleReviewer && actor.Role != constants.RoleAdmin {
		return model.EvidenceChallenge{}, &AppError{CodeForbidden, http.StatusForbidden, "only a reviewer or administrator may raise an evidence challenge", nil}
	}
	if actor.ID == proposal.CreatedBy {
		return model.EvidenceChallenge{}, &AppError{CodeForbidden, http.StatusForbidden, "proposal author cannot challenge their own evidence", nil}
	}

	citedIDs, err := parseObservationIDs(proposal.ObservationIDs)
	if err != nil {
		return model.EvidenceChallenge{}, internal("proposal evidence references are invalid", err)
	}
	cited := make(map[uint]bool, len(citedIDs))
	for _, id := range citedIDs {
		cited[id] = true
	}
	questionIDs := uniqueSortedIDs(append([]uint(nil), req.ObservationIDs...))
	for _, id := range questionIDs {
		if !cited[id] {
			return model.EvidenceChallenge{}, invalid(fmt.Sprintf("observation %d is not cited as evidence by proposal %d", id, proposalID), nil)
		}
		observation, obsErr := s.store.Observations.Get(id)
		if errors.Is(obsErr, repository.ErrNotFound) {
			return model.EvidenceChallenge{}, notFound("observation")
		}
		if obsErr != nil {
			return model.EvidenceChallenge{}, internal("load cited observation failed", obsErr)
		}
		if observation.ParcelID != proposal.ParcelID {
			return model.EvidenceChallenge{}, invalid("all challenged observations must belong to the proposal parcel", nil)
		}
	}
	question := strings.TrimSpace(req.Question)
	requestHash := geometry.Hash(
		strconv.FormatUint(uint64(proposal.ID), 10),
		observationHashPart(questionIDs),
		question,
	)
	if existing, findErr := s.store.Challenges.GetByActorKey(actor.ID, key); findErr == nil {
		return s.replayEvidenceChallenge(existing, requestHash)
	} else if !errors.Is(findErr, repository.ErrNotFound) {
		return model.EvidenceChallenge{}, internal("check challenge idempotency failed", findErr)
	}

	obsJSON, err := json.Marshal(questionIDs)
	if err != nil {
		return model.EvidenceChallenge{}, internal("encode challenged observations failed", err)
	}
	item := model.EvidenceChallenge{
		ProposalID: proposal.ID, ObservationIDs: string(obsJSON), Question: question,
		ChallengeState: constants.ChallengePending, RaisedBy: actor.ID,
		IdempotencyKey: key, RequestHash: requestHash,
	}
	err = s.store.Transaction(func(tx *repository.Store) error {
		sequence, seqErr := tx.Challenges.NextSequence(proposal.ID)
		if seqErr != nil {
			return seqErr
		}
		item.Sequence = sequence
		item.ChallengeCode = fmt.Sprintf("EC-%d-%d", proposal.ID, sequence)
		if createErr := tx.Challenges.Create(&item); createErr != nil {
			return createErr
		}
		return tx.Audits.Create(audit(actor, "challenge.raised", "EvidenceChallenge", item.ID, &proposal.ID, "{}", snapshot(item)))
	})
	if err != nil {
		if existing, findErr := s.store.Challenges.GetByActorKey(actor.ID, key); findErr == nil {
			return s.replayEvidenceChallenge(existing, requestHash)
		}
		return model.EvidenceChallenge{}, wrapCadastral(err, "raise evidence challenge failed")
	}
	return item, nil
}

// RespondEvidenceChallenge records the author's supplementary explanation. A
// challenge can only be answered once; the conditional update keeps duplicate
// responses from reopening or duplicating the archived exchange.
func (s *CadastralService) RespondEvidenceChallenge(challengeID uint, req dto.ChallengeResponseRequest, actor Actor) (model.EvidenceChallenge, error) {
	item, err := s.store.Challenges.Get(challengeID)
	if errors.Is(err, repository.ErrNotFound) {
		return item, notFound("evidence challenge")
	}
	if err != nil {
		return item, internal("get evidence challenge failed", err)
	}
	if item.ChallengeState != constants.ChallengePending {
		return item, conflict("evidence challenge has already been answered", nil)
	}
	proposal, err := s.store.Proposals.Get(item.ProposalID)
	if errors.Is(err, repository.ErrNotFound) {
		return item, notFound("proposal")
	}
	if err != nil {
		return item, internal("load proposal failed", err)
	}
	if proposal.ProposalState == constants.ProposalAccepted || proposal.ProposalState == constants.ProposalRejected {
		return item, conflict("a proposal that is already decided can no longer receive challenge responses", nil)
	}
	isAuthor := actor.ID == proposal.CreatedBy
	if !isAuthor && actor.Role != constants.RoleAdmin {
		return item, &AppError{CodeForbidden, http.StatusForbidden, "only the proposal author or an administrator may answer an evidence challenge", nil}
	}
	response := strings.TrimSpace(req.Response)
	before := item
	respondedAt := time.Now().UTC()
	err = s.store.Transaction(func(tx *repository.Store) error {
		if respondErr := tx.Challenges.Respond(item.ID, response, actor.ID, respondedAt); respondErr != nil {
			return respondErr
		}
		return tx.Audits.Create(audit(actor, "challenge.answered", "EvidenceChallenge", item.ID, &proposal.ID, snapshot(before), snapshot(map[string]any{"response": response, "challenge_state": constants.ChallengeAnswered, "responded_by": actor.ID})))
	})
	if err != nil {
		return item, conflict("evidence challenge changed while responding", err)
	}
	item.Response = response
	item.ChallengeState = constants.ChallengeAnswered
	item.RespondedBy = &actor.ID
	item.RespondedAt = &respondedAt
	return item, nil
}

// ListEvidenceChallenges lists challenges, optionally scoped to one proposal
// via ProposalID; the top-level endpoint omits the scope to return all pending
// questions that gate acceptance across the proposals page.
func (s *CadastralService) ListEvidenceChallenges(q dto.ChallengeQuery) ([]model.EvidenceChallenge, dto.Pagination, error) {
	normalizePage(&q.Page, &q.PageSize)
	items, total, err := s.store.Challenges.List(q)
	if err != nil {
		return nil, dto.Pagination{}, internal("list evidence challenges failed", err)
	}
	return items, dto.Pagination{Page: q.Page, PageSize: q.PageSize, Total: total}, nil
}

// pendingChallengeCodes returns the human-readable codes that must be shown on
// the disabled acceptance entry while evidence questions remain open.
func (s *CadastralService) pendingChallengeCodes(proposalID uint) ([]string, error) {
	pending, _, err := s.store.Challenges.List(dto.ChallengeQuery{ProposalID: &proposalID, State: string(constants.ChallengePending), Page: 1, PageSize: 100})
	if err != nil {
		return nil, internal("load pending evidence challenges failed", err)
	}
	codes := make([]string, 0, len(pending))
	for _, item := range pending {
		codes = append(codes, item.ChallengeCode)
	}
	sort.Strings(codes)
	return codes, nil
}

func (s *CadastralService) replayEvidenceChallenge(existing model.EvidenceChallenge, requestHash string) (model.EvidenceChallenge, error) {
	if existing.RequestHash != requestHash {
		return model.EvidenceChallenge{}, conflict("Idempotency-Key has already been used with a different challenge request", nil)
	}
	return existing, nil
}

func parseObservationIDs(raw string) ([]uint, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []uint{}, nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func observationHashPart(ids []uint) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(uint64(id), 10)
	}
	return strings.Join(parts, ",")
}
