package middleware

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

const (
	IdempotencyKeyHeader = "Idempotency-Key"
	idempotencyWait      = 3 * time.Second
	idempotencyMaxBody   = 16 << 20
)

type idempotencyWriter struct {
	gin.ResponseWriter
	body      bytes.Buffer
	status    int
	wrote     bool
	overLimit bool
}

func (w *idempotencyWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *idempotencyWriter) Write(data []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.body.Len()+len(data) <= idempotencyMaxBody {
		_, _ = w.body.Write(data)
	} else {
		w.overLimit = true
	}
	return w.ResponseWriter.Write(data)
}

func (w *idempotencyWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

// Idempotency serializes requests carrying Idempotency-Key per API key and
// replays the first completed HTTP response. Requests without the header are unchanged.
func Idempotency(repo *repository.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(IdempotencyKeyHeader)
		if key == "" {
			c.Next()
			return
		}
		if !repository.ValidIdempotencyKey(key) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "invalid Idempotency-Key", "type": "aegis_error"}})
			return
		}
		apiKey, ok := c.Get(CtxAPIKey)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		keyModel, ok := apiKey.(*model.ApiKey)
		if !ok || keyModel == nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		body, err := readAndRestoreBody(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "无法读取请求体", "type": "aegis_error"}})
			return
		}
		fingerprint := repository.IdempotencyFingerprint(c.Request.Method, c.Request.URL.RequestURI(), c.GetHeader("Content-Type"), body)
		ctx, cancel := context.WithTimeout(c.Request.Context(), idempotencyWait)
		defer cancel()
		state, owner, err := repo.ClaimIdempotency(ctx, keyModel.ID, key, fingerprint)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "幂等服务暂不可用", "type": "aegis_error"}})
			return
		}
		if !owner {
			if state.Fingerprint != fingerprint {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Idempotency-Key 已用于不同请求", "type": "aegis_error"}})
				return
			}
			if state.State == "done" {
				replayIdempotency(c, state)
				return
			}
			if state.State != "processing" {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": gin.H{"message": "此 Idempotency-Key 已被使用，但响应无法重放", "type": "aegis_error"}})
				return
			}
			state, err = waitForIdempotency(ctx, repo, keyModel.ID, key, fingerprint)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": gin.H{"message": "相同请求仍在处理中，请稍后重试", "type": "aegis_error"}})
				return
			}
			if state.State == "done" {
				replayIdempotency(c, state)
				return
			}
			if state.State != "processing" {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": gin.H{"message": "此 Idempotency-Key 已被使用，但响应无法重放", "type": "aegis_error"}})
				return
			}
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": gin.H{"message": "相同请求仍在处理中，请稍后重试", "type": "aegis_error"}})
			return
		}

		writer := &idempotencyWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		heartbeatStop := make(chan struct{})
		heartbeatDone := make(chan struct{})
		go idempotencyHeartbeat(c, repo, keyModel.ID, key, fingerprint, heartbeatStop, heartbeatDone)
		c.Next()
		close(heartbeatStop)
		<-heartbeatDone
		if writer.overLimit {
			// Do not replace a potentially large response with an unbounded Redis value.
			// Preserve a terminal marker so a later retry cannot re-run the provider.
			markCtx, markCancel := context.WithTimeout(context.Background(), time.Second)
			if err := repo.MarkIdempotencyUnreplayable(markCtx, keyModel.ID, key, fingerprint); err != nil {
				slog.Error("mark idempotency response unreplayable failed", "api_key_id", keyModel.ID, "error", err)
			}
			markCancel()
			return
		}
		result := repository.IdempotencyResult{Fingerprint: fingerprint, Status: writer.status, ContentType: writer.Header().Get("Content-Type"), Body: writer.body.Bytes()}
		if result.Status == 0 {
			result.Status = http.StatusOK
		}
		if err := repo.CompleteIdempotency(context.Background(), keyModel.ID, key, result); err != nil {
			// The original response is already sent; retaining the marker prevents a retry
			// from issuing a second provider call while Redis is unhealthy.
			markCtx, markCancel := context.WithTimeout(context.Background(), time.Second)
			if markErr := repo.MarkIdempotencyUnreplayable(markCtx, keyModel.ID, key, fingerprint); markErr != nil {
				slog.Error("mark idempotency completion failure failed", "api_key_id", keyModel.ID, "error", markErr)
			}
			markCancel()
			return
		}
	}
}

func readAndRestoreBody(c *gin.Context) ([]byte, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func idempotencyHeartbeat(c *gin.Context, repo *repository.Repository, keyID uint64, key, fingerprint string, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, _ = repo.RenewIdempotency(ctx, keyID, key, fingerprint)
			cancel()
		}
	}
}

func waitForIdempotency(ctx context.Context, repo *repository.Repository, keyID uint64, key, fingerprint string) (*repository.IdempotencyResult, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			state, found, err := repo.GetIdempotency(ctx, keyID, key)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, context.Canceled
			}
			if state.Fingerprint != fingerprint {
				return nil, context.Canceled
			}
			if state.State != "processing" {
				return state, nil
			}
		}
	}
}

func replayIdempotency(c *gin.Context, result *repository.IdempotencyResult) {
	if result.ContentType != "" {
		c.Header("Content-Type", result.ContentType)
	}
	c.Data(result.Status, result.ContentType, result.Body)
	c.Abort()
}
