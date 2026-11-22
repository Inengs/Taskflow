package controllers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Inengs/realtime-task-app/models"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

const maxMyTasks = 200

// ListMyTasks handles GET /tasks/mine.
// Tasks assigned to the caller in projects they still belong to.
func ListMyTasks(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "GET /tasks/mine")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// ?status= wins over the default "hide done" behaviour.
	status := c.Query("status")
	if status != "" && !validTaskStatuses[status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid status: %q", status)})
		return
	}
	includeDone := false
	if v := c.Query("include_done"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "include_done must be true or false"})
			return
		}
		includeDone = b
	}

	// Joining project_members keeps this correct even if an assignment outlived a membership.
	// Fetch one extra row to detect truncation.
	rows, err := db.Query(`
		SELECT t.id, t.project_id, p.name, t.title, COALESCE(t.description, ''),
		       t.status, t.priority, t.due_date, t.position, t.created_at, t.updated_at,
		       (SELECT COUNT(*) FROM subtasks s WHERE s.task_id = t.id),
		       (SELECT COUNT(*) FROM subtasks s WHERE s.task_id = t.id AND s.done),
		       (SELECT COUNT(*) FROM task_comments cm WHERE cm.task_id = t.id)
		FROM task_assignees ta
		JOIN tasks t ON t.id = ta.task_id
		JOIN projects p ON p.id = t.project_id
		JOIN project_members pm ON pm.project_id = t.project_id AND pm.user_id = $1
		WHERE ta.user_id = $1
		  AND (
		        ($2::text <> '' AND t.status = $2::text)
		     OR ($2::text = '' AND ($3::boolean OR t.status <> 'done'))
		      )
		ORDER BY t.due_date ASC NULLS LAST,
		         CASE t.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END,
		         t.created_at DESC, t.id DESC
		LIMIT $4`, uid, status, includeDone, maxMyTasks+1)
	if err != nil {
		slog.Error("Failed to list my tasks", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
		return
	}
	defer rows.Close()

	tasks := []models.MyTask{} // [] not null when empty
	ids := make([]int, 0, maxMyTasks)
	for rows.Next() {
		var t models.MyTask
		t.Assignees = []models.TaskUserBrief{}
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.ProjectName, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.DueDate, &t.Position, &t.CreatedAt, &t.UpdatedAt,
			&t.SubtaskTotal, &t.SubtaskDone, &t.CommentCount); err != nil {
			slog.Error("Failed to scan my task", "user_id", uid, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
			return
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating my tasks", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
		return
	}

	truncated := len(tasks) > maxMyTasks
	if truncated {
		tasks = tasks[:maxMyTasks]
	}
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}

	// Second query: every assignee of these tasks (the board shows avatars, not just me).
	if len(ids) > 0 {
		aRows, err := db.Query(`
			SELECT ta.task_id, u.id, u.username, COALESCE(u.avatar_url, '')
			FROM task_assignees ta
			JOIN users u ON u.id = ta.user_id
			WHERE ta.task_id = ANY($1::int[])
			ORDER BY ta.assigned_at, u.id`, pq.Array(ids))
		if err != nil {
			slog.Error("Failed to load assignees for my tasks", "user_id", uid, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
			return
		}
		defer aRows.Close()

		byTask := make(map[int][]models.TaskUserBrief, len(ids))
		for aRows.Next() {
			var taskID int
			var u models.TaskUserBrief
			if err := aRows.Scan(&taskID, &u.ID, &u.Username, &u.AvatarURL); err != nil {
				slog.Error("Failed to scan assignee", "user_id", uid, "error", err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
				return
			}
			byTask[taskID] = append(byTask[taskID], u)
		}
		if err := aRows.Err(); err != nil {
			slog.Error("Error iterating assignees", "user_id", uid, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
			return
		}
		for i := range tasks {
			if a, ok := byTask[tasks[i].ID]; ok {
				tasks[i].Assignees = a
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tasks fetched", "tasks": tasks, "truncated": truncated})
}
