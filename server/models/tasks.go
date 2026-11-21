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

// TaskSubtask is a checklist item on a task.
type TaskSubtask struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Done     bool   `json:"done"`
	Position int    `json:"position"`
}

// TaskDetail is the full task view for the detail drawer.
// ProjectTask is embedded, so its fields (assignees, counts, dates) stay top-level in the JSON.
type TaskDetail struct {
	ProjectTask
	ProjectName string         `json:"project_name"`
	Creator     *TaskUserBrief `json:"creator"` // null if the creator's account was deleted
	Subtasks    []TaskSubtask  `json:"subtasks"`
	CanEdit     bool           `json:"can_edit"`
	CanDelete   bool           `json:"can_delete"`
}

// TaskComment is one comment as shown in the task drawer.
type TaskComment struct {
	ID        int            `json:"id"`
	Body      string         `json:"body"`
	CreatedAt time.Time      `json:"created_at"`
	EditedAt  *time.Time     `json:"edited_at"`
	Author    *TaskUserBrief `json:"author"` // null if the author's account was deleted
	CanEdit   bool           `json:"can_edit"`
	CanDelete bool           `json:"can_delete"`
}

// CreateCommentInput is the body for POST /tasks/:id/comments and PUT /comments/:id.
type CreateCommentInput struct {
	Body string `json:"body" validate:"required,max=5000"`
}