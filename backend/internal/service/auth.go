// Package service 业务逻辑层：handler 只做编排，业务规则在 service。
package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// AuthService 管理后台登录与 JWT 签发。
type AuthService struct {
	repo   *repository.Repository
	secret string
	expire time.Duration
}

// NewAuthService 构造 AuthService。
func NewAuthService(repo *repository.Repository, secret string, expire time.Duration) *AuthService {
	return &AuthService{repo: repo, secret: secret, expire: expire}
}

// Login 校验用户名密码并签发 JWT。失败统一返回"用户名或密码错误"（不泄露账号存在性）。
func (s *AuthService) Login(username, password string) (string, error) {
	var u model.User
	if err := s.repo.DB.Where("username = ? AND status = 1", username).First(&u).Error; err != nil {
		return "", errors.New("用户名或密码错误")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", errors.New("用户名或密码错误")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"uid":      u.ID,
		"username": u.Username,
		"iat":      now.Unix(),
		"exp":      now.Add(s.expire).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(s.secret))
	if err != nil {
		return "", err
	}
	return ss, nil
}
