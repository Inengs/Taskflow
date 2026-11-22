package routes

import (
	"github.com/Inengs/realtime-task-app/controllers"
	"github.com/Inengs/realtime-task-app/middleware"
	"github.com/gin-gonic/gin"
)

func SubtasksRoutes(r *gin.Engine /* same params as TaskRoutes */) {
	subtasks := r.Group("/subtasks", middleware.AuthMiddleware())
	subtasks.PATCH("/:id", controllers.UpdateSubtask)
	subtasks.DELETE("/:id", controllers.DeleteSubtask)
	// #25 will add: DELETE /:id
}
