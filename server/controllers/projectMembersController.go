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
