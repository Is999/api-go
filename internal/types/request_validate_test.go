package types

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zeromicro/go-zero/core/validation"
	"github.com/zeromicro/go-zero/rest/httpx"
)

var (
	_ validation.Validator = (*RegisterReq)(nil)        // _ 校验 RegisterReq 实现 validation.Validator。
	_ validation.Validator = (*LoginReq)(nil)           // _ 校验 LoginReq 实现 validation.Validator。
	_ validation.Validator = (*ConfigItemQueryReq)(nil) // _ 校验 ConfigItemQueryReq 实现 validation.Validator。
	_ validation.Validator = (*UserRuntimeSyncReq)(nil) // _ 校验 UserRuntimeSyncReq 实现 validation.Validator。
)

// TestAuthReqValidate 验证认证请求基础校验和字段归一化。
func TestAuthReqValidate(t *testing.T) {
	registerReq := &RegisterReq{
		Username: " demo_user ",
		Password: "secret123",
		Nickname: " Demo ",
		Email:    " demo@example.com ",
		Phone:    " 13800138000 ",
	}
	if err := registerReq.Validate(); err != nil {
		t.Fatalf("RegisterReq.Validate() error = %v", err)
	}
	if registerReq.Username != "demo_user" || registerReq.Nickname != "Demo" || registerReq.Email != "demo@example.com" || registerReq.Phone != "13800138000" {
		t.Fatalf("RegisterReq.Validate() did not trim fields: %+v", registerReq)
	}

	cases := []struct {
		name string       // name 表示测试场景名称。
		req  *RegisterReq // req 表示测试字段。
	}{
		{name: "用户名过短", req: &RegisterReq{Username: "ab", Password: "secret123"}},
		{name: "密码为空", req: &RegisterReq{Username: "demo_user", Password: "   "}},
		{name: "密码超过bcrypt字节上限", req: &RegisterReq{Username: "demo_user", Password: strings.Repeat("密", 25)}},
		{name: "昵称过长", req: &RegisterReq{Username: "demo_user", Password: "secret123", Nickname: strings.Repeat("名", authNicknameMaxLength+1)}},
		{name: "邮箱过长", req: &RegisterReq{Username: "demo_user", Password: "secret123", Email: strings.Repeat("a", authEmailMaxLength+1)}},
		{name: "手机号过长", req: &RegisterReq{Username: "demo_user", Password: "secret123", Phone: strings.Repeat("1", authPhoneMaxLength+1)}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.req.Validate(); err == nil {
				t.Fatal("RegisterReq.Validate() should reject invalid request")
			}
		})
	}

	loginReq := &LoginReq{IdentityType: " username ", IdentityValue: " demo_user ", Password: "secret123"}
	if err := loginReq.Validate(); err != nil {
		t.Fatalf("LoginReq.Validate() error = %v", err)
	}
	if loginReq.IdentityType != LoginIdentityTypeUsername || loginReq.IdentityValue != "demo_user" {
		t.Fatalf("LoginReq.Validate() identity = %s:%s, want username:demo_user", loginReq.IdentityType, loginReq.IdentityValue)
	}
	if err := (&LoginReq{IdentityType: LoginIdentityTypeUsername, IdentityValue: "demo_user", Password: " "}).Validate(); err == nil {
		t.Fatal("LoginReq.Validate() should reject blank password")
	}
	if err := (&LoginReq{IdentityType: LoginIdentityTypeUsername, IdentityValue: "demo_user", Password: strings.Repeat("a", authPasswordMaxBytes)}).Validate(); err != nil {
		t.Fatalf("LoginReq.Validate() should accept a 72-byte password: %v", err)
	}
	if err := (&LoginReq{IdentityType: LoginIdentityTypeUsername, IdentityValue: "demo_user", Password: strings.Repeat("a", authPasswordMaxBytes+1)}).Validate(); err == nil {
		t.Fatal("LoginReq.Validate() should reject a 73-byte password")
	}
	if err := (&LoginReq{IdentityType: "oauth", IdentityValue: "demo", Password: "secret123"}).Validate(); err == nil {
		t.Fatal("LoginReq.Validate() should reject oauth password login")
	}
}

// TestGoZeroParseCallsValidate 验证 go-zero 解析请求后会调用 Validate。
func TestGoZeroParseCallsValidate(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"identityType":"username","identityValue":"demo_user","password":"   "}`))
	req.Header.Set("Content-Type", "application/json")

	var parsed LoginReq
	if err := httpx.Parse(req, &parsed); err == nil {
		t.Fatal("httpx.Parse() should call LoginReq.Validate()")
	}
}

// TestConfigItemQueryReqValidate 验证运行态配置查询参数默认值和边界。
func TestConfigItemQueryReqValidate(t *testing.T) {
	req := &ConfigItemQueryReq{
		Keyword:       " security ",
		SensitiveOnly: true,
		Page:          -1,
		PageSize:      1000,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("ConfigItemQueryReq.Validate() error = %v", err)
	}
	if req.Keyword != "security" || req.Page != 1 || req.PageSize != 100 {
		t.Fatalf("ConfigItemQueryReq.Validate() got %+v, want trimmed keyword and bounded page", req)
	}
	if err := (&ConfigItemQueryReq{Keyword: strings.Repeat("a", 129)}).Validate(); err == nil {
		t.Fatal("ConfigItemQueryReq.Validate() should reject long keyword")
	}
}

// TestUserRuntimeSyncReqValidate 验证内网用户运行态同步参数默认值和边界。
func TestUserRuntimeSyncReqValidate(t *testing.T) {
	req := &UserRuntimeSyncReq{ID: 42, Reason: " manual sync "}
	if err := req.Validate(); err != nil {
		t.Fatalf("UserRuntimeSyncReq.Validate() error = %v", err)
	}
	if !req.Profile || req.Sessions || req.Reason != "manual sync" {
		t.Fatalf("UserRuntimeSyncReq.Validate() got %+v, want profile default true and trimmed reason", req)
	}
	if err := (&UserRuntimeSyncReq{ID: 0}).Validate(); err == nil {
		t.Fatal("UserRuntimeSyncReq.Validate() should reject empty user ID")
	}
	if err := (&UserRuntimeSyncReq{ID: 42, Reason: strings.Repeat("a", userRuntimeSyncReasonMaxLength+1)}).Validate(); err == nil {
		t.Fatal("UserRuntimeSyncReq.Validate() should reject long reason")
	}
	if err := (&UserRuntimeSyncReq{ID: 42, Sessions: true}).Validate(); err == nil {
		t.Fatal("UserRuntimeSyncReq.Validate() should require authVersion for session invalidation")
	}
}
