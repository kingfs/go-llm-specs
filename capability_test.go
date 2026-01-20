package llmspecs

import (
	"reflect"
	"testing"
)

func TestCapability_Has(t *testing.T) {
	caps := ModalityTextIn | ModalityTextOut | CapFunctionCall

	if !caps.Has(ModalityTextIn) {
		t.Error("Expected to have ModalityTextIn")
	}
	if !caps.Has(CapFunctionCall) {
		t.Error("Expected to have CapFunctionCall")
	}
	if caps.Has(ModalityImageIn) {
		t.Error("Expected NOT to have ModalityImageIn")
	}

	// Test multiple at once
	if !caps.Has(ModalityTextIn | ModalityTextOut) {
		t.Error("Expected to have both text in and out")
	}
}

func TestCapability_All(t *testing.T) {
	tests := []struct {
		name     string
		input    Capability
		wantStr  string
		wantList []string
	}{
		{
			name:     "空值测试",
			input:    0,
			wantStr:  "None",
			wantList: []string{},
		},
		{
			name:     "单项测试",
			input:    ModalityTextIn,
			wantStr:  "TextIn",
			wantList: []string{"TextIn"},
		},
		{
			name:     "组合测试",
			input:    ModalityTextIn | CapFunctionCall,
			wantStr:  "TextIn|FunctionCall",
			wantList: []string{"TextIn", "FunctionCall"},
		},
		{
			name:     "未知位测试",
			input:    1 << 62, // 一个未定义的位
			wantStr:  "Unknown(0x4000000000000000)",
			wantList: []string{"Unknown(0x4000000000000000)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 测试 String()
			if got := tt.input.String(); got != tt.wantStr {
				t.Errorf("String() = %v, want %v", got, tt.wantStr)
			}
			// 测试 ToStrings()
			gotList := tt.input.ToStrings()
			if !reflect.DeepEqual(gotList, tt.wantList) {
				t.Errorf("ToStrings() = %v, want %v", gotList, tt.wantList)
			}
		})
	}
}
