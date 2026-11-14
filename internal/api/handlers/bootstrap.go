package handlers

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/db"
)

func SetupStatus(c *gin.Context) {
	required, err := db.SetupRequired()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Setup status unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"setup_required": required})
}

func SetupAdmin(c *gin.Context) {
	var req struct {
		Token    string `json:"token" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Name     string `json:"name" binding:"required"`
		Password string `json:"password" binding:"required,min=12"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid setup data; password must be at least 12 characters"})
		return
	}
	address, err := mail.ParseAddress(req.Email)
	if err != nil || !strings.EqualFold(address.Address, req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email"})
		return
	}
	user, err := db.CreateInitialAdmin(req.Token, req.Name, req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Administrator created. Sign in to continue.", "user": gin.H{"id": user.ID, "email": user.Email, "name": user.Name}})
}
