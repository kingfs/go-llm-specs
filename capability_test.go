package llmspecs

import "testing"

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
