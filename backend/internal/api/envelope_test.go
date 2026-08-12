package api

import "testing"

func TestFailureEnvelopeUsesChineseFieldErrors(t *testing.T) {
	got := Failure("VALIDATION_FAILED", "请检查输入", FieldError{"quantity": "数量必须大于零"})
	if got.OK || got.Error == nil || got.Error.Code != "VALIDATION_FAILED" || got.Error.Fields["quantity"] == "" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
}
