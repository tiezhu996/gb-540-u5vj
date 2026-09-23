package router

import (
	"cadastral-boundary-topology-resolution/backend/internal/constants"
	appmw "cadastral-boundary-topology-resolution/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

func registerEvidenceChallengeRoutes(api *gin.RouterGroup, deps Dependencies) {
	h := deps.CadastralHandler
	proposals := api.Group("/proposals")
	proposals.GET("/:id/challenges", h.ListEvidenceChallenges)
	proposals.POST("/:id/challenges", appmw.RBACMiddleware(constants.RoleReviewer, constants.RoleAdmin), h.CreateEvidenceChallenge)
	challenges := api.Group("/evidence-challenges")
	challenges.GET("", h.ListAllEvidenceChallenges)
	challenges.POST("/:id/respond", appmw.RBACMiddleware(constants.RoleSurveyor, constants.RoleGISAnalyst, constants.RoleAdmin), h.RespondEvidenceChallenge)
}
