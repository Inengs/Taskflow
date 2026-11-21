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
)

const (
	defaultCommentsPage = 20
	maxCommentsPage     = 50
)

var commentValidator = validator.New()

// ListTaskComments handles GET /tasks/:id/comments?limit=20&before=<comment_id>.
// Newest first. Pass the returned next_before as `before` to load earlier comments.
func ListTaskComments(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "GET /tasks/:id/comments")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	limit := defaultCommentsPage
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxCommentsPage {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 50"})
			return
		}
		limit = n
	}
	before := 0 // 0 = start from the newest
	if v := c.Query("before"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid before cursor"})
			return
		}
		before = n
	}

	// The URL has no project_id: resolve it from the task, then authorize on that.
	var projectID int
	err = db.QueryRow(`SELECT project_id FROM tasks WHERE id = $1`, taskID).Scan(&projectID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load task for comments", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}
	role, err := utils.RequireProjectRole(db, projectID, uid, "member")
	if err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}
	isAdmin := memberRank(role) >= memberRank("admin")

	// Fetch one extra row to learn whether an earlier page exists.
	rows, err := db.Query(`
		SELECT c.id, c.body, c.created_at, c.edited_at, c.user_id,
		       COALESCE(u.username, ''), COALESCE(u.avatar_url, '')
		FROM task_comments c
		LEFT JOIN users u ON u.id = c.user_id
		WHERE c.task_id = $1 AND ($2::int = 0 OR c.id < $2::int)
		ORDER BY c.id DESC
		LIMIT $3`, taskID, before, limit+1)
	if err != nil {
		slog.Error("Failed to list comments", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}
	defer rows.Close()

	comments := []models.TaskComment{} // [] not null when empty
	for rows.Next() {
		var cm models.TaskComment
		var authorID sql.NullInt64 // NULL if the author's account was deleted
		var authorName, authorAvatar string
		if err := rows.Scan(&cm.ID, &cm.Body, &cm.CreatedAt, &cm.EditedAt,
			&authorID, &authorName, &authorAvatar); err != nil {
			slog.Error("Failed to scan comment", "task_id", taskID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
			return
		}
		isAuthor := authorID.Valid && int(authorID.Int64) == uid
		if authorID.Valid {
			cm.Author = &models.TaskUserBrief{
				ID: int(authorID.Int64), Username: authorName, AvatarURL: authorAvatar,
			}
		}
		cm.CanEdit = isAuthor              // matches PUT /comments/:id
		cm.CanDelete = isAuthor || isAdmin // matches DELETE /comments/:id
		comments = append(comments, cm)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating comments", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	hasMore := len(comments) > limit
	var nextBefore *int // null when there is no earlier page
	if hasMore {
		comments = comments[:limit]
		oldest := comments[len(comments)-1].ID
		nextBefore = &oldest
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Comments fetched",
		"comments":    comments,
		"has_more":    hasMore,
		"next_before": nextBefore,
	})
}

// CreateTaskComment handles POST /tasks/:id/comments. Any project member can comment.
func CreateTaskComment(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "POST /tasks/:id/comments")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var input models.CreateCommentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	input.Body = strings.TrimSpace(input.Body) // whitespace-only becomes empty and fails "required"

	if err := commentValidator.Struct(input); err != nil {
		var errs []string
		for _, e := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", e.Field(), e.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	// The URL has no project_id: resolve it from the task, then authorize on that.
	var projectID int
	err = db.QueryRow(`SELECT project_id FROM tasks WHERE id = $1`, taskID).Scan(&projectID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load task for comment", "task_id", taskID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add comment"})
		return
	}
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	// Insert and fetch the author in one round trip.
	var cm models.TaskComment
	var username, avatar string
	err = db.QueryRow(`
		WITH ins AS (
			INSERT INTO task_comments (task_id, user_id, body)
			VALUES ($1, $2, $3)
			RETURNING id, body, created_at, edited_at
		)
		SELECT ins.id, ins.body, ins.created_at, ins.edited_at,
		       u.username, COALESCE(u.avatar_url, '')
		FROM ins JOIN users u ON u.id = $2`,
		taskID, uid, input.Body,
	).Scan(&cm.ID, &cm.Body, &cm.CreatedAt, &cm.EditedAt, &username, &avatar)
	if err != nil {
		slog.Error("Failed to insert comment",
			"task_id", taskID, "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add comment"})
		return
	}
	cm.Author = &models.TaskUserBrief{ID: uid, Username: username, AvatarURL: avatar}
	cm.CanEdit = true   // the author
	cm.CanDelete = true // the author

	// TODO(notifications): notify the task's assignees and creator (never the commenter).
	// TODO(realtime): broadcast the new comment to the project room.

	c.JSON(http.StatusCreated, gin.H{"message": "Comment added", "comment": cm})
}

