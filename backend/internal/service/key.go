package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// KeyService API Key 管理。
type KeyService struct {
	repo *repository.Repository
}

// NewKeyService 构造 KeyService。
func NewKeyService(repo *repository.Repository) *KeyService {
	return &KeyService{repo: repo}
}

// Create 生成新 Key：明文仅此一次返回，库中只存 SHA-256 哈希与展示前缀（ADR-007）。
func (s *KeyService) Create(userID uint64, name string, rps, burst int, quota int64, defaultModel string, budgetMonthly float64, expiresAt *time.Time) (*model.ApiKey, string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	token := "ak_" + hex.EncodeToString(raw) // ak_ + 32 位十六进制
	sum := sha256.Sum256([]byte(token))

	key := &model.ApiKey{
		Name:          name,
		KeyHash:       hex.EncodeToString(sum[:]),
		KeyPrefix:     token[:len("ak_")+8],
		UserID:        userID,
		Status:        1,
		QuotaTokens:   quota,
		DefaultModel:  defaultModel,
		BudgetMonthly: budgetMonthly,
		RPSLimit:      rps,
		Burst:         burst,
		ExpiresAt:     expiresAt,
	}
	if err := s.repo.DB.Create(key).Error; err != nil {
		return nil, "", err
	}
	return key, token, nil
}

// List 分页查询 Key（不含哈希与明文）。
func (s *KeyService) List(page, pageSize int) ([]model.ApiKey, int64, error) {
	var keys []model.ApiKey
	var total int64
	if err := s.repo.DB.Model(&model.ApiKey{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := s.repo.DB.Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&keys).Error
	return keys, total, err
}

// SetStatus 启用/禁用 Key（禁用即时生效：KeyAuth 会拒绝 status!=1）。
func (s *KeyService) SetStatus(id uint64, status int8) error {
	return s.repo.DB.Model(&model.ApiKey{}).Where("id = ?", id).
		Update("status", status).Error
}
