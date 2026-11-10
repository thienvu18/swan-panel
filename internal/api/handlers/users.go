package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/db"
	"golang.org/x/crypto/bcrypt"
)

// UsersHandler manages Users, Approval Queue, Roles, and User Groups
type UsersHandler struct{}

// NewUsersHandler creates a new UsersHandler
func NewUsersHandler() *UsersHandler {
	return &UsersHandler{}
}

// ListUsers returns users with optional status filtering
func (h *UsersHandler) ListUsers(c *gin.Context) {
	status := c.Query("status")
	slog.Debug("Listing users directory", "status_filter", status)

	query := db.DB.Preload("UserGroups").Preload("Devices").Preload("IDPProvider")
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var users []db.User
	if err := query.Order("created_at desc").Find(&users).Error; err != nil {
		slog.Error("Failed to query users", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

// CreateLocalUser creates a new manual user with email & password (US-03B)
func (h *UsersHandler) CreateLocalUser(c *gin.Context) {
	var req struct {
		Name         string   `json:"name" binding:"required"`
		Email        string   `json:"email" binding:"required"`
		Password     string   `json:"password" binding:"required"`
		IsAdmin      bool     `json:"is_admin"`
		UserGroupIDs []string `json:"user_group_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("Invalid create user payload", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	// Check if email already exists
	var existing db.User
	if err := db.DB.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		slog.Warn("Attempted to create user with existing email", "email", req.Email)
		c.JSON(http.StatusConflict, gin.H{"error": "Email already in use"})
		return
	}

	// Hash password with bcrypt
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("Failed to hash password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process credentials"})
		return
	}

	var groups []db.UserGroup
	if len(req.UserGroupIDs) > 0 {
		db.DB.Where("id IN ?", req.UserGroupIDs).Find(&groups)
	}

	newUser := db.User{
		ID:           "usr_" + uuid.New().String()[:8],
		Email:        req.Email,
		Name:         req.Name,
		AuthType:     "local",
		PasswordHash: string(hash),
		IsAdmin:      req.IsAdmin,
		Status:       "ACTIVE", // Local users created by admin are ACTIVE immediately
		UserGroups:   groups,
	}

	if err := db.DB.Create(&newUser).Error; err != nil {
		slog.Error("Failed to save local user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	slog.Info("Local user account created successfully", "id", newUser.ID, "email", newUser.Email, "is_admin", newUser.IsAdmin)
	c.JSON(http.StatusCreated, gin.H{"user": newUser})
}

// ToggleAdminRole promotes or demotes a user's administrator privileges (US-03)
func (h *UsersHandler) ToggleAdminRole(c *gin.Context) {
	id := c.Param("id")
	var user db.User
	if err := db.DB.Where("id = ?", id).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	user.IsAdmin = !user.IsAdmin
	if err := db.DB.Save(&user).Error; err != nil {
		slog.Error("Failed to toggle admin role", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}

	slog.Info("User admin privilege updated", "id", id, "email", user.Email, "is_admin", user.IsAdmin)
	c.JSON(http.StatusOK, gin.H{"user": user, "is_admin": user.IsAdmin})
}

// ApproveUser approves a candidate in the approval queue and assigns User Groups (US-02)
func (h *UsersHandler) ApproveUser(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		UserGroupIDs []string `json:"user_group_ids"`
	}
	_ = c.ShouldBindJSON(&req)

	var user db.User
	if err := db.DB.Where("id = ?", id).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Candidate not found"})
		return
	}

	user.Status = "ACTIVE"

	if len(req.UserGroupIDs) > 0 {
		var groups []db.UserGroup
		db.DB.Where("id IN ?", req.UserGroupIDs).Find(&groups)
		_ = db.DB.Model(&user).Association("UserGroups").Replace(groups)
	}

	if err := db.DB.Save(&user).Error; err != nil {
		slog.Error("Failed to approve user", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve user"})
		return
	}

	slog.Info("Candidate approved from queue, network access granted", "id", id, "email", user.Email)
	realtime.DefaultBroker.Broadcast("user_approved", map[string]interface{}{
		"user_id": user.ID,
		"email":   user.Email,
		"status":  user.Status,
	})

	c.JSON(http.StatusOK, gin.H{"user": user, "message": "User approved successfully"})
}

// RejectUser removes a pending candidate from the queue (US-02)
func (h *UsersHandler) RejectUser(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Where("id = ? AND status = ?", id, "PENDING").Delete(&db.User{}).Error; err != nil {
		slog.Error("Failed to reject candidate", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject candidate"})
		return
	}

	slog.Info("Candidate rejected from approval queue", "id", id)
	realtime.DefaultBroker.Broadcast("approval_queue_update", map[string]interface{}{"rejected_id": id})
	c.JSON(http.StatusOK, gin.H{"message": "Candidate rejected"})
}

// DeleteUser deletes a user and removes group associations
func (h *UsersHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	var user db.User
	if err := db.DB.Where("id = ?", id).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	_ = db.DB.Model(&user).Association("UserGroups").Clear()
	if err := db.DB.Delete(&user).Error; err != nil {
		slog.Error("Failed to delete user", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	slog.Info("User deleted", "id", id, "email", user.Email)
	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

// ListUserGroups returns all User Groups with member counts
func (h *UsersHandler) ListUserGroups(c *gin.Context) {
	slog.Debug("Listing user groups")
	var groups []db.UserGroup
	if err := db.DB.Preload("Users").Order("name asc").Find(&groups).Error; err != nil {
		slog.Error("Failed to query user groups", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user groups"})
		return
	}

	type GroupDTO struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		MemberCount int    `json:"member_count"`
	}

	var dtoList []GroupDTO
	for _, g := range groups {
		dtoList = append(dtoList, GroupDTO{
			ID:          g.ID,
			Name:        g.Name,
			Description: g.Description,
			MemberCount: len(g.Users),
		})
	}

	c.JSON(http.StatusOK, gin.H{"user_groups": dtoList})
}

// CreateUserGroup creates a new user group
func (h *UsersHandler) CreateUserGroup(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	grp := db.UserGroup{
		ID:          "ug_" + uuid.New().String()[:8],
		Name:        req.Name,
		Description: req.Description,
	}

	if err := db.DB.Create(&grp).Error; err != nil {
		slog.Error("Failed to create user group", "name", req.Name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create group"})
		return
	}

	slog.Info("User group created", "id", grp.ID, "name", grp.Name)
	c.JSON(http.StatusCreated, gin.H{"user_group": grp})
}

// DeleteUserGroup deletes a user group
func (h *UsersHandler) DeleteUserGroup(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Where("id = ?", id).Delete(&db.UserGroup{}).Error; err != nil {
		slog.Error("Failed to delete user group", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete group"})
		return
	}

	slog.Info("User group deleted", "id", id)
	c.JSON(http.StatusOK, gin.H{"message": "User group deleted successfully"})
}
