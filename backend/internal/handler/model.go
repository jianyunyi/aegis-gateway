package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/model"
)

// ListModels 管理侧查询模型目录（含禁用项）。
func ListModels(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ms, err := d.Models.List(false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": ms})
	}
}

// CreateModel 新建模型目录项。
func CreateModel(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var m model.Model
		if err := c.ShouldBindJSON(&m); err != nil || m.Name == "" || m.ProviderID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "name 与 provider_id 必填", "data": nil})
			return
		}
		if m.Tier == "" {
			m.Tier = "normal"
		}
		if err := d.Models.Create(&m); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "创建失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": m})
	}
}
