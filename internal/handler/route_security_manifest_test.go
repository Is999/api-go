package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gozero_api/internal/security"
)

// TestDefaultRouteSecurityManifestValid 确保前后端同步清单可通过完整性校验。
func TestDefaultRouteSecurityManifestValid(t *testing.T) {
	if err := ValidateDefaultRouteSecurityManifest(); err != nil {
		t.Fatalf("ValidateDefaultRouteSecurityManifest() error = %v", err)
	}
}

// TestDefaultRouteSecurityManifestMatchesRouteContracts 确保清单逐条覆盖真实路由契约。
func TestDefaultRouteSecurityManifestMatchesRouteContracts(t *testing.T) {
	manifest := DefaultRouteSecurityManifest()
	contractByRoute := make(map[string]RouteContract, len(DefaultRouteContracts()))
	for _, contract := range DefaultRouteContracts() {
		contractByRoute[routeKey(contract.Method, contract.Path)] = contract
	}
	if len(manifest) != len(contractByRoute) {
		t.Fatalf("manifest count = %d, contract count = %d", len(manifest), len(contractByRoute))
	}
	for _, item := range manifest {
		contract, ok := contractByRoute[routeKey(item.Method, item.Path)]
		if !ok {
			t.Fatalf("manifest route missing from contracts: %+v", item)
		}
		if item.Alias != contract.Meta.Alias || item.Access != contract.Meta.Access || item.DocumentPath != contract.DocumentPath {
			t.Fatalf("manifest route mismatch item=%+v contract=%+v", item, contract)
		}
	}
}

// TestDefaultRouteSecurityManifestMatchesPolicies 确保清单字段和后端安全策略一致。
func TestDefaultRouteSecurityManifestMatchesPolicies(t *testing.T) {
	for _, item := range DefaultRouteSecurityManifest() {
		policy := security.PolicyByRoute(string(item.Alias))
		if !reflect.DeepEqual(item.RequestSign, emptyToNil(policy.RequestSign)) ||
			!reflect.DeepEqual(item.RequestCipher, emptyToNil(policy.RequestCipher)) ||
			!reflect.DeepEqual(item.ResponseSign, emptyToNil(policy.ResponseSign)) ||
			!reflect.DeepEqual(item.ResponseCipher, emptyToNil(policy.ResponseCipher)) {
			t.Fatalf("manifest policy mismatch alias=%s item=%+v policy=%+v", item.Alias, item, policy)
		}
	}
}

// TestDefaultRouteSecurityManifestReturnsCopies 确保调用方不能通过清单修改全局策略。
func TestDefaultRouteSecurityManifestReturnsCopies(t *testing.T) {
	manifest := DefaultRouteSecurityManifest()
	for _, item := range manifest {
		if len(item.RequestSign) == 0 {
			continue
		}
		original := security.PolicyByRoute(string(item.Alias)).RequestSign[0]
		item.RequestSign[0] = "changed"
		if got := security.PolicyByRoute(string(item.Alias)).RequestSign[0]; got != original {
			t.Fatalf("global security policy changed alias=%s got=%s want=%s", item.Alias, got, original)
		}
		return
	}
	t.Fatal("manifest should contain at least one request sign field")
}

// TestDefaultRouteSecurityManifestMatchesFrontendSnapshot 确保前端同步快照未和后端安全清单漂移。
func TestDefaultRouteSecurityManifestMatchesFrontendSnapshot(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "site", "route_security_manifest.json"))
	if err != nil {
		t.Fatalf("read route security manifest snapshot: %v", err)
	}
	want := routeSecurityManifestSnapshotJSON(t)
	if strings.TrimSpace(string(body)) != strings.TrimSpace(want) {
		t.Fatalf("route security manifest snapshot drifted, update docs/site/route_security_manifest.json")
	}
}

func emptyToNil(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	return fields
}

type routeSecurityManifestSnapshot struct {
	Version int                                 `json:"version"` // 快照版本
	Routes  []routeSecurityManifestSnapshotItem `json:"routes"`  // 前端同步路由清单
}

type routeSecurityManifestSnapshotItem struct {
	Alias          string             `json:"alias"`          // 路由别名
	Method         string             `json:"method"`         // HTTP 方法
	Path           string             `json:"path"`           // HTTP 路径
	Access         RouteAccess        `json:"access"`         // 访问边界
	Chain          RouteSecurityChain `json:"chain"`          // 实际安全链路
	Describe       string             `json:"describe"`       // 中文业务说明
	RequestSign    []string           `json:"requestSign"`    // 请求签名字段
	RequestCipher  []string           `json:"requestCipher"`  // 请求解密字段
	ResponseSign   []string           `json:"responseSign"`   // 响应回签字段
	ResponseCipher []string           `json:"responseCipher"` // 响应加密字段
	DocumentPath   string             `json:"documentPath"`   // 接口文档路径
}

func routeSecurityManifestSnapshotJSON(t *testing.T) string {
	t.Helper()
	body, err := json.MarshalIndent(routeSecurityManifestSnapshot{
		Version: 1,
		Routes:  routeSecurityManifestSnapshotItems(DefaultRouteSecurityManifest()),
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal route security manifest snapshot: %v", err)
	}
	return string(body) + "\n"
}

func routeSecurityManifestSnapshotItems(items []RouteSecurityManifestItem) []routeSecurityManifestSnapshotItem {
	result := make([]routeSecurityManifestSnapshotItem, 0, len(items))
	for _, item := range items {
		result = append(result, routeSecurityManifestSnapshotItem{
			Alias:          string(item.Alias),
			Method:         item.Method,
			Path:           item.Path,
			Access:         item.Access,
			Chain:          item.Chain,
			Describe:       item.Describe,
			RequestSign:    emptyToSlice(item.RequestSign),
			RequestCipher:  emptyToSlice(item.RequestCipher),
			ResponseSign:   emptyToSlice(item.ResponseSign),
			ResponseCipher: emptyToSlice(item.ResponseCipher),
			DocumentPath:   item.DocumentPath,
		})
	}
	return result
}

func emptyToSlice(fields []string) []string {
	if len(fields) == 0 {
		return []string{}
	}
	return fields
}
