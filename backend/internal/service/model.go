package service

import (
	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// ModelService 模型目录与计价管理（Tier 用于语义路由分级）。
type ModelService struct {
	repo *repository.Repository
}

// NewModelService 构造 ModelService。
func NewModelService(repo *repository.Repository) *ModelService {
	return &ModelService{repo: repo}
}

// Create 新建模型目录项。
func (s *ModelService) Create(m *model.Model) error {
	return s.repo.DB.Create(m).Error
}

// List 查询模型目录（可选仅启用）。
func (s *ModelService) List(enabledOnly bool) ([]model.Model, error) {
	var ms []model.Model
	q := s.repo.DB.Order("tier ASC, id ASC")
	if enabledOnly {
		q = q.Where("enabled = 1")
	}
	err := q.Find(&ms).Error
	return ms, err
}

// GetByName 按上游模型名查询（代理链路使用）。
func (s *ModelService) GetByName(name string) (*model.Model, error) {
	var m model.Model
	if err := s.repo.DB.Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}
