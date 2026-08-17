package service

import (
	"golang.org/x/crypto/bcrypt"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// Seed 初始化演示数据（幂等：已存在则跳过）：
// 1. 默认管理员 admin/admin123
// 2. DeepSeek 提供商（API Key 留空，由后台补充）
// 3. 模型目录（deepseek-chat / deepseek-reasoner）
// 4. 示例 API Key demo（明文打印一次）
// 返回示例 Key 明文，用于启动日志展示。
func Seed(repo *repository.Repository) (seedKey string, err error) {
	// 1. 管理员
	var userCount int64
	if err := repo.DB.Model(&model.User{}).Count(&userCount).Error; err != nil {
		return "", err
	}
	if userCount == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if err != nil {
			return "", err
		}
		if err := repo.DB.Create(&model.User{
			Username: "admin", PasswordHash: string(hash), Role: "admin", Status: 1,
		}).Error; err != nil {
			return "", err
		}
	}

	// 2. 提供商 deepseek
	var p model.Provider
	if err := repo.DB.Where("name = ?", "deepseek").First(&p).Error; err != nil {
		p = model.Provider{
			Name: "deepseek", BaseURL: "https://api.deepseek.com",
			Enabled: 1, Priority: 0,
		}
		if err := repo.DB.Create(&p).Error; err != nil {
			return "", err
		}
	}

	// 3. 模型目录（价格为占位参考，可在后台调整）
	seedModels := []model.Model{
		{ProviderID: p.ID, Name: "deepseek-chat", DisplayName: "DeepSeek Chat", Tier: "normal", ContextWindow: 65536, PriceIn: 0.001, PriceOut: 0.002, Enabled: 1},
		{ProviderID: p.ID, Name: "deepseek-reasoner", DisplayName: "DeepSeek Reasoner", Tier: "strong", ContextWindow: 65536, PriceIn: 0.002, PriceOut: 0.008, Enabled: 1},
	}
	for i := range seedModels {
		var cnt int64
		if err := repo.DB.Model(&model.Model{}).Where("name = ?", seedModels[i].Name).Count(&cnt).Error; err != nil {
			return "", err
		}
		if cnt == 0 {
			if err := repo.DB.Create(&seedModels[i]).Error; err != nil {
				return "", err
			}
		}
	}

	// 4. 示例 Key
	var keyCount int64
	if err := repo.DB.Model(&model.ApiKey{}).Where("name = ?", "demo").Count(&keyCount).Error; err != nil {
		return "", err
	}
	if keyCount == 0 {
		ks := NewKeyService(repo)
		_, token, err := ks.Create(1, "demo", 10, 20, 0, "", 0, nil)
		if err != nil {
			return "", err
		}
		seedKey = token
	}
	return seedKey, nil
}
