package service

import (
	"errors"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
	"aegis-gateway/internal/util"
)

// ProviderService 上游提供商管理（API Key AES 加密落库）。
type ProviderService struct {
	repo   *repository.Repository
	secret string
}

// NewProviderService 构造 ProviderService。
func NewProviderService(repo *repository.Repository, secret string) *ProviderService {
	return &ProviderService{repo: repo, secret: secret}
}

// Create 新建提供商；apiKey 加密后存储，绝不落明文。
func (s *ProviderService) Create(name, baseURL, apiKey string, enabled int8, priority int) (*model.Provider, error) {
	enc, err := util.EncryptSecret(s.secret, apiKey)
	if err != nil {
		return nil, err
	}
	p := &model.Provider{
		Name:      name,
		BaseURL:   baseURL,
		APIKeyEnc: enc,
		Enabled:   enabled,
		Priority:  priority,
	}
	if err := s.repo.DB.Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

// List 查询全部提供商。
func (s *ProviderService) List() ([]model.Provider, error) {
	var ps []model.Provider
	err := s.repo.DB.Order("priority ASC, id ASC").Find(&ps).Error
	return ps, err
}

// Get 按 ID 查询提供商。
func (s *ProviderService) Get(id uint64) (*model.Provider, error) {
	var p model.Provider
	if err := s.repo.DB.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// DecryptKey 解密提供商 API Key（仅代理调用时使用，不对外返回）。
func (s *ProviderService) DecryptKey(p *model.Provider) (string, error) {
	if p.APIKeyEnc == "" {
		return "", nil
	}
	return util.DecryptSecret(s.secret, p.APIKeyEnc)
}

// ErrProviderHasModels 提供商下仍有模型时拒绝删除（防误删）。
var ErrProviderHasModels = errors.New("provider has models")

// Delete 删除提供商：若其下存在模型则拒绝（安全策略：先删模型再删提供商）。
func (s *ProviderService) Delete(id uint64) error {
	var count int64
	if err := s.repo.DB.Model(&model.Model{}).Where("provider_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrProviderHasModels
	}
	return s.repo.DB.Delete(&model.Provider{}, id).Error
}
