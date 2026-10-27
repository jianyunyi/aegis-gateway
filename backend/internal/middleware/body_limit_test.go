package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBodyLimitRejectsOversizedUnknownLengthBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(8))
	r.POST("/", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("POST", "/", strings.NewReader("123456789"))
	req.ContentLength = -1
	req.TransferEncoding = []string{"chunked"}
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != 413 {
		t.Fatalf("status=%d, want 413", resp.Code)
	}
}

func TestBodyLimitPassesBoundedBodyToHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(8))
	r.POST("/", func(c *gin.Context) {
		var body [8]byte
		if _, err := c.Request.Body.Read(body[:]); err != nil {
			c.Status(400)
			return
		}
		c.Status(200)
	})
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest("POST", "/", strings.NewReader("12345678")))
	if resp.Code != 200 {
		t.Fatalf("status=%d, want 200", resp.Code)
	}
}
