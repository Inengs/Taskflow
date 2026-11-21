package models

import "time"

// Project represents a project entity in the database
type Project struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UserID      int       `json:"user_id"` // Creator or owner
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProjectInput is used for creating/updating a project from a request body
type ProjectInput struct {
	Name        string `json:"name" binding:"required" validate:"required,max=255"`
	Description string `json:"description"`
}

type ProjectListItem struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Role        string          `json:"role"`
	TaskCount   int             `json:"task_count"`
	DoneCount   int             `json:"done_count"`
	MemberCount int             `json:"member_count"`
	Members     []TaskUserBrief `json:"members"` // up to 4, for the avatar stack
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ProjectDetail powers the header of a project's board page.
type ProjectDetail struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Role        string          `json:"role"` // the caller's role
	MemberCount int             `json:"member_count"`
	Members     []TaskUserBrief `json:"members"` // first 8
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ProjectTask is one card on the board (GET /projects/:id/tasks).
type ProjectTask struct {
	ID           int             `json:"id"`
	ProjectID    int             `json:"project_id"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Status       string          `json:"status"` // pending | in-progress | in-review | done
	Priority     string          `json:"priority"`
	DueDate      *time.Time      `json:"due_date"`
	Position     int             `json:"position"`
	Assignees    []TaskUserBrief `json:"assignees"`
	SubtaskTotal int             `json:"subtask_total"`
	SubtaskDone  int             `json:"subtask_done"`
	CommentCount int             `json:"comment_count"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// ProjectMember is a member as shown in the members list and assignee picker.
type ProjectMember struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role"`
}

type ProjectInvitation struct {
	ID        int       `json:"id"`
	ProjectID int       `json:"project_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	InvitedBy *int      `json:"invited_by"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type CreateInvitationInput struct {
	Email string `json:"email" validate:"required,email,max=255"`
	Role  string `json:"role" validate:"omitempty,oneof=admin member"` // default: member
}

// ProjectInvitationItem is an invitation as shown in the admin list.
type ProjectInvitationItem struct {
	ProjectInvitation
	InvitedByUsername string `json:"invited_by_username"`
}

// MyInvitation is a pending invitation as seen by the invited user.
type MyInvitation struct {
	ID                int       `json:"id"`
	ProjectID         int       `json:"project_id"`
	ProjectName       string    `json:"project_name"`
	Role              string    `json:"role"`
	InvitedByUsername string    `json:"invited_by_username"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}
