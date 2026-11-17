package controllers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Inengs/realtime-task-app/models"
	"github.com/Inengs/realtime-task-app/utils"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/lib/pq"
)

const maxTaskAssignees = 50

// loadProjectTask returns one task in the same shape as ListProjectTasks.
func loadProjectTask(db *sql.DB, taskID int) (models.ProjectTask, error) {
	var t models.ProjectTask
	t.Assignees = []models.TaskUserBrief{}

	err := db.QueryRow(`
		SELECT t.id, t.project_id, t.title, COALESCE(t.description, ''), t.status, t.priority,
		       t.due_date, t.position, t.created_at, t.updated_at,
		       (SELECT COUNT(*) FROM subtasks s WHERE s.task_id = t.id),
		       (SELECT COUNT(*) FROM subtasks s WHERE s.task_id = t.id AND s.done),
		       (SELECT COUNT(*) FROM task_comments cm WHERE cm.task_id = t.id)
		FROM tasks t WHERE t.id = $1`, taskID,
	).Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.Status, &t.Priority,
		&t.DueDate, &t.Position, &t.CreatedAt, &t.UpdatedAt,
		&t.SubtaskTotal, &t.SubtaskDone, &t.CommentCount)
	if err != nil {
		return t, err
	}

	rows, err := db.Query(`
		SELECT u.id, u.username, COALESCE(u.avatar_url, '')
		FROM task_assignees ta
		JOIN users u ON u.id = ta.user_id
		WHERE ta.task_id = $1
		ORDER BY ta.assigned_at, u.id`, taskID)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	for rows.Next() {
		var u models.TaskUserBrief
		if err := rows.Scan(&u.ID, &u.Username, &u.AvatarURL); err != nil {
			return t, err
		}
		t.Assignees = append(t.Assignees, u)
	}
	return t, rows.Err()
}

// UpdateTask handles PUT /tasks/:id. Any project member can edit.
func UpdateTask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "PUT /tasks/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var input models.UpdateTaskInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if err := taskValidator.Struct(input); err != nil {
		var errs []string
		for _, e := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", e.Field(), e.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}
	if !validTaskPriorities[input.Priority] {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid priority: %q", input.Priority)})
		return
	}
	if input.Assignees != nil && len(*input.Assignees) > maxTaskAssignees {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Too many assignees"})
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
		slog.Error("Failed to load task for update", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	// Assignees (only if provided): dedupe, then keep only project members.
	// Non-nil on purpose: pq.Array(nil slice) becomes NULL, which breaks the DELETE below.
	var validAssignees []int
	if input.Assignees != nil {
		seen := map[int]bool{}
		requested := make([]int, 0, len(*input.Assignees))
		for _, id := range *input.Assignees {
			if id > 0 && !seen[id] {
				seen[id] = true
				requested = append(requested, id)
			}
		}

		validAssignees = make([]int, 0, len(requested))
		if len(requested) > 0 {
			rows, err := db.Query(`
				SELECT user_id FROM project_members
				WHERE project_id = $1 AND user_id = ANY($2::int[])`,
				projectID, pq.Array(requested))
			if err != nil {
				slog.Error("Failed to validate assignee membership", "project_id", projectID, "error", err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
				return
			}
			for rows.Next() {
				var id int
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					slog.Error("Failed to scan assignee membership row", "error", err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
					return
				}
				validAssignees = append(validAssignees, id)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				slog.Error("Error iterating assignee membership rows", "error", err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
				return
			}
			if dropped := len(requested) - len(validAssignees); dropped > 0 {
				slog.Warn("Dropped non-member assignees", "project_id", projectID, "dropped", dropped)
			}
		}
	}

	tx, err := db.Begin()
	if err != nil {
		slog.Error("Failed to begin transaction", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}
	defer tx.Rollback() // no-op after Commit

	res, err := tx.Exec(`
		UPDATE tasks
		SET title = $1, description = $2, priority = $3, due_date = $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = $5`,
		input.Title, input.Description, input.Priority, input.DueDate, taskID)
	if err != nil {
		slog.Error("Failed to update task", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 { // deleted between the check and now
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	if input.Assignees != nil {
		// Remove assignees no longer in the set; keep the rest (preserves assigned_at).
		if _, err := tx.Exec(`
			DELETE FROM task_assignees
			WHERE task_id = $1 AND NOT (user_id = ANY($2::int[]))`,
			taskID, pq.Array(validAssignees)); err != nil {
			slog.Error("Failed to remove task assignees", "task_id", taskID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
			return
		}
		if len(validAssignees) > 0 {
			if _, err := tx.Exec(`
				INSERT INTO task_assignees (task_id, user_id)
				SELECT $1, UNNEST($2::int[])
				ON CONFLICT (task_id, user_id) DO NOTHING`,
				taskID, pq.Array(validAssignees)); err != nil {
				slog.Error("Failed to add task assignees", "task_id", taskID, "error", err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit task update", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}

	// No activity log for edits: it would flood the dashboard feed.
	// TODO(notifications): notify newly added assignees once the notification system is rebuilt.

	task, err := loadProjectTask(db, taskID)
	if err != nil {
		slog.Error("Failed to reload task after update", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Task updated but failed to load it"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task updated", "task": task})
}

// DeleteTask handles DELETE /tasks/:id. The task's creator or an admin+ can delete.
func DeleteTask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "DELETE /tasks/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var projectID int
	var creatorID sql.NullInt64 // NULL if the creator's account was deleted
	err = db.QueryRow(`SELECT project_id, user_id FROM tasks WHERE id = $1`, taskID).
		Scan(&projectID, &creatorID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load task for delete", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete task"})
		return
	}

	// Members get in (404 for outsiders); non-creators must also be admin+ (403).
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}
	isCreator := creatorID.Valid && int(creatorID.Int64) == uid
	if !isCreator {
		if _, err := utils.RequireProjectRole(db, projectID, uid, "admin"); err != nil {
			utils.RespondProjectAccessError(c, err)
			return
		}
	}

	tx, err := db.Begin()
	if err != nil {
		slog.Error("Failed to begin transaction", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete task"})
		return
	}
	defer tx.Rollback() // no-op after Commit

	// RETURNING gives the live status/position, not a possibly stale earlier read.
	var status, title string
	var position int
	err = tx.QueryRow(`
		DELETE FROM tasks WHERE id = $1
		RETURNING status, position, title`, taskID,
	).Scan(&status, &position, &title)
	if err == sql.ErrNoRows { // already deleted by someone else
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to delete task", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete task"})
		return
	}

	// Close the gap in its column. Assignees, subtasks and comments go via ON DELETE CASCADE.
	if _, err := tx.Exec(`
		UPDATE tasks SET position = position - 1
		WHERE project_id = $1 AND status = $2 AND position > $3`,
		projectID, status, position); err != nil {
		slog.Error("Failed to close position gap", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete task"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit task delete", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete task"})
		return
	}

	utils.LogActivity(db, uid, "task", taskID, "deleted", title, nil)

	c.JSON(http.StatusOK, gin.H{"message": "Task deleted"})
}
