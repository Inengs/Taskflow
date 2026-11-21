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

// ListProjectMembers handles GET /projects/:id/members.
// Any project member can see the member list (it feeds the assignee picker).
func ListProjectMembers(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "GET /projects/:id/members")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	rows, err := db.Query(`
		SELECT u.id, u.username, COALESCE(u.avatar_url, ''), pm.role
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = $1
		ORDER BY
			CASE pm.role WHEN 'owner' THEN 1 WHEN 'admin' THEN 2 ELSE 3 END,
			pm.joined_at,
			u.id`, projectID)
	if err != nil {
		slog.Error("Failed to list project members", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch members"})
		return
	}
	defer rows.Close()

	members := []models.ProjectMember{} // [] not null when empty
	for rows.Next() {
		var m models.ProjectMember
		if err := rows.Scan(&m.ID, &m.Username, &m.AvatarURL, &m.Role); err != nil {
			slog.Error("Failed to scan project member", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch members"})
			return
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating project members", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch members"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Members fetched", "members": members})
}


// UpdateProjectMemberRole handles PATCH /projects/:id/members/:userId. Owner only.
func UpdateProjectMemberRole(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "PATCH /projects/:id/members/:userId")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}
	targetID, err := strconv.Atoi(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var input models.UpdateMemberRoleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if input.Role != "admin" && input.Role != "member" && input.Role != "owner" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role must be admin, member or owner"})
		return
	}

	if _, err := utils.RequireProjectRole(db, projectID, uid, "owner"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	// The owner acting on themselves.
	if targetID == uid {
		if input.Role == "owner" { // already the owner: nothing to do
			c.JSON(http.StatusOK, gin.H{"message": "No change", "member": gin.H{"id": uid, "role": "owner"}})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Owners can't change their own role. Transfer ownership first."})
		return
	}

	transaction, err := db.Begin()
	if err != nil {
		slog.Error("Failed to begin transaction", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}
	defer transaction.Rollback() // no-op after Commit

	// Lock both rows (fixed order avoids deadlocks) and re-check roles under the lock:
	// the owner check above ran outside the transaction.
	rows, err := transaction.Query(`
		SELECT user_id, role FROM project_members
		WHERE project_id = $1 AND user_id IN ($2, $3)
		ORDER BY user_id
		FOR UPDATE`, projectID, uid, targetID)
	if err != nil {
		slog.Error("Failed to lock member rows", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}
	roles := map[int]string{}
	for rows.Next() {
		var id int
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			rows.Close()
			slog.Error("Failed to scan member row", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
			return
		}
		roles[id] = role
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating member rows", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}

	if roles[uid] != "owner" { // ownership changed since the first check
		c.JSON(http.StatusConflict, gin.H{"error": "Project ownership has changed"})
		return
	}
	targetRole, isMember := roles[targetID]
	if !isMember {
		c.JSON(http.StatusNotFound, gin.H{"error": "Member not found"})
		return
	}

	if input.Role == "owner" {
		// ─── Ownership transfer ───
		if targetRole != "admin" {
			c.JSON(http.StatusConflict, gin.H{"error": "Promote them to admin before transferring ownership"})
			return
		}
		if _, err := transaction.Exec(`
			UPDATE project_members SET role = 'owner'
			WHERE project_id = $1 AND user_id = $2`, projectID, targetID); err != nil {
			slog.Error("Failed to promote new owner", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to transfer ownership"})
			return
		}
		if _, err := transaction.Exec(`
			UPDATE project_members SET role = 'admin'
			WHERE project_id = $1 AND user_id = $2`, projectID, uid); err != nil {
			slog.Error("Failed to demote previous owner", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to transfer ownership"})
			return
		}
		var projectName string
		if err := transaction.QueryRow(`
			UPDATE projects SET user_id = $2 WHERE id = $1 RETURNING name`,
			projectID, targetID).Scan(&projectName); err != nil {
			slog.Error("Failed to update project owner", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to transfer ownership"})
			return
		}
		if err := transaction.Commit(); err != nil {
			slog.Error("Failed to commit transfer", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to transfer ownership"})
			return
		}

		utils.LogActivity(db, uid, "project", projectID, "ownership_transferred", projectName, nil)
		// TODO(notifications): tell the new owner once the notification system is rebuilt.

		c.JSON(http.StatusOK, gin.H{
			"message":   "Ownership transferred",
			"member":    gin.H{"id": targetID, "role": "owner"},
			"your_role": "admin",
		})
		return
	}

	// ─── Plain role change (admin <-> member) ───
	if targetRole == input.Role {
		c.JSON(http.StatusOK, gin.H{"message": "No change", "member": gin.H{"id": targetID, "role": targetRole}})
		return
	}
	if _, err := transaction.Exec(`
		UPDATE project_members SET role = $3
		WHERE project_id = $1 AND user_id = $2`, projectID, targetID, input.Role); err != nil {
		slog.Error("Failed to update member role", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}
	if err := transaction.Commit(); err != nil {
		slog.Error("Failed to commit role change", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}

	// No activity log for plain role changes: feed noise.
	c.JSON(http.StatusOK, gin.H{
		"message": "Role updated",
		"member":  gin.H{"id": targetID, "role": input.Role},
	})
}