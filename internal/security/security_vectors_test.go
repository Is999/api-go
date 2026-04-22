package security

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Is999/go-utils/errors"
)

type securityVectorFile struct {
	Version             int                          `json:"version"`
	SignVectors         []securitySignVector         `json:"signVectors"`
	CipherHeaderVectors []securityCipherHeaderVector `json:"cipherHeaderVectors"`
	FieldLimitVectors   []securityFieldLimitVector   `json:"fieldLimitVectors"`
}

type securitySignVector struct {
	Name      string         `json:"name"`
	AppID     string         `json:"appID"`
	TraceID   string         `json:"traceID"`
	Timestamp string         `json:"timestamp"`
	Fields    []string       `json:"fields"`
	Data      map[string]any `json:"data"`
	Expected  string         `json:"expected"`
}

type securityCipherHeaderVector struct {
	Name     string   `json:"name"`
	Fields   []string `json:"fields"`
	Expected string   `json:"expected"`
}

type securityFieldLimitVector struct {
	Name         string   `json:"name"`
	Fields       []string `json:"fields"`
	ShouldReject bool     `json:"shouldReject"`
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
	if vectors.Version != 1 {
		t.Fatalf("security vectors version = %d, want 1", vectors.Version)
	}
	return vectors
}
