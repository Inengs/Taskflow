package controllers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Inengs/realtime-task-app/models"
	"github.com/Inengs/realtime-task-app/utils"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// TaskListFunc lists the authenticated user's tasks
//
//	@Summary	List tasks
//	@Tags		tasks
//	@Produce	json
//	@Security	SessionCookie
//	@Success	200	{object}	map[string]any	"message + tasks array"
//	@Failure	401	{object}	map[string]any
//	@Router		/tasks [get]
func TaskListFunc(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB) // Setup database connection

	// Get user ID from session
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userIDInt, _ := userID.(int)

	rows, err := db.Query(`SELECT id, user_id, project_id, title, description, status, created_at, updated_at FROM tasks WHERE user_id = $1`, userIDInt) // query all tasks from database
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	defer rows.Close()

	// Collect tasks into a slice of Task structs
	var tasks []models.Task
	for rows.Next() {
		var task models.Task
		if err := rows.Scan(&task.ID, &task.UserID, &task.ProjectID, &task.Title, &task.Description, &task.Status, &task.CreatedAt, &task.UpdatedAt); err != nil {
			// Return 500 if scanning fails
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tasks retrieved successfully", "tasks": tasks})
}

// TaskDetailsFunc fetches one task by ID
//
//	@Summary	Task details
//	@Tags		tasks
//	@Produce	json
//	@Security	SessionCookie
//	@Param		id	path		int	true	"Task ID"
//	@Success	200	{object}	map[string]any
//	@Failure	404	{object}	map[string]any
//	@Router		/tasks/{id} [get]
func TaskDetailsFunc(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB) // Setup database connection

	// Get user ID from session
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userIDInt, _ := userID.(int)

	// Extract and validate ID from URL parameter
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// Query task by ID
	var task models.Task
	err = db.QueryRow("SELECT id, user_id, project_id, title, description, status, created_at, updated_at FROM tasks WHERE id = $1 AND user_id = $2", id, userIDInt).
		Scan(&task.ID, &task.UserID, &task.ProjectID, &task.Title, &task.Description, &task.Status, &task.CreatedAt, &task.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Task with ID %d not found", id)})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Return task details
	c.JSON(http.StatusOK, gin.H{"message": "Task retrieved successfully", "task": task})
}

// CreateNewTask creates a task
//
//	@Summary	Create task
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	SessionCookie
//	@Param		request	body		models.TaskInput	true	"Task"
//	@Success	201		{object}	map[string]any
//	@Failure	400		{object}	map[string]any
//	@Router		/tasks [post]
func CreateNewTask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB) // Setup database connection

	// Get user ID from session
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userIDInt, _ := userID.(int)

	// Bind and validate request body
	var input models.TaskInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate input using validator
	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		var errs []string
		for _, err := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", err.Field(), err.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	// Insert task into database
	var task models.Task
	err := db.QueryRow(
		"INSERT INTO tasks (title, description, status, user_id, project_id) VALUES ($1, $2, $3, $4, $5) RETURNING id, user_id, project_id, title, description, status, created_at, updated_at",
		input.Title, input.Description, input.Status, userIDInt, input.ProjectID,
	).Scan(&task.ID, &task.UserID, &task.ProjectID, &task.Title, &task.Description, &task.Status, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// ─── Log activity: "user created task X" ───
	utils.LogActivity(db, userIDInt, "task", task.ID, "created", task.Title, nil)

	// Send notification and broadcast task
	if err := SendNotification(db, userIDInt, fmt.Sprintf("New task created: %s", task.Title)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send notification"})
		return
	}
	manager.BroadcastTask(userIDInt, task, "task_created")

	// Return created task
	c.JSON(http.StatusCreated, gin.H{"message": "Task created successfully", "task": task})
}

