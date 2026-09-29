package service

import (
	"errors"
	"fmt"
	"time"

	"backend/config"
	"backend/model"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	cfg *config.Config
}

func NewAuthService(cfg *config.Config) *AuthService {
	return &AuthService{cfg: cfg}
}

func (s *AuthService) Register(username, password string) (*model.LoginResponseData, error) {
	var existing model.User
	if err := DB.Where("username = ?", username).First(&existing).Error; err == nil {
		return nil, errors.New("用户名已存在")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("密码加密失败")
	}

	user := model.User{
		Username:     username,
		PasswordHash: string(hashedPassword),
		Nickname:     username,
		Role:         "R_USER",
	}

	if err := DB.Create(&user).Error; err != nil {
		return nil, errors.New("注册失败: " + err.Error())
	}

	token, err := s.generateToken(user, 2*time.Hour)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &model.LoginResponseData{
		Token:        token,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) Login(username, password string) (interface{}, error) {
	var user model.User
	if err := DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	// Check if 2FA is required for R_SUPER users
	if user.Role == "R_SUPER" {
		twoFAEnabled, _ := NewSystemConfigService().GetValue("admin_2fa_enabled")
		if twoFAEnabled == "true" {
			tempToken, err := s.generate2FATempToken(user)
			if err != nil {
				return nil, errors.New("生成临时令牌失败")
			}
			needSetup := user.TotpSecret == nil
			return &model.TwoFactorLoginResponse{
				Need2FA:   true,
				TempToken: tempToken,
				NeedSetup: needSetup,
			}, nil
		}
	}

	// Normal login (no 2FA)
	token, err := s.generateToken(user, 2*time.Hour)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	// Update last login time
	now := time.Now()
	DB.Model(&user).Update("last_login_at", now)

	return &model.LoginResponseData{
		Token:        token,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) generateToken(user model.User, duration time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"userId":   user.ID,
		"userName": user.Username,
		"role":     user.Role,
		"typ":      "access",
		"exp":      time.Now().Add(duration).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.Auth.JWTSecret))
}

// generateRefreshToken 生成专用刷新令牌 (typ=refresh), 与访问令牌区分,
// 防止 7 天有效期的 refresh token 泄露后被直接当作 access token 调用业务接口
func (s *AuthService) generateRefreshToken(user model.User) (string, error) {
	claims := jwt.MapClaims{
		"userId":   user.ID,
		"userName": user.Username,
		"role":     user.Role,
		"typ":      "refresh",
		"exp":      time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.Auth.JWTSecret))
}

func (s *AuthService) GetUserInfo(userId uint) (*model.UserInfoResponseData, error) {
	var user model.User
	if err := DB.First(&user, userId).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	permissions, err := s.getPermissionsByRole(user.Role)
	if err != nil {
		return nil, err
	}

	return &model.UserInfoResponseData{
		UserId:      fmt.Sprintf("%d", user.ID),
		UserName:    user.Username,
		Nickname:    user.Nickname,
		Roles:       []string{user.Role},
		Buttons:     permissions,
		Permissions: permissions,
	}, nil
}

func (s *AuthService) RefreshToken(refreshTokenStr string) (*model.LoginResponseData, error) {
	// 验证 refreshToken 是否有效
	token, err := jwt.Parse(refreshTokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.cfg.Auth.JWTSecret), nil
	})

	if err != nil || !token.Valid {
		return nil, errors.New("刷新令牌无效或已过期")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("无效的令牌声明")
	}

	// 仅接受专用刷新令牌, 拒绝访问令牌混用
	if typ, _ := claims["typ"].(string); typ != "refresh" {
		return nil, errors.New("令牌类型错误")
	}

	userId, ok := claims["userId"].(float64)
	if !ok {
		return nil, errors.New("无法获取用户ID")
	}

	// 查询用户信息
	var user model.User
	if err := DB.First(&user, uint(userId)).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	// 生成新的 token 和 refreshToken
	newToken, err := s.generateToken(user, 2*time.Hour)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := s.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &model.LoginResponseData{
		Token:        newToken,
		RefreshToken: newRefreshToken,
	}, nil
}

