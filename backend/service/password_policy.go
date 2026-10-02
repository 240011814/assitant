package service

import "errors"

// ValidatePasswordStrength 密码强度校验: 8~72 位 (bcrypt 有效上限 72 字节), 须同时包含字母和数字
func ValidatePasswordStrength(password string) error {
	n := len(password)
	if n < 8 {
		return errors.New("密码强度不足: 至少 8 位")
	}
	if n > 72 {
		return errors.New("密码强度不足: 最多 72 位")
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("密码强度不足: 须同时包含字母和数字")
	}
	return nil
}