// UpdateTask updates a task
//
//	@Summary	Update task
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	SessionCookie
//	@Param		id		path		int					true	"Task ID"
//	@Param		request	body		models.TaskInput	true	"Task"
//	@Success	200		{object}	map[string]any
//	@Failure	404		{object}	map[string]any
//	@Router		/tasks/{id} [put]
func UpdateTask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB) // Setup database connection

	// Get user ID from session
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userIDInt, _ := userID.(int)

	// Extract and validate ID from URL parameter
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// Bind and validate request body
	var input models.TaskInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		var errs []string
		for _, err := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", err.Field(), err.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	// Update task in database
	var task models.Task
	err = db.QueryRow(
		"UPDATE tasks SET title = $1, description = $2, status = $3, project_id = $4, updated_at = CURRENT_TIMESTAMP WHERE id = $5 AND user_id = $6 RETURNING id, user_id, project_id, title, description, status, created_at, updated_at",
		input.Title, input.Description, input.Status, input.ProjectID, id, userIDInt,
	).Scan(&task.ID, &task.UserID, &task.ProjectID, &task.Title, &task.Description, &task.Status, &task.CreatedAt, &task.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Task with ID %d not found", id)})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Send notification and broadcast task
	if err := SendNotification(db, userIDInt, fmt.Sprintf("Task updated: %s", task.Title)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send notification"})
		return
	}
	manager.BroadcastTask(userIDInt, task, "task_updated")

	// Return Updated task
	c.JSON(http.StatusOK, gin.H{"message": "Task updated successfully", "task": task})
}

// DeleteTask deletes a task
//
//	@Summary	Delete task
//	@Tags		tasks
//	@Produce	json
//	@Security	SessionCookie
//	@Param		id	path		int	true	"Task ID"
//	@Success	200	{object}	map[string]any
//	@Failure	404	{object}	map[string]any
//	@Router		/tasks/{id} [delete]
func DeleteTask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	// Get user ID from session
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userIDInt, _ := userID.(int)

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var task models.Task
	err = db.QueryRow(
		"SELECT id, user_id, project_id, title, description, status, created_at, updated_at FROM tasks WHERE id = $1 AND user_id = $2",
		id, userIDInt,
	).Scan(&task.ID, &task.UserID, &task.ProjectID, &task.Title, &task.Description, &task.Status, &task.CreatedAt, &task.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Task with ID %d not found", id)})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	result, err := db.Exec("DELETE FROM tasks WHERE id = $1 and user_id = $2", id, userIDInt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Check if task was deleted
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Task with ID %d not found", id)})
		return
	}

	// ─── Log activity: "user deleted task X" ───
	// Note: task.ID is gone from DB now, but we still log the title for the feed.
	utils.LogActivity(db, userIDInt, "task", task.ID, "deleted", task.Title, nil)

	// Send notification and broadcast task deletion
	if err := SendNotification(db, userIDInt, fmt.Sprintf("Task deleted: %s", task.Title)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send notification"})
		return
	}
	manager.BroadcastTask(userIDInt, task, "task_deleted")

	// Return success response
	c.JSON(http.StatusOK, gin.H{"message": "Task deleted successfully"})
}