func (s *AuthService) GetUserProfile(userId uint) (*model.UserProfileResponse, error) {
	var user model.User
	if err := DB.First(&user, userId).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	return &model.UserProfileResponse{
		UserId:      user.ID,
		UserName:    user.Username,
		Nickname:    user.Nickname,
		Email:       user.Email,
		Role:        user.Role,
		LastLoginAt: user.LastLoginAt,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}, nil
}

func (s *AuthService) UpdateProfile(userId uint, nickname, email string) error {
	result := DB.Model(&model.User{}).Where("id = ?", userId).Updates(map[string]interface{}{
		"nickname": nickname,
		"email":    email,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("用户不存在")
	}
	return nil
}

func (s *AuthService) ChangePassword(userId uint, oldPassword, newPassword string) error {
	var user model.User
	if err := DB.First(&user, userId).Error; err != nil {
		return errors.New("用户不存在")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return errors.New("原密码错误")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("密码加密失败")
	}

	result := DB.Model(&user).Update("password_hash", string(hashedPassword))
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// generate2FATempToken generates a short-lived JWT for 2FA verification
func (s *AuthService) generate2FATempToken(user model.User) (string, error) {
	claims := jwt.MapClaims{
		"userId":  user.ID,
		"purpose": "2fa",
		"exp":     time.Now().Add(10 * time.Minute).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.Auth.JWTSecret))
}

// Validate2FATempToken validates a 2FA temp token and returns the userId
func (s *AuthService) Validate2FATempToken(tempTokenStr string) (uint, error) {
	token, err := jwt.Parse(tempTokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.cfg.Auth.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return 0, errors.New("临时令牌无效或已过期")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("无效的令牌声明")
	}
	purpose, _ := claims["purpose"].(string)
	if purpose != "2fa" {
		return 0, errors.New("无效的令牌用途")
	}
	userId, ok := claims["userId"].(float64)
	if !ok {
		return 0, errors.New("无法获取用户ID")
	}
	return uint(userId), nil
}

// SetupTOTP generates a TOTP secret and returns QR code URL
func (s *AuthService) SetupTOTP(userId uint) (*model.TwoFactorSetupResponse, error) {
	var user model.User
	if err := DB.First(&user, userId).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "SaberOne",
		AccountName: user.Username,
		SecretSize:  20,
	})
	if err != nil {
		return nil, errors.New("生成TOTP密钥失败")
	}

	secret := key.Secret()
	result := DB.Model(&user).Update("totp_secret", secret)
	if result.Error != nil {
		return nil, errors.New("保存TOTP密钥失败")
	}

	return &model.TwoFactorSetupResponse{
		QRCodeURL: key.URL(),
		Secret:    secret,
	}, nil
}

// VerifyTOTP validates a TOTP code and returns real login tokens
func (s *AuthService) VerifyTOTP(userId uint, code string) (*model.LoginResponseData, error) {
	var user model.User
	if err := DB.First(&user, userId).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	if user.TotpSecret == nil || *user.TotpSecret == "" {
		return nil, errors.New("TOTP未配置")
	}

	if !totp.Validate(code, *user.TotpSecret) {
		return nil, errors.New("验证码错误")
	}

	token, err := s.generateToken(user, 2*time.Hour)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	// Update last login time
	now := time.Now()
	DB.Model(&user).Update("last_login_at", now)

	return &model.LoginResponseData{
		Token:        token,
		RefreshToken: refreshToken,
	}, nil
}

// ProxyLogin allows R_SUPER to generate tokens for another user
func (s *AuthService) ProxyLogin(targetUserId uint) (*model.LoginResponseData, error) {
	var user model.User
	if err := DB.First(&user, targetUserId).Error; err != nil {
		return nil, errors.New("目标用户不存在")
	}

	token, err := s.generateToken(user, 2*time.Hour)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &model.LoginResponseData{
		Token:        token,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) getPermissionsByRole(role string) ([]string, error) {
	if role == "R_SUPER" {
		var permissions []model.Permission
		if err := DB.Find(&permissions).Error; err != nil {
			return nil, err
		}
		codes := make([]string, 0, len(permissions))
		for _, permission := range permissions {
			codes = append(codes, permission.Code)
		}
		return codes, nil
	}

	var rows []model.RolePermission
	if err := DB.Where("role_code = ?", role).Find(&rows).Error; err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		codes = append(codes, row.PermissionCode)
	}
	return codes, nil
}
