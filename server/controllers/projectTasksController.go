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

// taskValidator is reused across task handlers. Building one per request is wasteful.
var taskValidator = validator.New()

// Whitelists mirror the DB check constraints. Defense-in-depth: even if a bug
// bypasses the validator, an invalid value never reaches the database.
var validTaskStatuses = map[string]bool{
	"pending":     true,
	"in-progress": true,
	"in-review":   true,
	"done":        true,
}

var validTaskPriorities = map[string]bool{
	"low":    true,
	"medium": true,
	"high":   true,
}

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

// CreateProjectTask creates a task in a project.
//
// Behavior:
//
//   - Any project member can create tasks.
//
//   - The new task appears at the TOP of its column (position 0); existing
//     tasks in the same column are shifted down by 1.
//
//   - Assignees must be project members — non-members are silently dropped.
//
//   - The creator is dropped from the assignee list (no self-assign).
//
//   - Each surviving assignee gets a notification AFTER the commit. A
//     notification failure does not roll back the task creation.
//
//   - Activity is logged inside the transaction so it rolls back with the task.
//
//     @Summary	Create task in project
//     @Tags		tasks
//     @Accept		json
//     @Produce	json
//     @Security	BearerAuth
//     @Param		id		path		int							true	"Project ID"
//     @Param		request	body		CreateProjectTaskInput		true	"Task"
//     @Success	201		{object}	map[string]any
//     @Failure	400		{object}	map[string]any
//     @Failure	403		{object}	map[string]any
//     @Failure	404		{object}	map[string]any
//     @Router		/projects/{id}/tasks [post]
func CreateProjectTask(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB) // Pulls the shared *sql.DB connection pool from Gin's context

	userID, exists := c.Get("user_id") // read user id from Gin context
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"}) // check for errors
		return
	}
	uid, ok := userID.(int) // type-assert from `any` to `int
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "POST /projects/:id/tasks")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"}) // handle errors
		return
	}

	projectID, err := strconv.Atoi(c.Param("id")) // get project id from the url parameter
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"}) // handles error
		return
	}

	// Any project member can create tasks. the person must be a member or higher
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	var input models.CreateProjectTaskInput // Gin parses JSON to the struct
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	input.Title = strings.TrimSpace(input.Title)             // trim whitespaces
	input.Description = strings.TrimSpace(input.Description) // trim unecessary white spaces

	if err := taskValidator.Struct(input); err != nil {
		// Runs the struct tags to validate accordingly
		var errs []string // returns all violations as a list — frontend can display them next to each field.
		for _, e := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", e.Field(), e.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	// Defaults + whitelist validation (defense-in-depth; the DB also enforces these).
	if input.Status == "" {
		input.Status = "pending"
	} else if !validTaskStatuses[input.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid status: %q", input.Status)})
		return
	}

	// Defaults + whitelist validation (defense-in-depth; the DB also enforces these). this is just to aid validation
	if input.Priority == "" {
		input.Priority = "medium"
	} else if !validTaskPriorities[input.Priority] {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid priority: %q", input.Priority)})
		return
	}

	// Deduplicate assignee IDs and exclude the creator (no self-assign).
	// when looking to assign a task, the list you may get from the frontend, might have duplicates, you have to select all the ids
	// without assigning to someone the same project twice, or assigning a project to yourself, since you are the one creating it
	seen := map[int]bool{uid: true}                   // Creates a Go map (like a JavaScript object) that we'll use as a checklist to track which user IDs we've already processed, and pre-seeds it with the creator's ID marked as "already seen" so the creator gets automatically skipped when we loop through the assignee list.
	requested := make([]int, 0, len(input.Assignees)) // creates an empty integer slice with length 0 but capacity pre-allocated to match the input size
	for _, id := range input.Assignees {
		if id > 0 && !seen[id] {
			seen[id] = true
			requested = append(requested, id)
		}
	}

	// Assignees must be project members — otherwise a member could assign a
	// task to an outsider who has no business seeing it.
	validAssignees := make([]int, 0, len(requested))
	if len(requested) > 0 {
		// Of these requested assignees, which ones are actually project members
		rows, err := db.Query(`
			SELECT user_id FROM project_members
			WHERE project_id = $1 AND user_id = ANY($2::int[])`,
			projectID, pq.Array(requested))
		if err != nil {
			// handle errors
			slog.Error("Failed to validate assignee membership",
				"project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
			return
		}

		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				slog.Error("Failed to scan assignee membership row", "error", err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
				return
			}
			validAssignees = append(validAssignees, id)
		}

		rows.Close()
		if err := rows.Err(); err != nil {
			slog.Error("Error iterating assignee membership rows", "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
			return
		}
		// Logs how many invalid assignees were silently dropped. useful for debugging
		if dropped := len(requested) - len(validAssignees); dropped > 0 {
			slog.Warn("Dropped non-member assignees",
				"project_id", projectID, "dropped", dropped)
		}
	}

	// Everything from here on must succeed or fail together. The whole task creation is atomic — either all of these succeed together, or none happen
	transaction, err := db.Begin()
	if err != nil {
		slog.Error("Failed to begin transaction", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}
	defer transaction.Rollback() // no-op after Commit

	// 1. Shift existing tasks in the same column down by 1, so the new task
	//    can sit at position 0 (top of the column).
	if _, err := transaction.Exec(`
		UPDATE tasks SET position = position + 1
		WHERE project_id = $1 AND status = $2`,
		projectID, input.Status); err != nil {
		slog.Error("Failed to shift task positions",
			"project_id", projectID, "status", input.Status, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}

	// 2. Insert the new task at position 0.
	var task models.ProjectTask
	task.Assignees = []models.TaskUserBrief{} // [] not null when empty
	err = transaction.QueryRow(`
		INSERT INTO tasks (project_id, user_id, title, description, status, priority, due_date, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0)
		RETURNING id, project_id, title, description, status, priority, due_date, position, created_at, updated_at`,
		projectID, uid, input.Title, input.Description, input.Status, input.Priority, input.DueDate,
	).Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.Status,
		&task.Priority, &task.DueDate, &task.Position, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		slog.Error("Failed to insert task",
			"project_id", projectID, "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}

	// 3. Insert assignees and fetch their username + avatar for the response.
	if len(validAssignees) > 0 {
		// One multi-row INSERT instead of N round trips.
		values := make([]string, len(validAssignees))
		args := make([]interface{}, 0, len(validAssignees)+1)
		args = append(args, task.ID)
		for i, assigneeID := range validAssignees {
			values[i] = fmt.Sprintf("($1, $%d)", i+2)
			args = append(args, assigneeID)
		}
		insertQuery := fmt.Sprintf(`
			INSERT INTO task_assignees (task_id, user_id) VALUES %s
			ON CONFLICT (task_id, user_id) DO NOTHING`,
			strings.Join(values, ","))
		if _, err := transaction.Exec(insertQuery, args...); err != nil {
			slog.Error("Failed to insert task assignees",
				"task_id", task.ID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
			return
		}

		// Fetch the assignees so the response matches ListProjectTasks' shape.
		assigneeRows, err := transaction.Query(`
			SELECT u.id, u.username, COALESCE(u.avatar_url, '')
			FROM task_assignees ta
			JOIN users u ON u.id = ta.user_id
			WHERE ta.task_id = $1
			ORDER BY ta.assigned_at, u.id`, task.ID)
		if err != nil {
			slog.Error("Failed to load task assignees",
				"task_id", task.ID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
			return
		}
		for assigneeRows.Next() {
			var u models.TaskUserBrief
			if err := assigneeRows.Scan(&u.ID, &u.Username, &u.AvatarURL); err != nil {
				assigneeRows.Close()
				slog.Error("Failed to scan assignee row", "error", err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
				return
			}
			task.Assignees = append(task.Assignees, u)
		}
		assigneeRows.Close()
		if err := assigneeRows.Err(); err != nil {
			slog.Error("Error iterating assignee rows", "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
			return
		}
	}

	// 4. Log activity inside the transaction so it rolls back with the task.
	if _, err := transaction.Exec(`
		INSERT INTO activity_log (user_id, entity_type, entity_id, action, target)
		VALUES ($1, 'task', $2, 'created', $3)`,
		uid, task.ID, task.Title); err != nil {
		slog.Warn("Failed to log activity", "task_id", task.ID, "error", err.Error())
		// Non-fatal: the task is created, we just couldn't log it.
	}

	if err := transaction.Commit(); err != nil {
		slog.Error("Failed to commit task creation",
			"project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}

	slog.Info("Task created",
		"user_id", uid, "project_id", projectID, "task_id", task.ID,
		"assignees", len(validAssignees))
	c.JSON(http.StatusCreated, gin.H{"message": "Task created", "task": task})
}
