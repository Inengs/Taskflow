package controllers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Inengs/realtime-task-app/models"
	"github.com/Inengs/realtime-task-app/utils"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var subtaskValidator = validator.New()

const maxSubtasksPerTask = 100

// CreateSubtask handles POST /tasks/:id/subtasks. Any project member can add one.
func CreateSubtask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "POST /tasks/:id/subtasks")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var input models.CreateSubtaskInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	input.Title = strings.TrimSpace(input.Title)

	if err := subtaskValidator.Struct(input); err != nil {
		var errs []string
		for _, e := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", e.Field(), e.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	// The URL has no project_id: resolve it from the task, then authorize on that.
	var projectID int
	err = db.QueryRow(`SELECT project_id FROM tasks WHERE id = $1`, taskID).Scan(&projectID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load task for subtask", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add subtask"})
		return
	}
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	// Append at the end, and enforce the cap, in one statement. HAVING filters the single
	// aggregate row, so a task already at the cap inserts nothing (ErrNoRows).
	// A burst of simultaneous requests could overshoot the cap by a few; that's acceptable.
	var st models.TaskSubtask
	err = db.QueryRow(`
		INSERT INTO subtasks (task_id, title, position)
		SELECT $1::int, $2::text, COALESCE(MAX(position), -1) + 1
		FROM subtasks WHERE task_id = $1::int
		HAVING COUNT(*) < $3::int
		RETURNING id, title, done, position`,
		taskID, input.Title, maxSubtasksPerTask,
	).Scan(&st.ID, &st.Title, &st.Done, &st.Position)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusConflict, gin.H{
			"error": fmt.Sprintf("A task can have at most %d subtasks", maxSubtasksPerTask)})
		return
	}
	if err != nil {
		slog.Error("Failed to insert subtask", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add subtask"})
		return
	}

	// TODO(realtime): broadcast the new subtask to the project room.
	c.JSON(http.StatusCreated, gin.H{"message": "Subtask added", "subtask": st})
}

// UpdateSubtask handles PATCH /subtasks/:id. Any project member can rename or toggle.
func UpdateSubtask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "PATCH /subtasks/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	subtaskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subtask ID"})
		return
	}

	var input models.UpdateSubtaskInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if input.Title == nil && input.Done == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nothing to update"})
		return
	}
	if input.Title != nil {
		t := strings.TrimSpace(*input.Title)
		if t == "" || utf8.RuneCountInString(t) > 255 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Title must be 1 to 255 characters"})
			return
		}
		input.Title = &t
	}

	// Resolve the subtask's project through its task, then authorize on that.
	var projectID int
	err = db.QueryRow(`
		SELECT t.project_id
		FROM subtasks s
		JOIN tasks t ON t.id = s.task_id
		WHERE s.id = $1`, subtaskID).Scan(&projectID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subtask not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load subtask", "subtask_id", subtaskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update subtask"})
		return
	}
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	// A nil pointer is sent as NULL, and COALESCE keeps the current value.
	var st models.TaskSubtask
	err = db.QueryRow(`
		UPDATE subtasks
		SET title = COALESCE($2::text, title),
		    done  = COALESCE($3::boolean, done)
		WHERE id = $1
		RETURNING id, title, done, position`,
		subtaskID, input.Title, input.Done,
	).Scan(&st.ID, &st.Title, &st.Done, &st.Position)
	if err == sql.ErrNoRows { // deleted between the check and now
		c.JSON(http.StatusNotFound, gin.H{"error": "Subtask not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to update subtask", "subtask_id", subtaskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update subtask"})
		return
	}

	// TODO(realtime): broadcast the change to the project room.
	c.JSON(http.StatusOK, gin.H{"message": "Subtask updated", "subtask": st})
}

// DeleteSubtask handles DELETE /subtasks/:id. Any project member can delete.
func DeleteSubtask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "DELETE /subtasks/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	subtaskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subtask ID"})
		return
	}

	// Resolve the subtask's project through its task, then authorize on that.
	var projectID int
	err = db.QueryRow(`
		SELECT t.project_id
		FROM subtasks s
		JOIN tasks t ON t.id = s.task_id
		WHERE s.id = $1`, subtaskID).Scan(&projectID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subtask not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load subtask", "subtask_id", subtaskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete subtask"})
		return
	}
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	var deletedID int
	err = db.QueryRow(`DELETE FROM subtasks WHERE id = $1 RETURNING id`, subtaskID).Scan(&deletedID)
	if err == sql.ErrNoRows { // already deleted by someone else
		c.JSON(http.StatusNotFound, gin.H{"error": "Subtask not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to delete subtask", "subtask_id", subtaskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete subtask"})
		return
	}

	// TODO(realtime): broadcast the deletion to the project room.
	c.JSON(http.StatusOK, gin.H{"message": "Subtask deleted"})
}
