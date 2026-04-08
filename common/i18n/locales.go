package i18n

import (
	"strings"

	"golang.org/x/text/language"
)

// 支持的响应语言标签。
const (
	// LocaleZHCN 表示简体中文。
	LocaleZHCN = "zh-CN"
	// LocaleENUS 表示美式英文。
	LocaleENUS = "en-US"
)

// NormalizeLocale 归一化请求语言，未知语言默认中文。
func NormalizeLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return LocaleZHCN
	}
	tags, _, err := language.ParseAcceptLanguage(locale)
	if err != nil || len(tags) == 0 {
		tag, parseErr := language.Parse(locale)
		if parseErr != nil {
			return LocaleZHCN
		}
		tags = []language.Tag{tag}
	}
	for _, tag := range tags {
		if locale := supportedLocale(tag); locale != "" {
			return locale
		}
	}
	return LocaleZHCN
}

// supportedLocale 将标准语言标签映射到当前后端支持的语言。
func supportedLocale(tag language.Tag) string {
	if strings.EqualFold(tag.String(), LocaleENUS) {
		return LocaleENUS
	}
	base, _ := tag.Base()
	switch base.String() {
	case "en":
		return LocaleENUS
	case "zh":
		return LocaleZHCN
	default:
		return ""
	}
}
