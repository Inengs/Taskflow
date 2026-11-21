package routes

import (
	"github.com/Inengs/realtime-task-app/controllers"
	"github.com/Inengs/realtime-task-app/middleware"
	"github.com/gin-gonic/gin"
)

func InvitationRoutes(router *gin.Engine) {
	invitations := router.Group("/invitations", middleware.AuthMiddleware())
	{
		invitations.POST("/:id/accept", controllers.AcceptInvitation)
		invitations.POST("/:id/decline", controllers.DeclineInvitation)
	}

}
