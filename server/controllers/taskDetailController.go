package controllers

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Inengs/realtime-task-app/models"
	"github.com/Inengs/realtime-task-app/utils"
	"github.com/gin-gonic/gin"
)

// GetTask handles GET /tasks/:id. Any member of the task's project can read it.
func GetTask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "GET /tasks/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// The URL has no project_id: resolve it (plus project name and creator) from the task.
	var projectID int
	var projectName, creatorName, creatorAvatar string
	var creatorID sql.NullInt64 // NULL if the creator's account was deleted
	err = db.QueryRow(`
		SELECT t.project_id, p.name, t.user_id, COALESCE(u.username, ''), COALESCE(u.avatar_url, '')
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		LEFT JOIN users u ON u.id = t.user_id
		WHERE t.id = $1`, taskID,
	).Scan(&projectID, &projectName, &creatorID, &creatorName, &creatorAvatar)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load task", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch task"})
		return
	}

	// Authorize on the task's project. Non-members get 404, same as a missing task.
	role, err := utils.RequireProjectRole(db, projectID, uid, "member")
	if err != nil {
		if err == utils.ErrNotProjectMember {
			c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
			return
		}
		utils.RespondProjectAccessError(c, err)
		return
	}

	task, err := loadProjectTask(db, taskID) // task fields, assignees, subtask/comment counts
	if err == sql.ErrNoRows {                // deleted between the two reads
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load task detail", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch task"})
		return
	}

	subtasks := []models.TaskSubtask{} // [] not null when empty
	rows, err := db.Query(`
		SELECT id, title, done, position
		FROM subtasks WHERE task_id = $1
		ORDER BY position, id`, taskID)
	if err != nil {
		slog.Error("Failed to load subtasks", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch task"})
		return
	}
	defer rows.Close()
	for rows.Next() {
		var s models.TaskSubtask
		if err := rows.Scan(&s.ID, &s.Title, &s.Done, &s.Position); err != nil {
			slog.Error("Failed to scan subtask", "task_id", taskID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch task"})
			return
		}
		subtasks = append(subtasks, s)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating subtasks", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch task"})
		return
	}

	detail := models.TaskDetail{
		ProjectTask: task,
		ProjectName: projectName,
		Subtasks:    subtasks,
		CanEdit:     true, // any member (matches PUT /tasks/:id)
		// Matches DELETE /tasks/:id: the creator, or admin and above.
		CanDelete: (creatorID.Valid && int(creatorID.Int64) == uid) || memberRank(role) >= memberRank("admin"),
	}
	if creatorID.Valid {
		detail.Creator = &models.TaskUserBrief{
			ID: int(creatorID.Int64), Username: creatorName, AvatarURL: creatorAvatar,
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task fetched", "task": detail})
}
