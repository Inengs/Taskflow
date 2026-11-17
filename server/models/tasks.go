package models

import "time"

type Task struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	UserID      int        `json:"user_id"`     // Creator
	AssigneeID  *int       `json:"assignee_id"` // Who's responsible (nullable — task can be unassigned)
	ProjectID   int        `json:"project_id"`
	DueDate     *time.Time `json:"due_date"` // Nullable — task may have no deadline
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type TaskInput struct {
	Title       string     `json:"title" binding:"required" validate:"required"`
	Description string     `json:"description"`
	Status      string     `json:"status" binding:"required" validate:"required,oneof=pending in-progress done"`
	Priority    string     `json:"priority" validate:"omitempty,oneof=low medium high"`
	AssigneeID  *int       `json:"assignee_id" validate:"omitempty,gt=0"`
	ProjectID   int        `json:"project_id" binding:"required" validate:"required,gt=0"`
	DueDate     *time.Time `json:"due_date"`
}

type StatusInput struct {
	Status string `json:"status" binding:"required" validate:"required,oneof=pending in-progress done"`
}

// CreateProjectTaskInput is the request body for POST /projects/:id/tasks.
type CreateProjectTaskInput struct {
	Title       string     `json:"title" binding:"required" validate:"required,max=255"`
	Description string     `json:"description"`
	Status      string     `json:"status"`    // optional, defaults to "pending"
	Priority    string     `json:"priority"`  // optional, defaults to "medium"
	DueDate     *time.Time `json:"due_date"`  // optional, nullable
	Assignees   []int      `json:"assignees"` // optional, list of user IDs
}

// UpdateTaskStatusInput is the request body for PATCH /tasks/:id/status.
type UpdateTaskStatusInput struct {
	Status   string `json:"status" binding:"required"`
	Position int    `json:"position" binding:"min=0"`
}

// UpdateTaskInput is the body for PUT /tasks/:id.
// Assignees: nil = leave unchanged, [] = clear, [..] = replace the set.
type UpdateTaskInput struct {
	Title       string     `json:"title" validate:"required,max=255"`
	Description string     `json:"description" validate:"max=10000"`
	Priority    string     `json:"priority" validate:"required"`
	DueDate     *time.Time `json:"due_date"` // null clears it
	Assignees   *[]int     `json:"assignees"`
}
