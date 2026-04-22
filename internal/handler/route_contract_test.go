package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"api/internal/config"
	"api/internal/handler/shared"
	"api/internal/security"
	"api/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// TestDefaultRouteContractsMatchRegisteredRoutes 确保契约表与真实注册路由一致。
func TestDefaultRouteContractsMatchRegisteredRoutes(t *testing.T) {
	server := rest.MustNewServer(rest.RestConf{Host: "127.0.0.1", Port: 0})
	defer server.Stop()

	RegisterHandlers(server, svc.NewServiceContext(config.Config{}, "test-version", svc.Dependencies{}))

	registered := routeSet(server.Routes())
	contracts := DefaultRouteContracts()
	if len(registered) != len(contracts) {
		t.Fatalf("registered route count = %d, contract count = %d", len(registered), len(contracts))
	}
	for _, contract := range contracts {
		key := routeKey(contract.Method, contract.Path)
		if _, ok := registered[key]; !ok {
			t.Fatalf("contract route is not registered: %s", key)
		}
	}
}

// TestDefaultRouteContractsValid 确保契约字段完整且访问边界与路径前缀匹配。
func TestDefaultRouteContractsValid(t *testing.T) {
	seenRoutes := make(map[string]struct{}, len(DefaultRouteContracts()))
	knownMetas := routeMetaAccessByAlias()
	for _, contract := range DefaultRouteContracts() {
		if contract.Method == "" || contract.Path == "" || contract.DocumentPath == "" {
			t.Fatalf("route contract has empty field: %+v", contract)
		}
		key := routeKey(contract.Method, contract.Path)
		if _, ok := seenRoutes[key]; ok {
			t.Fatalf("duplicate route contract: %s", key)
		}
		seenRoutes[key] = struct{}{}

		access, ok := knownMetas[string(contract.Meta.Alias)]
		if !ok {
			t.Fatalf("route contract meta alias missing from DefaultRouteMetas: %+v", contract)
		}
		if access != contract.Meta.Access {
			t.Fatalf("route contract access mismatch: %+v", contract)
		}
		if contract.Meta.Access == shared.RouteAccessInternal && !strings.HasPrefix(contract.Path, "/internal/") {
			t.Fatalf("internal route must use /internal/ prefix: %+v", contract)
		}
		if contract.Meta.Access != shared.RouteAccessInternal && strings.HasPrefix(contract.Path, "/internal/") {
			t.Fatalf("non-internal route must not use /internal/ prefix: %+v", contract)
		}
	}
}

// TestRouteContractDocumentsContainPath 确保接口文档包含契约表中的真实路径。
func TestRouteContractDocumentsContainPath(t *testing.T) {
	for _, contract := range DefaultRouteContracts() {
		documentPath := filepath.Join("..", "..", contract.DocumentPath)
		body, err := os.ReadFile(documentPath)
		if err != nil {
			t.Fatalf("read route document %s: %v", contract.DocumentPath, err)
		}
		if !strings.Contains(string(body), contract.Path) {
			t.Fatalf("document %s does not contain route path %s", contract.DocumentPath, contract.Path)
		}
	}
}

// TestRouteSecurityPoliciesMatchDocuments 确保接口文档安全字段和路由安全策略一致。
func TestRouteSecurityPoliciesMatchDocuments(t *testing.T) {
	for _, contract := range DefaultRouteContracts() {
		documentPath := filepath.Join("..", "..", contract.DocumentPath)
		body, err := os.ReadFile(documentPath)
		if err != nil {
			t.Fatalf("read route document %s: %v", contract.DocumentPath, err)
		}
		section, ok := routeDocumentSection(string(body), routeKey(contract.Method, contract.Path))
		if !ok {
			t.Fatalf("document %s missing route section %s", contract.DocumentPath, routeKey(contract.Method, contract.Path))
		}
		rows := routeSecurityDocumentRows(security.PolicyByRoute(string(contract.Meta.Alias)))
		for label, value := range rows {
			row := "| " + label + " | " + value + " |"
			if !strings.Contains(section, row) {
				t.Fatalf("document %s route %s missing security row %q", contract.DocumentPath, routeKey(contract.Method, contract.Path), row)
			}
		}
	}
}

// TestRouteSecurityPoliciesUseContractAliases 确保安全策略只绑定契约表内路由。
func TestRouteSecurityPoliciesUseContractAliases(t *testing.T) {
	aliases := make(map[string]struct{}, len(DefaultRouteContracts()))
	for _, contract := range DefaultRouteContracts() {
		aliases[string(contract.Meta.Alias)] = struct{}{}
	}
	for alias := range security.RouteSecurityPolicies {
		if _, ok := aliases[alias]; !ok {
			t.Fatalf("security policy route alias missing from route contracts: %s", alias)
		}
	}
}

func routeSet(routes []rest.Route) map[string]struct{} {
	result := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		key := routeKey(route.Method, route.Path)
		if _, ok := result[key]; ok {
			continue
		}
		result[key] = struct{}{}
	}
	return result
}

func routeDocumentSection(document string, key string) (string, bool) {
	lines := strings.Split(document, "\n")
	start := -1
	marker := "`" + key + "`"
	for index, line := range lines {
		if strings.TrimSpace(line) == marker {
			start = index
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "### ") {
			end = index
			break
		}
	}
	return strings.Join(lines[start:end], "\n"), true
}

func routeSecurityDocumentRows(policy security.RouteSecurityPolicy) map[string]string {
	return map[string]string{
		"请求签名字段": securityDocumentFieldValue(policy.RequestSign, "不参与签名"),
		"请求加密字段": securityDocumentFieldValue(policy.RequestCipher, "不参与加密"),
		"响应签名字段": securityDocumentFieldValue(policy.ResponseSign, "不参与签名"),
		"响应加密字段": securityDocumentFieldValue(policy.ResponseCipher, "不参与加密"),
	}
}

func securityDocumentFieldValue(fields []string, empty string) string {
	if len(fields) == 0 {
		return empty
	}
	return strings.Join(fields, ", ")
}

func routeKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}
