package routes

import (
	"github.com/Inengs/realtime-task-app/controllers"
	"github.com/Inengs/realtime-task-app/middleware"
	"github.com/gin-gonic/gin"
)

func ProjectRoutes(router *gin.Engine) {
	projects := router.Group("/projects", middleware.AuthMiddleware())

	{
		projects.GET("/", controllers.ListProjects)
		projects.GET("/:id", controllers.ProjectDetails)
		projects.POST("/", controllers.CreateProject)
		projects.PUT("/:id", controllers.UpdateProject)
		projects.DELETE("/:id", controllers.DeleteProject)

		// Tasks scoped to a project (the kanban board's data source)
		projects.GET("/:id/tasks", controllers.ListProjectTasks) // list every project tasks
		projects.POST("/:id/tasks", controllers.CreateProjectTask)

		projects.GET("/:id/members", controllers.ListProjectMembers)

		projects.POST("/:id/invitations", controllers.CreateProjectInvitation)

		projects.GET("/:id/invitations", controllers.ListProjectInvitations)

		projects.DELETE("/:id/invitations/:invId", controllers.RevokeProjectInvitation)
	}
}
