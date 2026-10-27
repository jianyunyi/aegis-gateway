package middleware

import (
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDIgnoresClientValueAndGeneratesUniqueIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.POST("/", func(c *gin.Context) { c.String(200, c.GetString(RequestIDKey)) })

	ids := map[string]bool{}
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/", nil)
		req.Header.Set("X-Request-ID", "same-client-controlled-id")
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)
		id := resp.Header().Get("X-Request-ID")
		if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
			t.Fatalf("unexpected internal request ID %q", id)
		}
		if id == "same-client-controlled-id" || ids[id] {
			t.Fatalf("client ID was accepted or generated ID repeated: %q", id)
		}
		ids[id] = true
	}
}
