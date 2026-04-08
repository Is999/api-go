package i18n

// localeMessageCatalog 表示单个语言包。
type localeMessageCatalog map[string]string

// messageCatalog 按语言标签收口后端语言包。
var messageCatalog = map[string]localeMessageCatalog{
	LocaleZHCN: zhCNMessageCatalog,
	LocaleENUS: enUSMessageCatalog,
}
