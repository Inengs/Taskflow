package routes

import (
	"github.com/Inengs/realtime-task-app/controllers"
	"github.com/Inengs/realtime-task-app/middleware" // ensure this exists
	"github.com/gin-gonic/gin"
)

func TaskRoutes(router *gin.Engine) {
	tasks := router.Group("/tasks", middleware.AuthMiddleware())
	{
		tasks.GET("/", controllers.TaskListFunc)
		tasks.GET("/:id", controllers.GetTask)
		tasks.POST("/", controllers.CreateNewTask)
		tasks.PUT("/:id", controllers.UpdateTask)
		tasks.DELETE("/:id", controllers.DeleteTask)
		tasks.PATCH("/:id/status", controllers.UpdateTaskStatus)
		tasks.GET("/:id/comments", controllers.ListTaskComments)
		tasks.POST("/:id/comments", controllers.CreateTaskComment)
		tasks.POST("/:id/subtasks", controllers.CreateSubtask)
		tasks.GET("/mine", controllers.ListMyTasks)
	}
}
