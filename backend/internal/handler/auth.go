package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminLogin 处理 POST /api/v1/admin/auth/login，签发 JWT。
func AdminLogin(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "用户名和密码必填", "data": nil})
			return
		}
		token, err := d.Auth.Login(req.Username, req.Password)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": err.Error(), "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"token": token}})
	}
}
