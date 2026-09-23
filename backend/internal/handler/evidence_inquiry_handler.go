package handler

import (
	"net/http"
	"strings"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"github.com/gin-gonic/gin"
)

func (h *CadastralHandler) ListEvidenceInquiries(c *gin.Context) {
	items, meta, err := h.service.ListEvidenceInquiries(dto.EvidenceInquiryQuery{
		ProposalID: queryUint(c, "proposal_id"),
		Status:     c.Query("status"),
		AuthorID:   queryUint(c, "author_id"),
		Page:       queryInt(c, "page", 1),
		PageSize:   queryInt(c, "page_size", 50),
	}, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, items, meta)
}

func (h *CadastralHandler) CreateEvidenceInquiry(c *gin.Context) {
	proposalID, valid := idParam(c)
	if !valid {
		return
	}
	var req dto.CreateEvidenceInquiryRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.CreateEvidenceInquiry(proposalID, req, strings.TrimSpace(c.GetHeader("Idempotency-Key")), actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, item, nil)
}

func (h *CadastralHandler) GetEvidenceInquiry(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := h.service.GetEvidenceInquiry(id, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, item, nil)
}

func (h *CadastralHandler) AnswerEvidenceInquiry(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req dto.AnswerEvidenceInquiryRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.AnswerEvidenceInquiry(id, req, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, item, nil)
}
