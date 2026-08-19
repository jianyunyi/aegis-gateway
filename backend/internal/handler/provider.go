package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListProviders 查询全部提供商。
func ListProviders(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ps, err := d.Providers.List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": ps})
	}
}

// CreateProvider 新建提供商（API Key 加密落库，不回显）。
func CreateProvider(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name     string `json:"name"`
			BaseURL  string `json:"base_url"`
			APIKey   string `json:"api_key"`
			Enabled  int8   `json:"enabled"`
			Priority int    `json:"priority"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.BaseURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "name 和 base_url 必填", "data": nil})
			return
		}
		if req.Enabled != 0 && req.Enabled != 1 {
			req.Enabled = 1
		}
		p, err := d.Providers.Create(req.Name, req.BaseURL, req.APIKey, req.Enabled, req.Priority)
		if err != nil {
			if isDuplicate(err) {
				c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "提供商名称已存在", "data": nil})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "创建失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": p})
	}
}
