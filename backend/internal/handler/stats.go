package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/service"
)

// StatsOverview 今日大盘概览。
func StatsOverview(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ov, err := d.Stats.Overview(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询概览失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": ov})
	}
}

// StatsTrends 趋势：?range=7d|30d。
func StatsTrends(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := 7
		switch c.Query("range") {
		case "30d":
			days = 30
		case "7d", "":
			days = 7
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "range 仅支持 7d/30d", "data": nil})
			return
		}
		rows, err := d.Stats.Trends(c, days)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询趋势失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": rows})
	}
}

// ListLogs 调用日志查询：?page=&page_size=&model_name=&api_key_id=&status=。
func ListLogs(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyID, _ := strconv.ParseUint(c.Query("api_key_id"), 10, 64)
		f := service.LogFilter{
			Page:      atoiDefault(c.Query("page"), 1),
			PageSize:  atoiDefault(c.Query("page_size"), 20),
			ModelName: c.Query("model_name"),
			APIKeyID:  keyID,
			Status:    c.Query("status"),
		}
		logs, total, err := d.Stats.Logs(c, f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询日志失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{
			"list": logs, "total": total, "page": f.Page, "page_size": f.PageSize,
		}})
	}
}

// BillingDaily 每日账单：?days=30（默认 30 天，实时聚合自 usage_logs）。
func BillingDaily(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := atoiDefault(c.Query("days"), 30)
		rows, err := d.Billing.Daily(c, days)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询账单失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": rows})
	}
}

// BillingReconcile 手动触发一次对账（演示/运维用）。
func BillingReconcile(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := d.Billing.Reconcile(c, 7)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "对账失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": res})
	}
}
