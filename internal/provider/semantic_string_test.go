package provider

import (
	"context"
	"testing"
)

type equivalenceCase struct {
	a, b string
	want bool
}

func runEquivalenceCases(t *testing.T, name string, f func(a, b string) bool, cases []equivalenceCase) {
	t.Helper()
	for _, c := range cases {
		if got := f(c.a, c.b); got != c.want {
			t.Errorf("%s(%q, %q) = %t, want %t", name, c.a, c.b, got, c.want)
		}
	}
}

func TestCharsetNameEquivalent(t *testing.T) {
	runEquivalenceCases(t, "charsetNameEquivalent", charsetNameEquivalent, []equivalenceCase{
		{"utf8mb4", "UTF8MB4", true},
		{"utf8mb4_bin", "UTF8MB4_BIN", true},
		{"utf8mb3", "utf8", true},
		{"utf8mb3_general_ci", "utf8_general_ci", true},
		{"utf8mb4", "latin1", false},
		{"utf8mb4", "utf8", false},
		{"utf8mb4_bin", "utf8mb4_general_ci", false},
	})
}

func TestGlobalVariableValueEquivalent(t *testing.T) {
	runEquivalenceCases(t, "globalVariableValueEquivalent", globalVariableValueEquivalent, []equivalenceCase{
		{"1", "ON", true},
		{"1", "on", true},
		{"1", "TRUE", true},
		{"0", "OFF", true},
		{"0", "false", true},
		{"1", "OFF", false},
		{"0", "ON", false},
		{"MINIMAL", "minimal", true},
		{"ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION", "STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION,ONLY_FULL_GROUP_BY", true},
		{"STRICT_TRANS_TABLES", "STRICT_TRANS_TABLES,ONLY_FULL_GROUP_BY", false},
		{"100000", "1000000", false},
		{"2", "2", true},
		{"", "", true},
		{"/var/lib/A", "/var/lib/a", false},
		{"a.b", "A.B", false},
	})
}

func TestPrivTypeEquivalent(t *testing.T) {
	runEquivalenceCases(t, "privTypeEquivalent", privTypeEquivalent, []equivalenceCase{
		{"ALL PRIVILEGES", "ALL", true},
		{"ALL", "ALL PRIVILEGES", true},
		{"SELECT", "SELECT", true},
		{"SELECT", "INSERT", false},
		{"ALL PRIVILEGES", "SELECT", false},
	})
}

func TestSemanticStringValue_StringSemanticEquals(t *testing.T) {
	ctx := context.Background()

	equal, diags := NewCharsetNameValue("utf8mb4").StringSemanticEquals(ctx, NewCharsetNameValue("UTF8MB4"))
	if !equal || diags.HasError() {
		t.Errorf("utf8mb4 vs UTF8MB4: got (%t, %v), want (true, no error)", equal, diags)
	}

	equal, diags = NewCharsetNameValue("utf8mb4").StringSemanticEquals(ctx, NewCharsetNameValue("latin1"))
	if equal || diags.HasError() {
		t.Errorf("utf8mb4 vs latin1: got (%t, %v), want (false, no error)", equal, diags)
	}

	equal, diags = NewCharsetNameValue("utf8mb4").StringSemanticEquals(ctx, NewPrivTypeValue("utf8mb4"))
	if equal || !diags.HasError() {
		t.Errorf("different value types: got (%t, %v), want (false, error)", equal, diags)
	}

	if NewCharsetNameValue("a").Equal(NewCharsetNameValue("A")) {
		t.Error("Equal must stay an exact comparison")
	}

	if (CharsetNameType{}).Equal(PrivTypeType{}) {
		t.Error("types with different equivalences must not be equal")
	}
}
