package runtimefile

import "testing"

// TestSectionSpecsValid 验证运行时配置分段注册完整且 key 集合同步。
func TestSectionSpecsValid(t *testing.T) {
	specs := sectionSpecs()
	if len(specs) == 0 {
		t.Fatal("runtime config section specs should not be empty")
	}
	keys := sectionKeys()
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.Key == "" {
			t.Fatal("runtime config section spec has empty key")
		}
		if spec.apply == nil {
			t.Fatalf("runtime config section %s missing apply function", spec.Key)
		}
		if _, ok := seen[spec.Key]; ok {
			t.Fatalf("runtime config section duplicate key=%s", spec.Key)
		}
		if _, ok := keys[spec.Key]; !ok {
			t.Fatalf("runtime config section key %s missing from key set", spec.Key)
		}
		seen[spec.Key] = struct{}{}
	}
}
