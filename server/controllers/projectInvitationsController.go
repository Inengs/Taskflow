package controllers

import (
	"database/sql"
	"errors"
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

var invitationValidator = validator.New()

const maxPendingInvitations = 100 // per project; stops invite spam

// CreateProjectInvitation handles POST /projects/:id/invitations.
// Admin+ can invite as member; only the owner can invite as admin.
func CreateProjectInvitation(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "POST /projects/:id/invitations")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	callerRole, err := utils.RequireProjectRole(db, projectID, uid, "admin")
	if err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	var input models.CreateInvitationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if input.Role == "" {
		input.Role = "member"
	}

	if err := invitationValidator.Struct(input); err != nil {
		var errs []string
		for _, e := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", e.Field(), e.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	// Role escalation guard: admins can only bring in plain members.
	if input.Role == "admin" && callerRole != "owner" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the owner can invite admins"})
		return
	}

	// Already a member? (matched through the account's email)
	var alreadyMember bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM project_members pm
			JOIN users u ON u.id = pm.user_id
			WHERE pm.project_id = $1 AND lower(u.email) = $2
		)`, projectID, input.Email).Scan(&alreadyMember)
	if err != nil {
		slog.Error("Failed to check existing membership", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}
	if alreadyMember {
		c.JSON(http.StatusConflict, gin.H{"error": "That person is already a member"})
		return
	}

	transaction, err := db.Begin()
	if err != nil {
		slog.Error("Failed to begin transaction", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}
	defer transaction.Rollback() // no-op after Commit

	// An expired pending invite would still hold the unique slot: retire it first.
	if _, err := transaction.Exec(`
		UPDATE project_invitations
		SET status = 'revoked', responded_at = NOW()
		WHERE project_id = $1 AND email = $2 AND status = 'pending' AND expires_at < NOW()`,
		projectID, input.Email); err != nil {
		slog.Error("Failed to retire expired invitation", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}

	var pending int
	if err := transaction.QueryRow(`
		SELECT COUNT(*) FROM project_invitations
		WHERE project_id = $1 AND status = 'pending' AND expires_at >= NOW()`,
		projectID).Scan(&pending); err != nil {
		slog.Error("Failed to count pending invitations", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}
	if pending >= maxPendingInvitations {
		c.JSON(http.StatusConflict, gin.H{"error": "Too many pending invitations"})
		return
	}

	var inv models.ProjectInvitation
	err = transaction.QueryRow(`
		INSERT INTO project_invitations (project_id, email, role, invited_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, project_id, email, role, status, invited_by, created_at, expires_at`,
		projectID, input.Email, input.Role, uid,
	).Scan(&inv.ID, &inv.ProjectID, &inv.Email, &inv.Role, &inv.Status,
		&inv.InvitedBy, &inv.CreatedAt, &inv.ExpiresAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" { // unique violation
			c.JSON(http.StatusConflict, gin.H{"error": "That email already has a pending invitation"})
			return
		}
		slog.Error("Failed to insert invitation", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}

	if err := transaction.Commit(); err != nil {
		slog.Error("Failed to commit invitation", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}

	// TODO(notifications): email the invitee once the notification system is rebuilt.
	c.JSON(http.StatusCreated, gin.H{"message": "Invitation created", "invitation": inv})
}