// UpdateTaskStatus moves a task between kanban columns (and/or reorders it
// within its current column). All position shifts in both the source and
// destination columns happen in one transaction.
//
// Behavior:
//
//   - Any project member can move tasks.
//
//   - Cross-column move: close the gap in the source column, make room in the
//     destination column, then update the task.
//
//   - Same-column reorder: shift the in-between tasks up or down by 1.
//
//   - If old and new (status, position) are identical, the request is a no-op.
//
//   - Activity is logged only when a task transitions INTO "done" — every
//     column hop would be too noisy for the dashboard feed.
//
//     @Summary	Move task between columns
//     @Tags		tasks
//     @Accept		json
//     @Produce	json
//     @Security	BearerAuth
//     @Param		id		path		int							true	"Task ID"
//     @Param		request	body		UpdateTaskStatusInput		true	"New status + position"
//     @Success	200		{object}	map[string]any
//     @Failure	400		{object}	map[string]any
//     @Failure	403		{object}	map[string]any
//     @Failure	404		{object}	map[string]any
//     @Router		/tasks/{id}/status [patch]
func UpdateTaskStatus(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "PATCH /tasks/:id/status")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	taskID, err := strconv.Atoi(c.Param("id")) // get taskID from the params
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var input models.UpdateTaskStatusInput
	// Parse the JSON body into { status, position }
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if !validTaskStatuses[input.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid status: %q", input.Status)}) // catch invalid status
		return
	}
	if input.Position < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Position must be >= 0"})
		return
	}

	// Fetch the task's current state. We need the project_id (for the auth
	// check), the old status and position (for the shift math), and the title
	// (for the activity log on completion).
	var projectID int    //  needed for RequireProjectRole (the auth check). The URL has no project_id — we resolve it from the task itself.
	var oldStatus string // needed to know if this is a same-column reorder or cross-column move (the shift math differs).
	var oldPosition int  // needed for the same-column shift math (range [new, old) or (old, new]).
	var title string     // needed for the activity log if the task is being marked "done".

	err = db.QueryRow(`
		SELECT project_id, status, position, title FROM tasks WHERE id = $1`,
		taskID,
	).Scan(&projectID, &oldStatus, &oldPosition, &title)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load task for status change",
			"task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}

	// Auth check on the task's project — not the URL. This is what makes the
	// endpoint safe even though the URL has no project_id.
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	// No-op: nothing changed. Skip the transaction entirely.
	if oldStatus == input.Status && oldPosition == input.Position {
		c.JSON(http.StatusOK, gin.H{
			"message": "No change",
			"task": gin.H{
				"id":         taskID,
				"status":     oldStatus,
				"position":   oldPosition,
				"updated_at": nil,
			},
		})
		return
	}

	transaction, err := db.Begin()
	if err != nil {
		slog.Error("Failed to begin transaction", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}
	defer transaction.Rollback() // no-op after Commit

	if oldStatus == input.Status {
		// ─── Same-column reorder ───
		if input.Position < oldPosition {
			// Moving UP: shift tasks in [new, old) down by 1 to make room.
			// (Yes, "down by 1" = position+1, because lower position = higher
			// on the board. Confusing naming, but the math is right.)
			if _, err := transaction.Exec(`
				UPDATE tasks SET position = position + 1
				WHERE project_id = $1 AND status = $2
				  AND position >= $3 AND position < $4`,
				projectID, oldStatus, input.Position, oldPosition); err != nil {
				slog.Error("Failed to shift tasks (same-column up)",
					"task_id", taskID, "error", err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
				return
			}
		} else {
			// Moving DOWN: shift tasks in (old, new] up by 1 to close the gap
			// above the moved task.
			if _, err := transaction.Exec(`
				UPDATE tasks SET position = position - 1
				WHERE project_id = $1 AND status = $2
				  AND position > $3 AND position <= $4`,
				projectID, oldStatus, oldPosition, input.Position); err != nil {
				slog.Error("Failed to shift tasks (same-column down)",
					"task_id", taskID, "error", err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
				return
			}
		}
	} else {
		// ─── Cross-column move ───
		// Source column: close the gap left by the moved task.
		if _, err := transaction.Exec(`
			UPDATE tasks SET position = position - 1
			WHERE project_id = $1 AND status = $2 AND position > $3`,
			projectID, oldStatus, oldPosition); err != nil {
			slog.Error("Failed to shift source column",
				"task_id", taskID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
			return
		}
		// Destination column: make room at the target position.
		if _, err := transaction.Exec(`
			UPDATE tasks SET position = position + 1
			WHERE project_id = $1 AND status = $2 AND position >= $3`,
			projectID, input.Status, input.Position); err != nil {
			slog.Error("Failed to shift destination column",
				"task_id", taskID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
			return
		}
	}

	// Finally, move the task itself.
	var updatedAt sql.NullTime
	err = transaction.QueryRow(`
		UPDATE tasks SET status = $1, position = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
		RETURNING updated_at`,
		input.Status, input.Position, taskID,
	).Scan(&updatedAt)
	if err != nil {
		slog.Error("Failed to update task status",
			"task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}

	// Activity log — only when a task transitions INTO "done".
	// Logging every column hop would flood the dashboard feed.
	if input.Status == "done" && oldStatus != "done" {
		if _, err := transaction.Exec(`
			INSERT INTO activity_log (user_id, entity_type, entity_id, action, target)
			VALUES ($1, 'task', $2, 'completed', $3)`,
			uid, taskID, title); err != nil {
			slog.Warn("Failed to log completion activity",
				"task_id", taskID, "error", err.Error())
			// Non-fatal: the move already happened.
		}
	}

	if err := transaction.Commit(); err != nil {
		slog.Error("Failed to commit status change",
			"task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}

	slog.Info("Task moved",
		"user_id", uid, "task_id", taskID,
		"from", fmt.Sprintf("%s@%d", oldStatus, oldPosition),
		"to", fmt.Sprintf("%s@%d", input.Status, input.Position))

	c.JSON(http.StatusOK, gin.H{
		"message": "Task moved",
		"task": gin.H{
			"id":         taskID,
			"status":     input.Status,
			"position":   input.Position,
			"updated_at": updatedAt,
		},
	})
}
