package security

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Is999/go-utils/errors"
)

// securityVectorFile 表示测试使用的辅助结构。
type securityVectorFile struct {
	Version             int                          `json:"version"`             // Version 表示测试向量版本。
	SignVectors         []securitySignVector         `json:"signVectors"`         // SignVectors 表示签名测试向量。
	CipherHeaderVectors []securityCipherHeaderVector `json:"cipherHeaderVectors"` // CipherHeaderVectors 表示密文字段头测试向量。
	FieldLimitVectors   []securityFieldLimitVector   `json:"fieldLimitVectors"`   // FieldLimitVectors 表示字段限制测试向量。
}

// securitySignVector 表示测试使用的辅助结构。
type securitySignVector struct {
	Name      string         `json:"name"`      // Name 表示测试名称。
	AppID     string         `json:"appID"`     // AppID 表示应用 ID。
	TraceID   string         `json:"traceID"`   // TraceID 表示链路追踪 ID。
	Timestamp string         `json:"timestamp"` // Timestamp 表示请求时间戳。
	Fields    []string       `json:"fields"`    // Fields 表示参与计算的字段集合。
	Data      map[string]any `json:"data"`      // Data 表示响应数据。
	Expected  string         `json:"expected"`  // Expected 表示期望结果。
}

// securityCipherHeaderVector 表示测试使用的辅助结构。
type securityCipherHeaderVector struct {
	Name     string   `json:"name"`     // Name 表示测试名称。
	Fields   []string `json:"fields"`   // Fields 表示参与计算的字段集合。
	Expected string   `json:"expected"` // Expected 表示期望结果。
}

// securityFieldLimitVector 表示测试使用的辅助结构。
type securityFieldLimitVector struct {
	Name         string   `json:"name"`         // Name 表示测试名称。
	Fields       []string `json:"fields"`       // Fields 表示参与计算的字段集合。
	ShouldReject bool     `json:"shouldReject"` // ShouldReject 表示测试字段。
}

// TestSecurityVectorsBuildSignString 固定前后端共享的签名串拼接样例。
func TestSecurityVectorsBuildSignString(t *testing.T) {
	vectors := loadSecurityVectors(t)
	for _, vector := range vectors.SignVectors {
		t.Run(vector.Name, func(t *testing.T) {
			got := BuildSignString(vector.Data, vector.Fields, vector.TraceID, vector.Timestamp, vector.AppID)
			if got != vector.Expected {
				t.Fatalf("BuildSignString() = %q, want %q", got, vector.Expected)
			}
		})
	}
}

// TestSecurityVectorsEncodeCipherParams 固定 X-Cipher 字段编码样例。
func TestSecurityVectorsEncodeCipherParams(t *testing.T) {
	vectors := loadSecurityVectors(t)
	for _, vector := range vectors.CipherHeaderVectors {
		t.Run(vector.Name, func(t *testing.T) {
			got := EncodeCipherParams(vector.Fields)
			if got != vector.Expected {
				t.Fatalf("EncodeCipherParams() = %q, want %q", got, vector.Expected)
			}
		})
	}
}

// TestSecurityVectorsFieldLimits 固定字段级安全处理数量边界。
func TestSecurityVectorsFieldLimits(t *testing.T) {
	vectors := loadSecurityVectors(t)
	for _, vector := range vectors.FieldLimitVectors {
		t.Run(vector.Name, func(t *testing.T) {
			err := ValidateSecurityFieldCount(vector.Fields, "security vector")
			if vector.ShouldReject && !errors.Is(err, ErrSecurityPayloadTooLarge) {
				t.Fatalf("ValidateSecurityFieldCount() error = %v, want ErrSecurityPayloadTooLarge", err)
			}
			if !vector.ShouldReject && err != nil {
				t.Fatalf("ValidateSecurityFieldCount() error = %v", err)
			}
		})
	}
}

// loadSecurityVectors 表示测试辅助逻辑。
func loadSecurityVectors(t *testing.T) securityVectorFile {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "security_vectors.json"))
	if err != nil {
		t.Fatalf("read security vectors: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var vectors securityVectorFile
	if err := decoder.Decode(&vectors); err != nil {
		t.Fatalf("decode security vectors: %v", err)
	}
	if vectors.Version != 2 {
		t.Fatalf("security vectors version = %d, want 2", vectors.Version)
	}
	return vectors
}
