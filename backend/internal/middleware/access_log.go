package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog 结构化访问日志（JSON，携带 request_id）。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		result := c.GetString("result")
		if result == "" {
			switch {
			case c.Writer.Status() == 401:
				result = "authentication_error"
			case c.Writer.Status() == 429:
				result = "rate_limited"
			case c.Writer.Status() >= 500:
				result = "server_error"
			case c.Writer.Status() >= 400:
				result = "client_error"
			default:
				result = "success"
			}
		}
		slog.Info("http_request",
			"request_id", c.GetString(RequestIDKey),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"result", result,
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}
