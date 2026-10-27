package service

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// Seed initializes the optional demonstration model catalog. The first admin
// is created only when explicit bootstrap credentials are supplied; no API key
// is created or logged by startup.
func Seed(repo *repository.Repository, username, password string) error {
	var userCount int64
	if err := repo.DB.Model(&model.User{}).Count(&userCount).Error; err != nil {
		return err
	}
	if userCount == 0 {
		if strings.TrimSpace(username) == "" || len([]byte(password)) < 12 {
			return errors.New("bootstrap administrator credentials are required (password must be at least 12 bytes)")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := repo.DB.Create(&model.User{
			Username: strings.TrimSpace(username), PasswordHash: string(hash), Role: "admin", Status: 1,
		}).Error; err != nil {
			return err
		}
	}

	var p model.Provider
	if err := repo.DB.Where("name = ?", "deepseek").First(&p).Error; err != nil {
		p = model.Provider{
			Name: "deepseek", BaseURL: "https://api.deepseek.com",
			Enabled: 1, Priority: 0,
		}
		if err := repo.DB.Create(&p).Error; err != nil {
			return err
		}
	}

	seedModels := []model.Model{
		{ProviderID: p.ID, Name: "deepseek-chat", DisplayName: "DeepSeek Chat", Tier: "normal", ContextWindow: 65536, PriceIn: 0.001, PriceOut: 0.002, Enabled: 1},
		{ProviderID: p.ID, Name: "deepseek-reasoner", DisplayName: "DeepSeek Reasoner", Tier: "strong", ContextWindow: 65536, PriceIn: 0.002, PriceOut: 0.008, Enabled: 1},
	}
	for i := range seedModels {
		var count int64
		if err := repo.DB.Model(&model.Model{}).Where("name = ?", seedModels[i].Name).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := repo.DB.Create(&seedModels[i]).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
