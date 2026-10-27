package userpref

import (
	"encoding/json"

	iface "backend/interface"
	"backend/model"

	"gorm.io/gorm"
)

// DB 本包专用数据库句柄, 由 main.go 接线赋值 (userpref.DB = db)。
var DB *gorm.DB

// 编译期断言: 确认本包服务满足 learning 集群所需的窄接口 (原 service/interface_asserts.go)。
var _ iface.PreferenceGetter = (*UserPreferenceService)(nil)

type UserPreferenceService struct{}

func NewUserPreferenceService() *UserPreferenceService {
	return &UserPreferenceService{}
}

// GetPreference 获取用户指定 key 的偏好设置
func (s *UserPreferenceService) GetPreference(userID uint, key string) (*model.UserPreference, error) {
	var pref model.UserPreference
	err := DB.Where("user_id = ? AND pref_key = ?", userID, key).First(&pref).Error
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

// SavePreference 保存用户偏好设置（存在则更新，不存在则创建）
func (s *UserPreferenceService) SavePreference(userID uint, key string, value interface{}) error {
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}

	var pref model.UserPreference
	err = DB.Where("user_id = ? AND pref_key = ?", userID, key).First(&pref).Error
	if err != nil {
		// 不存在，创建新记录
		pref = model.UserPreference{
			UserID:    userID,
			PrefKey:   key,
			PrefValue: model.JSONValue(valueBytes),
		}
		return DB.Create(&pref).Error
	}

	// 存在，更新
	pref.PrefValue = model.JSONValue(valueBytes)
	return DB.Save(&pref).Error
}
