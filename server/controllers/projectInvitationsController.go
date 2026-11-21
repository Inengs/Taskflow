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

var errEmailNotVerified = errors.New("email not verified")

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

// ListProjectInvitations handles GET /projects/:id/invitations. Admin+ only.
func ListProjectInvitations(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "GET /projects/:id/invitations")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	if _, err := utils.RequireProjectRole(db, projectID, uid, "admin"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	rows, err := db.Query(`
		SELECT i.id, i.project_id, i.email, i.role, i.status, i.invited_by,
		       i.created_at, i.expires_at, COALESCE(u.username, '')
		FROM project_invitations i
		LEFT JOIN users u ON u.id = i.invited_by
		WHERE i.project_id = $1 AND i.status = 'pending' AND i.expires_at >= NOW()
		ORDER BY i.created_at DESC, i.id DESC
		LIMIT $2`, projectID, maxPendingInvitations)
	if err != nil {
		slog.Error("Failed to list invitations", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invitations"})
		return
	}
	defer rows.Close()

	invitations := []models.ProjectInvitationItem{} // [] not null when empty
	for rows.Next() {
		var inv models.ProjectInvitationItem
		if err := rows.Scan(&inv.ID, &inv.ProjectID, &inv.Email, &inv.Role, &inv.Status,
			&inv.InvitedBy, &inv.CreatedAt, &inv.ExpiresAt, &inv.InvitedByUsername); err != nil {
			slog.Error("Failed to scan invitation", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invitations"})
			return
		}
		invitations = append(invitations, inv)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating invitations", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invitations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitations fetched", "invitations": invitations})
}

// RevokeProjectInvitation handles DELETE /projects/:id/invitations/:invId. Admin+ only.
func RevokeProjectInvitation(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "DELETE /projects/:id/invitations/:invId")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}
	invID, err := strconv.Atoi(c.Param("invId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invitation ID"})
		return
	}

	if _, err := utils.RequireProjectRole(db, projectID, uid, "admin"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	// Scoped by project_id so an invitation from another project can't be touched via this URL.
	var revokedID int
	err = db.QueryRow(`
		UPDATE project_invitations
		SET status = 'revoked', responded_at = NOW()
		WHERE id = $1 AND project_id = $2 AND status = 'pending'
		RETURNING id`, invID, projectID).Scan(&revokedID)
	if err == sql.ErrNoRows { // wrong ID, wrong project, or no longer pending
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to revoke invitation",
			"project_id", projectID, "invitation_id", invID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation revoked"})
}

// ListMyInvitations handles GET /invitations/mine.
// Matches by the account's email, and only for verified emails.
func ListMyInvitations(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "GET /invitations/mine")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	var email string
	var verified bool
	err := db.QueryRow(`
		SELECT lower(email), COALESCE(verified, false) FROM users WHERE id = $1`, uid,
	).Scan(&email, &verified)
	if err != nil {
		slog.Error("Failed to load user email", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invitations"})
		return
	}

	invitations := []models.MyInvitation{} // [] not null when empty

	// Unverified email: never match invitations (anyone can register with any address).
	if !verified {
		c.JSON(http.StatusOK, gin.H{
			"message":        "Invitations fetched",
			"email_verified": false,
			"invitations":    invitations,
		})
		return
	}

	rows, err := db.Query(`
		SELECT i.id, i.project_id, p.name, i.role, COALESCE(u.username, ''),
		       i.created_at, i.expires_at
		FROM project_invitations i
		JOIN projects p ON p.id = i.project_id
		LEFT JOIN users u ON u.id = i.invited_by
		WHERE i.email = $1 AND i.status = 'pending' AND i.expires_at >= NOW()
		ORDER BY i.created_at DESC, i.id DESC
		LIMIT $2`, email, maxPendingInvitations)
	if err != nil {
		slog.Error("Failed to list my invitations", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invitations"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var inv models.MyInvitation
		if err := rows.Scan(&inv.ID, &inv.ProjectID, &inv.ProjectName, &inv.Role,
			&inv.InvitedByUsername, &inv.CreatedAt, &inv.ExpiresAt); err != nil {
			slog.Error("Failed to scan my invitation", "user_id", uid, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invitations"})
			return
		}
		invitations = append(invitations, inv)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating my invitations", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch invitations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Invitations fetched",
		"email_verified": true,
		"invitations":    invitations,
	})
}

// verifiedUserEmail returns the user's lowercased email, or errEmailNotVerified.
// Invitations are matched by email, so an unverified address must never match.
func verifiedUserEmail(db *sql.DB, uid int) (string, error) {
	var email string
	var verified bool
	if err := db.QueryRow(`
		SELECT lower(email), COALESCE(verified, false) FROM users WHERE id = $1`, uid,
	).Scan(&email, &verified); err != nil {
		return "", err
	}
	if !verified {
		return "", errEmailNotVerified
	}
	return email, nil
}

// AcceptInvitation handles POST /invitations/:id/accept.
func AcceptInvitation(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "POST /invitations/:id/accept")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	invID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invitation ID"})
		return
	}

	email, err := verifiedUserEmail(db, uid)
	if errors.Is(err, errEmailNotVerified) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Verify your email to respond to invitations"})
		return
	}
	if err != nil {
		slog.Error("Failed to load user email", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}

	transaction, err := db.Begin()
	if err != nil {
		slog.Error("Failed to begin transaction", "invitation_id", invID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}
	defer transaction.Rollback() // no-op after Commit

	// Lock the row so two simultaneous accepts can't both proceed.
	var projectID int
	var invEmail, role, status string
	var expired bool
	err = transaction.QueryRow(`
		SELECT project_id, email, role, status, expires_at < NOW()
		FROM project_invitations WHERE id = $1 FOR UPDATE`, invID,
	).Scan(&projectID, &invEmail, &role, &status, &expired)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load invitation", "invitation_id", invID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}
	if invEmail != email { // not yours: same answer as "doesn't exist"
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}
	if status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"error": "Invitation is no longer pending"})
		return
	}
	if expired {
		c.JSON(http.StatusGone, gin.H{"error": "Invitation has expired"})
		return
	}

	// Never overwrite an existing membership: an invite must not change a current role.
	if _, err := transaction.Exec(`
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, user_id) DO NOTHING`,
		projectID, uid, role); err != nil {
		slog.Error("Failed to add member", "project_id", projectID, "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}

	if _, err := transaction.Exec(`
		UPDATE project_invitations SET status = 'accepted', responded_at = NOW()
		WHERE id = $1`, invID); err != nil {
		slog.Error("Failed to mark invitation accepted", "invitation_id", invID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}

	var projectName, heldRole string
	if err := transaction.QueryRow(`
		SELECT p.name, pm.role
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = $2
		WHERE p.id = $1`, projectID, uid).Scan(&projectName, &heldRole); err != nil {
		slog.Error("Failed to load project after accept", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}

	if err := transaction.Commit(); err != nil {
		slog.Error("Failed to commit accept", "invitation_id", invID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to accept invitation"})
		return
	}

	utils.LogActivity(db, uid, "project", projectID, "joined", projectName, nil)
	// TODO(notifications): tell the inviter once the notification system is rebuilt.

	c.JSON(http.StatusOK, gin.H{
		"message": "Invitation accepted",
		"project": gin.H{"id": projectID, "name": projectName},
		"role":    heldRole,
	})
}


// DeclineInvitation handles POST /invitations/:id/decline.
func DeclineInvitation(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "POST /invitations/:id/decline")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	invID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invitation ID"})
		return
	}

	email, err := verifiedUserEmail(db, uid)
	if errors.Is(err, errEmailNotVerified) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Verify your email to respond to invitations"})
		return
	}
	if err != nil {
		slog.Error("Failed to load user email", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decline invitation"})
		return
	}

	var declinedID int
	err = db.QueryRow(`
		UPDATE project_invitations
		SET status = 'declined', responded_at = NOW()
		WHERE id = $1 AND email = $2 AND status = 'pending'
		RETURNING id`, invID, email).Scan(&declinedID)
	if err == sql.ErrNoRows { // missing, not yours, or no longer pending
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to decline invitation", "invitation_id", invID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decline invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation declined"})
}