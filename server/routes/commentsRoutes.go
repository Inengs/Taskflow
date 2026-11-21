package routes

import (
	"github.com/Inengs/realtime-task-app/controllers"
	"github.com/Inengs/realtime-task-app/middleware"
	"github.com/gin-gonic/gin"
)

func CommentsRoutes(r *gin.Engine /* same params as TaskRoutes */) {
	comments := r.Group("/comments", middleware.AuthMiddleware())
	comments.PUT("/:id", controllers.UpdateComment)
	comments.DELETE("/:id", controllers.DeleteComment)
	// #22 will add: DELETE /:id
}
