package i18n

import (
	"fmt"
	"strings"
)

// MessageByCode 按业务码返回本地化消息，适合作为默认响应文案。
func MessageByCode(code int, locale string) string {
	key, ok := codeToMessageKey[code]
	if !ok {
		return MessageByKey(MsgKeyFail, locale)
	}
	return MessageByKey(key, locale)
}

// MessageByKey 按消息 key 返回本地化文案，支持模板参数。
func MessageByKey(key, locale string, args ...any) string {
	normalizedLocale := NormalizeLocale(locale)
	tpl := ""
	if m, ok := messageCatalog[normalizedLocale]; ok {
		tpl = m[key]
	}
	if tpl == "" {
		tpl = messageCatalog[LocaleZHCN][key]
	}
	if tpl == "" {
		return key
	}
	if len(args) == 0 {
		return tpl
	}
	return fmt.Sprintf(tpl, args...)
}

// MessageTemplateHasArgs 判断消息模板是否包含格式化占位符。
func MessageTemplateHasArgs(key string) bool {
	tpl := messageCatalog[LocaleZHCN][key]
	if tpl == "" {
		for _, catalog := range messageCatalog {
			if v := catalog[key]; v != "" {
				tpl = v
				break
			}
		}
	}
	return strings.Contains(tpl, "%")
}
