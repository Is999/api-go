package helper

import (
	"strings"

	utils "github.com/Is999/go-utils"
)

// UniqueNonEmptyStrings 清洗字符串列表并按首次出现顺序去重。
func UniqueNonEmptyStrings(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		result = append(result, text)
	}
	return utils.Unique(result)
}
