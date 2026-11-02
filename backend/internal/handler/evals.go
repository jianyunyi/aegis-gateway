package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"aegis-gateway/internal/service"
)

// CreateEvalDataset 创建评测数据集。
func CreateEvalDataset(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "name 必填", "data": nil})
			return
		}
		ds, err := d.Eval.CreateDataset(req.Name, req.Description)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "创建失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": ds})
	}
}

// ListEvalDatasets 列出评测数据集。
func ListEvalDatasets(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ds, err := d.Eval.ListDatasets()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": ds})
	}
}

// ListEvalSamples 列出数据集样本。
func ListEvalSamples(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "id 无效", "data": nil})
			return
		}
		samples, err := d.Eval.ListSamples(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": samples})
	}
}

// AddEvalSample 手动添加评测样本（source=manual）。
func AddEvalSample(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			DatasetID                      uint64 `json:"dataset_id"`
			Prompt                         string `json:"prompt"`
			Reference                      string `json:"reference"`
			ConfirmSyntheticOrDeidentified bool   `json:"confirm_synthetic_or_deidentified"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.DatasetID == 0 || req.Prompt == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "dataset_id 与 prompt 必填", "data": nil})
			return
		}
		if !req.ConfirmSyntheticOrDeidentified {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "仅允许合成或彻底去标识数据；请先确认", "data": nil})
			return
		}
		if err := d.Eval.AddManualSample(req.DatasetID, req.Prompt, req.Reference, req.ConfirmSyntheticOrDeidentified); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "添加失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
	}
}

// DeleteEvalSample 删除单条样本，并清除已保存报告中对应的文本。
func DeleteEvalSample(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "id 无效", "data": nil})
			return
		}
		if err := d.Eval.DeleteSampleFromAdmin(id); err != nil {
			if errors.Is(err, service.ErrLegacySampleRequiresRehearsal) {
				c.JSON(http.StatusConflict, gin.H{"code": 40901, "message": "旧样本须先在隔离环境完成盘点和备份，再通过逐 ID 清理工具删除", "data": nil})
				return
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"code": 40401, "message": "评测样本不存在", "data": nil})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "删除失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
	}
}

// SampleEval 从真实调用日志采样入数据集。
func SampleEval(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.Cfg == nil || !d.Cfg.CapturePrompts {
			c.JSON(http.StatusForbidden, gin.H{"code": 40301, "message": "调用日志采样已关闭；需显式启用 PROMPT_CAPTURE_ENABLED", "data": nil})
			return
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "id 无效", "data": nil})
			return
		}
		var req struct {
			Count     int    `json:"count"`
			ModelName string `json:"model_name"`
		}
		_ = c.ShouldBindJSON(&req)
		added, err := d.Eval.SampleFromLogs(id, req.Count, req.ModelName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "采样失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"added": added}})
	}
}

// LabelEvalSample 人工打标。
func LabelEvalSample(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "id 无效", "data": nil})
			return
		}
		var req struct {
			Label int8 `json:"label"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || (req.Label != 0 && req.Label != 1) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "label 必须为 0 或 1", "data": nil})
			return
		}
		if err := d.Eval.LabelSample(id, req.Label); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "打标失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
	}
}

// RunEval 触发 A/B 回归评测。
func RunEval(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			DatasetID uint64 `json:"dataset_id"`
			ModelA    string `json:"model_a"`
			ModelB    string `json:"model_b"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.DatasetID == 0 || req.ModelA == "" || req.ModelB == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "dataset_id/model_a/model_b 必填", "data": nil})
			return
		}
		run, err := d.Eval.RunEvaluation(c, req.DatasetID, req.ModelA, req.ModelB)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": err.Error(), "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": run})
	}
}

// ListEvalRuns 列出评测运行。
func ListEvalRuns(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		runs, err := d.Eval.ListRuns()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "查询失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": runs})
	}
}

// GetEvalReport 评测报告（含逐样本明细）。
func GetEvalReport(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "id 无效", "data": nil})
			return
		}
		run, err := d.Eval.GetRun(id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 40401, "message": "评测运行不存在", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": run})
	}
}
