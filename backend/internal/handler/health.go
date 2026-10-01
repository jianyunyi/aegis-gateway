package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health 健康检查端点，M1 演示点。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "aegis-gateway",
		"version": "v0.1.0-m1",
	})
}

// Ready is separate from liveness and checks dependencies with a bounded deadline.
func Ready(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.Repo.Draining.Load() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "draining"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), d.Cfg.ReadinessTimeout)
		defer cancel()
		db, err := d.Repo.DB.DB()
		if err == nil {
			err = db.PingContext(ctx)
		}
		if err == nil {
			err = d.Repo.Redis.Ping(ctx).Err()
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
