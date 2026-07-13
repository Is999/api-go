package svc

import (
	"context"
	"net/http/httptest"
	"testing"

	"api/internal/config"
)

// TestScopedWithContextCopiesConfigSnapshot 验证对应场景符合预期。
func TestScopedWithContextCopiesConfigSnapshot(t *testing.T) {
	svcCtx := NewServiceContext(config.Config{AppID: "root"}, "root-version", Dependencies{})
	svcCtx.configValue.Store(config.Config{AppID: "request"})

	scoped := svcCtx.ScopedWithContext(context.Background())
	if scoped == nil {
		t.Fatal("ScopedWithContext() = nil")
	}
	if got := scoped.CurrentConfig().AppID; got != "request" {
		t.Fatalf("scoped AppID = %q, want request", got)
	}
}

// TestClientIPHonorsExplicitTrustedProxies 验证只有显式可信代理才能提供转发客户端地址。
func TestClientIPHonorsExplicitTrustedProxies(t *testing.T) {
	svcCtx := NewServiceContext(config.Config{TrustedProxies: []string{"10.0.0.0/8"}}, "", Dependencies{})

	trustedRequest := httptest.NewRequest("GET", "/", nil)
	trustedRequest.RemoteAddr = "10.0.0.10:8080"
	trustedRequest.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.11")
	if got := svcCtx.ClientIP(trustedRequest); got != "203.0.113.9" {
		t.Fatalf("可信代理解析客户端 IP=%q，期望 203.0.113.9", got)
	}

	untrustedRequest := httptest.NewRequest("GET", "/", nil)
	untrustedRequest.RemoteAddr = "192.0.2.20:8080"
	untrustedRequest.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := svcCtx.ClientIP(untrustedRequest); got != "192.0.2.20" {
		t.Fatalf("非可信来源不应采用转发头，实际客户端 IP=%q", got)
	}
}
