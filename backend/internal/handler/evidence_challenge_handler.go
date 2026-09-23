package handler

import (
	"net/http"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"github.com/gin-gonic/gin"
)

func (h *CadastralHandler) ListAllEvidenceChallenges(c *gin.Context) {
	items, meta, err := h.service.ListEvidenceChallenges(dto.ChallengeQuery{State: c.Query("state"), Page: queryInt(c, "page", 1), PageSize: queryInt(c, "page_size", 100)})
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, items, meta)
}

func (h *CadastralHandler) ListEvidenceChallenges(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	items, meta, err := h.service.ListEvidenceChallenges(dto.ChallengeQuery{ProposalID: &id, State: c.Query("state"), Page: queryInt(c, "page", 1), PageSize: queryInt(c, "page_size", 100)})
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, items, meta)
}

func (h *CadastralHandler) CreateEvidenceChallenge(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req dto.CreateChallengeRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.CreateEvidenceChallenge(id, req, c.GetHeader("Idempotency-Key"), actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, item, nil)
}

func (h *CadastralHandler) RespondEvidenceChallenge(c *gin.Context) {
	challengeID, valid := idParam(c)
	if !valid {
		return
	}
	var req dto.ChallengeResponseRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.RespondEvidenceChallenge(challengeID, req, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, item, nil)
}
