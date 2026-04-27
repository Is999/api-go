package security

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"api/helper"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
)

// 路由安全策略字段标记常量。
const (
	// SignFieldAll 表示签名时所有首层字段参与排序签名。
	SignFieldAll = "*"
	// CipherWholeBody 表示已废弃的整包加密标记，仅用于识别并拒绝非法输入。
	CipherWholeBody = "cipher"
	// CipherJSONPrefix 表示字段值在加解密前需要按 JSON 编解码。
	CipherJSONPrefix = "json:"
)

// RouteSecurityPolicy 定义单个路由的请求验签、响应回签与响应加密策略。
type RouteSecurityPolicy struct {
	RequestSign    []string // RequestSign 表示请求验签关键字段；新接口禁止使用 *
	RequestCipher  []string // RequestCipher 表示请求允许解密的字段；禁止使用 cipher 整包加密
	ResponseSign   []string // ResponseSign 表示响应回签关键字段；禁止使用 *
	ResponseCipher []string // ResponseCipher 表示响应需要加密的字段路径；禁止使用 cipher 整包加密
}

// RouteSecurityPolicies 定义前台 API 的推荐安全策略。
var RouteSecurityPolicies = map[string]RouteSecurityPolicy{
	"auth.register": {
		RequestSign:    []string{"username", "password", "nickname", "email", "phone"},
		RequestCipher:  []string{"password", "email", "phone"},
		ResponseSign:   []string{"token", "expiresAt"},
		ResponseCipher: []string{"token", "user.email", "user.phone"},
	},
	"auth.login": {
		RequestSign:    []string{"username", "password"},
		RequestCipher:  []string{"password"},
		ResponseSign:   []string{"token", "expiresAt"},
		ResponseCipher: []string{"token", "user.email", "user.phone"},
	},
	"auth.refresh": {
		ResponseSign:   []string{"token", "expiresAt"},
		ResponseCipher: []string{"token"},
	},
	"auth.logout": {},
	"user.profile": {
		ResponseCipher: []string{"email", "phone"},
	},
	"system.config_reload.status": {},
	"system.config_reload.run":    {},
}

// PolicyByRoute 根据路由别名读取统一安全策略。
func PolicyByRoute(route string) RouteSecurityPolicy {
	route = strings.TrimSpace(route)
	if route == "" || strings.EqualFold(route, "ignore") {
		return RouteSecurityPolicy{}
	}
	if policy, ok := RouteSecurityPolicies[route]; ok {
		return policy
	}
	return RouteSecurityPolicy{}
}

// BuildSignString 生成待签名字符串，按字段排序后拼接时间绑定的请求盐值。
func BuildSignString(data map[string]any, signParams []string, traceID, timestamp, appID string) string {
	params := resolveSignParams(data, signParams)
	sort.Strings(params)

	var builder strings.Builder
	for _, key := range params {
		value, ok := data[key]
		if !ok || isEmptySignValue(value) {
			continue
		}
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(SignValueString(value))
		builder.WriteString("&")
	}
	builder.WriteString("key=")
	builder.WriteString(utils.Md5(appID + "-" + traceID + "-" + timestamp))
	return builder.String()
}

// EncodeCipherParams 把字段级加密配置编码成请求头值；整包加密标记不再生成请求头。
func EncodeCipherParams(params []string) string {
	params = helper.UniqueNonEmptyStrings(params)
	if len(params) == 0 {
		return ""
	}
	for _, param := range params {
		if strings.EqualFold(param, CipherWholeBody) {
			return ""
		}
	}
	body, err := json.Marshal(params)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(body)
}

// resolveSignParams 解析签名字段列表；配置了 * 时，对当前 map 的所有首层字段签名。
func resolveSignParams(data map[string]any, signParams []string) []string {
	params := helper.UniqueNonEmptyStrings(signParams)
	if !utils.IsHas(SignFieldAll, params) {
		return params
	}
	result := make([]string, 0, len(data))
	for key := range data {
		switch strings.TrimSpace(key) {
		case "", "sign", "ciphertext":
			continue
		default:
			result = append(result, key)
		}
	}
	return result
}

// SignValueString 把参与签名的值转换为稳定字符串。
func SignValueString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), ".")
	case float32:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), ".")
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, bool:
		return fmt.Sprint(v)
	default:
		body, err := stableJSONMarshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(body)
	}
}

// stableJSONMarshal 对复杂值执行递归稳定 JSON 序列化。
func stableJSONMarshal(value any) ([]byte, error) {
	normalized, err := normalizeStableJSONValue(value)
	if err != nil {
		return nil, errors.Tag(err)
	}
	var builder strings.Builder
	if err := writeStableJSON(&builder, normalized); err != nil {
		return nil, errors.Tag(err)
	}
	return []byte(builder.String()), nil
}

// normalizeStableJSONValue 先把任意复杂值收敛成基础结构。
func normalizeStableJSONValue(value any) (any, error) {
	switch v := value.(type) {
	case nil, string, bool, json.Number,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return v, nil
	case map[string]any:
		return v, nil
	case []any:
		return v, nil
	default:
		body, err := json.Marshal(v)
		if err != nil {
			return nil, errors.Tag(err)
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var normalized any
		if err := decoder.Decode(&normalized); err != nil {
			return nil, errors.Tag(err)
		}
		return normalized, nil
	}
}

// writeStableJSON 递归输出稳定 JSON，map key 统一按字典序排序。
func writeStableJSON(builder *strings.Builder, value any) error {
	switch v := value.(type) {
	case nil:
		builder.WriteString("null")
	case string:
		body, err := json.Marshal(v)
		if err != nil {
			return errors.Tag(err)
		}
		builder.Write(body)
	case bool:
		builder.WriteString(fmt.Sprint(v))
	case json.Number:
		builder.WriteString(v.String())
	case float64:
		builder.WriteString(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), "."))
	case float32:
		builder.WriteString(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), "."))
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		builder.WriteString(fmt.Sprint(v))
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		builder.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				builder.WriteByte(',')
			}
			keyBody, err := json.Marshal(key)
			if err != nil {
				return errors.Tag(err)
			}
			builder.Write(keyBody)
			builder.WriteByte(':')
			if err := writeStableJSON(builder, v[key]); err != nil {
				return errors.Tag(err)
			}
		}
		builder.WriteByte('}')
	case []any:
		builder.WriteByte('[')
		for index, item := range v {
			if index > 0 {
				builder.WriteByte(',')
			}
			if err := writeStableJSON(builder, item); err != nil {
				return errors.Tag(err)
			}
		}
		builder.WriteByte(']')
	default:
		normalized, err := normalizeStableJSONValue(v)
		if err != nil {
			return errors.Tag(err)
		}
		return writeStableJSON(builder, normalized)
	}
	return nil
}

// isEmptySignValue 判断字段是否应跳过签名。
func isEmptySignValue(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return text == ""
	}
	return false
}
