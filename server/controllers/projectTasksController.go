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

// Safety cap: the board loads a project's tasks in one call and filters in the browser.
const maxProjectTasks = 500

// Query 1: the tasks, with subtask and comment counts.
const listProjectTasksSQL = `
	SELECT t.id, t.project_id, t.title, COALESCE(t.description, ''), t.status,
	       COALESCE(t.priority, 'medium'), t.due_date, t.position, t.created_at, t.updated_at,
	       COALESCE(st.total, 0), COALESCE(st.done, 0), COALESCE(cc.total, 0)
	FROM tasks t
	LEFT JOIN LATERAL (
		SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE done) AS done
		FROM subtasks WHERE task_id = t.id
	) st ON true
	LEFT JOIN LATERAL (
		SELECT COUNT(*) AS total FROM task_comments WHERE task_id = t.id
	) cc ON true
	WHERE t.project_id = $1
	ORDER BY t.position, t.created_at DESC
	LIMIT $2`

// Query 2: every assignee of every task in the project, in one round trip.
const listProjectTaskAssigneesSQL = `
	SELECT ta.task_id, u.id, u.username, COALESCE(u.avatar_url, '')
	FROM task_assignees ta
	JOIN tasks t ON t.id = ta.task_id
	JOIN users u ON u.id = ta.user_id
	WHERE t.project_id = $1
	ORDER BY ta.task_id, ta.assigned_at, u.id`

// ListProjectTasks returns all tasks of a project for the board, list and calendar views.
//
//	@Summary	List a project's tasks
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"Project ID"
//	@Success	200	{object}	map[string]any
//	@Failure	404	{object}	map[string]any
//	@Router		/projects/{id}/tasks [get]
func ListProjectTasks(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "GET /projects/:id/tasks")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	// Any member can see all of the project's tasks.
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	// to list the project tasks, but put a cap of 500 for it
	rows, err := db.Query(listProjectTasksSQL, projectID, maxProjectTasks+1) // +1 detects truncation
	if err != nil {
		slog.Error("Failed to query project tasks", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tasks"})
		return
	}
	defer rows.Close()

	// scan the rows queried and add them to the struct
	tasks := []models.ProjectTask{} // [] not null when empty
	for rows.Next() {
		var t models.ProjectTask
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.Status,
			&t.Priority, &t.DueDate, &t.Position, &t.CreatedAt, &t.UpdatedAt,
			&t.SubtaskTotal, &t.SubtaskDone, &t.CommentCount); err != nil {
			slog.Error("Failed to scan task row", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tasks"})
			return
		}
		t.Assignees = []models.TaskUserBrief{}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating task rows", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tasks"})
		return
	}

	truncated := len(tasks) > maxProjectTasks
	if truncated {
		tasks = tasks[:maxProjectTasks]
	}

	index := make(map[int]int, len(tasks)) // task id -> position in tasks
	for i, t := range tasks {
		index[t.ID] = i
	}

	// list the assignees for the task, using sql from the postgres db
	assigneerows, err := db.Query(listProjectTaskAssigneesSQL, projectID)
	if err != nil {
		slog.Error("Failed to query task assignees", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tasks"})
		return
	}
	defer assigneerows.Close()

	// map the values to the struct
	for assigneerows.Next() {
		var taskID int
		var u models.TaskUserBrief
		if err := assigneerows.Scan(&taskID, &u.ID, &u.Username, &u.AvatarURL); err != nil {
			slog.Error("Failed to scan assignee row", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tasks"})
			return
		}
		if i, found := index[taskID]; found { // skips tasks cut off by the cap
			tasks[i].Assignees = append(tasks[i].Assignees, u)
		}
	}

	if err := assigneerows.Err(); err != nil {
		slog.Error("Error iterating assignee rows", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load tasks"})
		return
	}

	slog.Info("Project tasks loaded", "user_id", uid, "project_id", projectID, "count", len(tasks))
	c.JSON(http.StatusOK, gin.H{"message": "Tasks loaded", "tasks": tasks, "truncated": truncated})
}
