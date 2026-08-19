package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/middleware"
)

// ListKeys 分页查询 API Key。
func ListKeys(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := atoiDefault(c.Query("page"), 1)
		pageSize := atoiDefault(c.Query("page_size"), 10)
		keys, total, err := d.Keys.List(page, pageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{
			"list": keys, "total": total, "page": page, "page_size": pageSize,
		}})
	}
}

// CreateKey 创建 Key，明文仅返回一次。
func CreateKey(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name          string  `json:"name"`
			RPSLimit      int     `json:"rps_limit"`
			Burst         int     `json:"burst"`
			QuotaTokens   int64   `json:"quota_tokens"`
			DefaultModel  string  `json:"default_model"`
			BudgetMonthly float64 `json:"budget_monthly"`
			ExpiresAt     string  `json:"expires_at"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "请求参数格式错误", "data": nil})
			return
		}
		if req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "name 必填", "data": nil})
			return
		}
		if req.RPSLimit <= 0 {
			req.RPSLimit = 10
		}
		if req.Burst <= 0 {
			req.Burst = 20
		}
		// 兼容宽松时间格式（RFC3339 / "YYYY-MM-DD HH:mm:ss" / 纯日期）
		var expiresAt *time.Time
		if req.ExpiresAt != "" {
			t, err := parseTimeFlexible(req.ExpiresAt)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "expires_at 格式无效，请使用 YYYY-MM-DD HH:mm:ss", "data": nil})
				return
			}
			expiresAt = &t
		}
		userID, _ := c.Get(middleware.CtxUserID)
		uid := toUint64(userID)

		key, token, err := d.Keys.Create(uid, req.Name, req.RPSLimit, req.Burst, req.QuotaTokens, req.DefaultModel, req.BudgetMonthly, expiresAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "创建失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{
			"id": key.ID, "name": key.Name, "key": token, "key_prefix": key.KeyPrefix,
			"expires_at": key.ExpiresAt,
		}})
	}
}

// parseTimeFlexible 兼容多种时间输入格式（RFC3339 / 空格分隔 / 纯日期）。
func parseTimeFlexible(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time format: %s", s)
}

// SetKeyStatus 启用/禁用 Key。
func SetKeyStatus(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "id 无效", "data": nil})
			return
		}
		var req struct {
			Status int8 `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || (req.Status != 0 && req.Status != 1) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "status 必须为 0 或 1", "data": nil})
			return
		}
		if err := d.Keys.SetStatus(id, req.Status); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "更新失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
	}
}

// ---- 通用解析 ----

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func toUint64(v any) uint64 {
	switch n := v.(type) {
	case float64:
		return uint64(n)
	case int64:
		return uint64(n)
	case int:
		return uint64(n)
	}
	return 0
}
