package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var errLastAdministrator = errors.New("last active administrator")

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

	// Match bootstrap password policy. bcrypt silently truncates beyond 72 bytes.
	if len(req.Password) < 12 || len(req.Password) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be 12 to 72 bytes"})
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
		if err := db.DB.Where("id IN ?", req.UserGroupIDs).Find(&groups).Error; err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Cannot validate user groups"})
			return
		}
		if len(groups) != len(uniqueStringValues(req.UserGroupIDs)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown user group ID"})
			return
		}
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

// ToggleAdminRole serializes the last-admin check and the role update in one
// SQLite write transaction, preventing concurrent demotions from bypassing it.
func (h *UsersHandler) ToggleAdminRole(c *gin.Context) {
	id := c.Param("id")
	var user db.User
	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		if user.IsAdmin && user.Status == "ACTIVE" {
			var count int64
			if err := tx.Model(&db.User{}).Where("is_admin = ? AND status = ?", true, "ACTIVE").Count(&count).Error; err != nil {
				return err
			}
			if count <= 1 {
				return errLastAdministrator
			}
		}
		user.IsAdmin = !user.IsAdmin
		return tx.Save(&user).Error
	})
	if errors.Is(err, errLastAdministrator) {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot demote the last administrator"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		slog.Error("Failed to toggle admin role", "id", id, "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to update role"})
		return
	}
	slog.Info("User admin privilege updated", "id", id, "email", user.Email, "is_admin", user.IsAdmin)
	c.JSON(http.StatusOK, gin.H{"user": user, "is_admin": user.IsAdmin})
}

// ApproveUser approves a candidate in the approval queue and assigns User Groups (US-02)
func uniqueStringValues(values []string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" && !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}
	return unique
}

// ApproveUser writes membership and account status in a single transaction.
func (h *UsersHandler) ApproveUser(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		UserGroupIDs []string `json:"user_group_ids"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user group payload"})
			return
		}
	}
	var user db.User
	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		if req.UserGroupIDs != nil {
			ids := uniqueStringValues(req.UserGroupIDs)
			var groups []db.UserGroup
			if len(ids) > 0 {
				if err := tx.Where("id IN ?", ids).Find(&groups).Error; err != nil {
					return err
				}
				if len(groups) != len(ids) {
					return fmt.Errorf("unknown user group ID")
				}
			}
			if err := tx.Model(&user).Association("UserGroups").Replace(groups); err != nil {
				return err
			}
		}
		user.Status = "ACTIVE"
		return tx.Save(&user).Error
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not approve user: " + err.Error()})
		return
	}
	realtime.DefaultBroker.Broadcast("user_approved", map[string]any{"user_id": user.ID, "status": user.Status})
	if err := policy.Reconcile(c.Request.Context(), db.DB); err != nil {
		slog.Error("Approved user policy update pending", "user_id", user.ID, "error", err)
		c.JSON(http.StatusAccepted, gin.H{"user": user, "runtime_status": "pending", "message": "User approved, gateway update pending"})
		return
	}
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

// DeleteUser atomically removes membership/account while recording durable
// revocations for every issued VPN identity. External enforcement is retried
// after the transaction, without ever leaving an active deleted user's cert.
func (h *UsersHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	var user db.User
	var devices []db.Device
	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		if user.IsAdmin && user.Status == "ACTIVE" {
			var count int64
			if err := tx.Model(&db.User{}).Where("is_admin = ? AND status = ?", true, "ACTIVE").Count(&count).Error; err != nil {
				return err
			}
			if count <= 1 {
				return errLastAdministrator
			}
		}
		if err := tx.Where("user_id = ?", id).Find(&devices).Error; err != nil {
			return err
		}
		for _, device := range devices {
			if device.CertSerial == "" && !device.IsRevoked {
				return fmt.Errorf("device %s has no revocable certificate serial", device.ID)
			}
			if err := tx.Model(&device).Update("is_revoked", true).Error; err != nil {
				return err
			}
			if device.CertSerial != "" {
				record := db.CertificateRevocation{Serial: device.CertSerial, DeviceID: device.ID}
				record.RevokedAt = time.Now().UTC()
				if err := tx.Where("serial = ?", record.Serial).FirstOrCreate(&record).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&user).Association("UserGroups").Clear(); err != nil {
			return err
		}
		// Device rows and their revoked identities must remain for audit, but
		// the user foreign key must be detached before removing the account.
		if err := tx.Model(&db.Device{}).Where("user_id = ?", id).Update("user_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	})
	switch {
	case errors.Is(err, errLastAdministrator):
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot delete the last active administrator"})
		return
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	case err != nil:
		slog.Error("Atomic user deletion failed", "id", id, "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "User deletion failed; no changes committed"})
		return
	}
	var enforcementErrors []error
	for _, device := range devices {
		if device.CertSerial == "" {
			continue
		}
		if err := applyRevocation(c.Request.Context(), &device); err != nil {
			enforcementErrors = append(enforcementErrors, err)
		}
	}
	if err := policy.Reconcile(c.Request.Context(), db.DB); err != nil {
		enforcementErrors = append(enforcementErrors, err)
	}
	if len(enforcementErrors) != 0 {
		slog.Error("Deleted user revocations pending enforcement", "id", id, "error", errors.Join(enforcementErrors...))
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "message": "User deleted and revocations recorded; gateway enforcement pending"})
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
	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var grp db.UserGroup
		if err := tx.First(&grp, "id = ?", id).Error; err != nil {
			return err
		}
		var referenced int64
		if err := tx.Model(&db.AccessRule{}).Where("subject_type = ? AND subject_id = ?", "user_group", id).Count(&referenced).Error; err != nil {
			return err
		}
		if referenced > 0 {
			return fmt.Errorf("group is referenced by access rules; remove them first")
		}
		if err := tx.Model(&grp).Association("Users").Clear(); err != nil {
			return err
		}
		return tx.Delete(&grp).Error
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err := policy.Reconcile(c.Request.Context(), db.DB); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "error": "Group deleted but gateway policy is pending"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User group deleted successfully"})
}
