package handler

import (
	"errors"

	"gorm.io/gorm"
)

// isDuplicate 判断数据库唯一约束冲突（MySQL 1062 → GORM ErrDuplicatedKey）。
// 用于把"重复创建"从 500 内部错误降级为 400 + 明确提示。
func isDuplicate(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}