// UpdateComment handles PUT /comments/:id. Author only.
func UpdateComment(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "PUT /comments/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	commentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return
	}

	var input models.CreateCommentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	input.Body = strings.TrimSpace(input.Body)

	if err := commentValidator.Struct(input); err != nil {
		var errs []string
		for _, e := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", e.Field(), e.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	// Resolve the comment's author and project, then authorize.
	var authorID sql.NullInt64 // NULL if the author's account was deleted
	var projectID int
	err = db.QueryRow(`
		SELECT c.user_id, t.project_id
		FROM task_comments c
		JOIN tasks t ON t.id = c.task_id
		WHERE c.id = $1`, commentID).Scan(&authorID, &projectID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load comment", "comment_id", commentID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update comment"})
		return
	}

	// Must still be a member (404 for outsiders), and must be the author (403 for everyone else).
	if _, err := utils.RequireProjectRole(db, projectID, uid, "member"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}
	if !authorID.Valid || int(authorID.Int64) != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the author can edit a comment"})
		return
	}

	// edited_at only changes when the text actually changed. user_id is re-checked in the
	// WHERE as a guard.
	var cm models.TaskComment
	var username, avatar string
	err = db.QueryRow(`
		WITH upd AS (
			UPDATE task_comments
			SET body = $2::text,
			    edited_at = CASE WHEN body IS DISTINCT FROM $2::text THEN NOW() ELSE edited_at END
			WHERE id = $1 AND user_id = $3
			RETURNING id, body, created_at, edited_at
		)
		SELECT upd.id, upd.body, upd.created_at, upd.edited_at,
		       u.username, COALESCE(u.avatar_url, '')
		FROM upd JOIN users u ON u.id = $3`,
		commentID, input.Body, uid,
	).Scan(&cm.ID, &cm.Body, &cm.CreatedAt, &cm.EditedAt, &username, &avatar)
	if err == sql.ErrNoRows { // deleted between the check and now
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to update comment", "comment_id", commentID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update comment"})
		return
	}
	cm.Author = &models.TaskUserBrief{ID: uid, Username: username, AvatarURL: avatar}
	cm.CanEdit = true
	cm.CanDelete = true

	// TODO(realtime): broadcast the edit to the project room.
	c.JSON(http.StatusOK, gin.H{"message": "Comment updated", "comment": cm})
}

// DeleteComment handles DELETE /comments/:id. Author, or admin and above.
func DeleteComment(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "DELETE /comments/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	commentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return
	}

	var authorID sql.NullInt64 // NULL if the author's account was deleted
	var projectID int
	err = db.QueryRow(`
		SELECT c.user_id, t.project_id
		FROM task_comments c
		JOIN tasks t ON t.id = c.task_id
		WHERE c.id = $1`, commentID).Scan(&authorID, &projectID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load comment", "comment_id", commentID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete comment"})
		return
	}

	// Members get in (404 for outsiders); non-authors must also be admin+ (403).
	role, err := utils.RequireProjectRole(db, projectID, uid, "member")
	if err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}
	isAuthor := authorID.Valid && int(authorID.Int64) == uid
	if !isAuthor && memberRank(role) < memberRank("admin") {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to do that"})
		return
	}

	var deletedID int
	err = db.QueryRow(`DELETE FROM task_comments WHERE id = $1 RETURNING id`, commentID).Scan(&deletedID)
	if err == sql.ErrNoRows { // already deleted by someone else
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to delete comment", "comment_id", commentID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete comment"})
		return
	}

	// TODO(realtime): broadcast the deletion to the project room.
	c.JSON(http.StatusOK, gin.H{"message": "Comment deleted"})
}
