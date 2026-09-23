package router

import (
	"cadastral-boundary-topology-resolution/backend/internal/constants"
	appmw "cadastral-boundary-topology-resolution/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

func registerEvidenceInquiryRoutes(api *gin.RouterGroup, deps Dependencies) {
	h := deps.CadastralHandler
	inquiries := api.Group("/evidence-inquiries", appmw.RBACMiddleware(
		constants.RoleSurveyor,
		constants.RoleGISAnalyst,
		constants.RoleReviewer,
		constants.RoleAuditor,
		constants.RoleAdmin,
	))
	inquiries.GET("", h.ListEvidenceInquiries)
	inquiries.GET("/:id", h.GetEvidenceInquiry)
	api.POST("/proposals/:id/evidence-inquiries", appmw.RBACMiddleware(constants.RoleReviewer, constants.RoleAdmin), h.CreateEvidenceInquiry)
	inquiries.POST("/:id/answer", appmw.RBACMiddleware(constants.RoleSurveyor, constants.RoleGISAnalyst, constants.RoleAdmin), h.AnswerEvidenceInquiry)
}
