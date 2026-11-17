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

// Reuse one validator; building it per request is wasteful.
var projectValidator = validator.New()

// ListProjects returns the projects the authenticated user is a member of.
//
//	@Summary	List projects
//	@Tags		projects
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]any	"message + projects array"
//	@Failure	401	{object}	map[string]any
//	@Router		/projects [get]
func ListProjects(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "/projects")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	rows, err := db.Query(`
	SELECT p.id, p.name, COALESCE(p.description, ''), pm.role,
	       COALESCE(tc.total, 0), COALESCE(tc.done, 0),
	       (SELECT COUNT(*) FROM project_members m WHERE m.project_id = p.id),
	       p.created_at, p.updated_at
	FROM project_members pm
	JOIN projects p ON p.id = pm.project_id
	LEFT JOIN LATERAL (
		SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE status = 'done') AS done
		FROM tasks WHERE project_id = p.id
	) tc ON true
	WHERE pm.user_id = $1
	ORDER BY p.updated_at DESC`, uid)
	if err != nil {
		slog.Error("Failed to query projects", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load projects"})
		return
	}
	defer rows.Close()

	projects := []models.ProjectListItem{} // empty slice, so JSON is [] not null
	index := map[int]int{}                 // project id -> position in projects
	for rows.Next() {
		var project models.ProjectListItem
		if err := rows.Scan(&project.ID, &project.Name, &project.Description, &project.Role, &project.TaskCount, &project.DoneCount, &project.MemberCount, &project.CreatedAt, &project.UpdatedAt); err != nil {
			slog.Error("Failed to scan project row", "user_id", uid, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load projects"})
			return
		}
		project.Members = []models.TaskUserBrief{}
		index[project.ID] = len(projects)
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating project rows", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load projects"})
		return
	}

	// Attach member avatars
	mrows, err := db.Query(`
	SELECT x.project_id, u.id, u.username, COALESCE(u.avatar_url, '')
	FROM (
		SELECT project_id, user_id,
		       ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY joined_at, user_id) AS rn
		FROM project_members
		WHERE project_id IN (SELECT project_id FROM project_members WHERE user_id = $1)
	) x
	JOIN users u ON u.id = x.user_id
	WHERE x.rn <= 4
	ORDER BY x.project_id, x.rn`, uid)
	if err != nil {
		slog.Error("Failed to query project members", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load projects"})
		return
	}
	defer mrows.Close()

	for mrows.Next() {
		var projectID int
		var m models.TaskUserBrief
		if err := mrows.Scan(&projectID, &m.ID, &m.Username, &m.AvatarURL); err != nil {
			slog.Error("Failed to scan member row", "user_id", uid, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load projects"})
			return
		}
		if i, found := index[projectID]; found {
			projects[i].Members = append(projects[i].Members, m)
		}
	}

	if err := mrows.Err(); err != nil {
		slog.Error("Error iterating member rows", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load projects"})
		return
	}

	slog.Info("Projects loaded", "user_id", uid, "count", len(projects))
	c.JSON(http.StatusOK, gin.H{"message": "Projects loaded", "projects": projects})
}

// ProjectDetails returns one project, the caller's role and a few members.
//
//	@Summary	Project details
//	@Tags		projects
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"Project ID"
//	@Success	200	{object}	map[string]any
//	@Failure	404	{object}	map[string]any
//	@Router		/projects/{id} [get]
func ProjectDetails(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int) // convert the
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "GET /projects/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	projectID, err := strconv.Atoi(c.Param("id")) // convert the param to an int
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	// Membership check first. Non-members get 404, so a project's existence isn't leaked.
	role, err := utils.RequireProjectRole(db, projectID, uid, "member")
	if err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	project := models.ProjectDetail{Role: role, Members: []models.TaskUserBrief{}} // creates the response struct
	err = db.QueryRow(`
		SELECT p.id, p.name, COALESCE(p.description, ''), p.created_at, p.updated_at,
		       (SELECT COUNT(*) FROM project_members WHERE project_id = p.id)
		FROM projects p
		WHERE p.id = $1`, projectID,
	).Scan(&project.ID, &project.Name, &project.Description,
		&project.CreatedAt, &project.UpdatedAt, &project.MemberCount)
	if err == sql.ErrNoRows { // deleted between the role check and here
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to load project", "user_id", uid, "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load project"})
		return
	}

	//this is to get the project members
	rows, err := db.Query(`
		SELECT u.id, u.username, COALESCE(u.avatar_url, '')
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = $1
		ORDER BY pm.joined_at, pm.user_id
		LIMIT 8`, projectID)
	if err != nil {
		slog.Error("Failed to load project members", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load project"})
		return
	}
	defer rows.Close()

	//It loops over the member rows the query returned in the previous code, copies each row into a TaskUserBrief and appends it to project.Members
	for rows.Next() {
		var m models.TaskUserBrief
		if err := rows.Scan(&m.ID, &m.Username, &m.AvatarURL); err != nil {
			slog.Error("Failed to scan member row", "project_id", projectID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load project"})
			return
		}
		project.Members = append(project.Members, m)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Error iterating member rows", "project_id", projectID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load project"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Project loaded", "project": project})
}

// CreateProject creates a project and makes the caller its owner.
//
//	@Summary	Create project
//	@Tags		projects
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		request	body		models.ProjectInput	true	"Project"
//	@Success	201		{object}	map[string]any
//	@Failure	400		{object}	map[string]any
//	@Router		/projects [post]
func CreateProject(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int) //convert to int
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "POST /projects")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	var input models.ProjectInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)               // "   " must not count as a name
	input.Description = strings.TrimSpace(input.Description) // trim the white spaces

	if err := projectValidator.Struct(input); err != nil {
		// validate the input gotten, by the model defined
		var errs []string
		for _, e := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", e.Field(), e.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	// Project + owner membership must succeed or fail together.
	transaction, err := db.Begin() // insert the row into projects and insert the row into project_members with role owner
	if err != nil {
		slog.Error("Failed to begin transaction", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}
	defer transaction.Rollback() // no-op once Commit succeeds

	var project models.Project
	// below we insert the new project row and then we read it back, all in one database call
	err = transaction.QueryRow(
		`INSERT INTO projects (name, description, user_id) VALUES ($1, $2, $3)
		 RETURNING id, user_id, name, description, created_at, updated_at`,
		input.Name, input.Description, uid,
	).Scan(&project.ID, &project.UserID, &project.Name, &project.Description, &project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		slog.Error("Failed to insert project", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

	// inserts the owner into the project_members section
	if _, err := transaction.Exec(
		`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'owner')`,
		project.ID, uid,
	); err != nil {
		slog.Error("Failed to add project owner", "user_id", uid, "project_id", project.ID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

	if err := transaction.Commit(); err != nil {
		slog.Error("Failed to commit project creation", "user_id", uid, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

	slog.Info("Project created", "user_id", uid, "project_id", project.ID)
	c.JSON(http.StatusCreated, gin.H{"message": "Project created successfully", "project": project})
}

// UpdateProject updates a project
//
//	@Summary	Update project
//	@Tags		projects
//	@Accept		json
//	@Produce	json
//	@Security	SessionCookie
//	@Param		id		path		int					true	"Project ID"
//	@Param		request	body		models.ProjectInput	true	"Project"
//	@Success	200		{object}	map[string]any
//	@Failure	404		{object}	map[string]any
//	@Router		/projects/{id} [put]
//
// UpdateProject handles PUT /projects/:id. Admin or owner only.
func UpdateProject(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "PUT /projects/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	if _, err := utils.RequireProjectRole(db, id, uid, "admin"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	var input models.ProjectInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)

	if err := projectValidator.Struct(input); err != nil {
		var errs []string
		for _, e := range err.(validator.ValidationErrors) {
			errs = append(errs, fmt.Sprintf("Field '%s' failed on '%s'", e.Field(), e.Tag()))
		}
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	var project models.Project
	err = db.QueryRow(`
		UPDATE projects
		SET name = $1, description = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
		RETURNING id, user_id, name, description, created_at, updated_at`,
		input.Name, input.Description, id,
	).Scan(&project.ID, &project.UserID, &project.Name, &project.Description,
		&project.CreatedAt, &project.UpdatedAt)
	if err == sql.ErrNoRows { // deleted between the role check and now
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to update project", "project_id", id, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project"})
		return
	}

	// TODO(notifications): notify members once the notification system is rebuilt.
	// TODO(realtime): broadcast to the project room, not just the acting user.
	manager.BroadcastProject(uid, project, "project_updated")

	c.JSON(http.StatusOK, gin.H{"message": "Project updated", "project": project})
}

// DeleteProject deletes a project and its tasks
//
//	@Summary	Delete project
//	@Tags		projects
//	@Produce	json
//	@Security	SessionCookie
//	@Param		id	path		int	true	"Project ID"
//	@Success	200	{object}	map[string]any
//	@Failure	404	{object}	map[string]any
//	@Router		/projects/{id} [delete]
//
// DeleteProject handles DELETE /projects/:id. Owner only.
// Tasks, assignees, subtasks, comments and members go via ON DELETE CASCADE.
func DeleteProject(c *gin.Context) {
	db := c.MustGet("db").(*sql.DB)

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	uid, ok := userID.(int)
	if !ok {
		slog.Error("user_id in context is not an int", "endpoint", "DELETE /projects/:id")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	if _, err := utils.RequireProjectRole(db, id, uid, "owner"); err != nil {
		utils.RespondProjectAccessError(c, err)
		return
	}

	// One statement is atomic, so the project and everything under it go together.
	var project models.Project
	err = db.QueryRow(`
		DELETE FROM projects WHERE id = $1
		RETURNING id, user_id, name, description, created_at, updated_at`, id,
	).Scan(&project.ID, &project.UserID, &project.Name, &project.Description,
		&project.CreatedAt, &project.UpdatedAt)
	if err == sql.ErrNoRows { // already deleted
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to delete project", "project_id", id, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete project"})
		return
	}

	utils.LogActivity(db, uid, "project", id, "deleted", project.Name, nil)

	// TODO(notifications): notify members once the notification system is rebuilt.
	// TODO(realtime): broadcast to the project room, not just the acting user.
	manager.BroadcastProject(uid, project, "project_deleted")

	c.JSON(http.StatusOK, gin.H{"message": "Project deleted"})
}
