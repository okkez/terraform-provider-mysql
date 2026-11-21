package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// stringEquivalence decides whether two strings returned by or sent to MySQL have the same
// meaning although they are written differently. MySQL normalizes some values, such as
// lowercasing character set names, so comparing them as plain strings leaves a diff after
// every apply.
type stringEquivalence interface {
	typeName() string
	equivalent(a, b string) bool
}

// semanticStringType is a string type whose values are compared with E when Terraform
// decides whether the value read from the server differs from the prior one.
type semanticStringType[E stringEquivalence] struct {
	basetypes.StringType
}

var _ basetypes.StringTypable = semanticStringType[charsetNameEquivalence]{}

func (t semanticStringType[E]) Equal(o attr.Type) bool {
	_, ok := o.(semanticStringType[E])
	return ok
}

func (t semanticStringType[E]) String() string {
	var e E
	return e.typeName()
}

func (t semanticStringType[E]) ValueType(_ context.Context) attr.Value {
	return semanticStringValue[E]{}
}

func (t semanticStringType[E]) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return semanticStringValue[E]{StringValue: in}, nil
}

func (t semanticStringType[E]) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}

	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type of %T", attrValue)
	}

	stringValuable, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to StringValuable: %v", diags)
	}

	return stringValuable, nil
}

// semanticStringValue is a value of semanticStringType.
type semanticStringValue[E stringEquivalence] struct {
	basetypes.StringValue
}

var _ basetypes.StringValuableWithSemanticEquals = semanticStringValue[charsetNameEquivalence]{}

func (v semanticStringValue[E]) Type(_ context.Context) attr.Type {
	return semanticStringType[E]{}
}

// Equal stays an exact comparison, as the framework requires. Equivalence is reported by
// StringSemanticEquals instead.
func (v semanticStringValue[E]) Equal(o attr.Value) bool {
	other, ok := o.(semanticStringValue[E])
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

func (v semanticStringValue[E]) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(semanticStringValue[E])
	if !ok {
		diags.AddError(
			"Semantic Equality Check Error",
			"An unexpected value type was received while performing semantic equality checks. "+
				"Please report this to the provider developers.\n\n"+
				"Expected Value Type: "+fmt.Sprintf("%T", v)+"\n"+
				"Got Value Type: "+fmt.Sprintf("%T", newValuable),
		)
		return false, diags
	}

	var e E
	return e.equivalent(v.ValueString(), newValue.ValueString()), diags
}

// Character set and collation names.

type charsetNameEquivalence struct{}

func (charsetNameEquivalence) typeName() string            { return "CharsetNameType" }
func (charsetNameEquivalence) equivalent(a, b string) bool { return charsetNameEquivalent(a, b) }

type (
	CharsetNameType  = semanticStringType[charsetNameEquivalence]
	CharsetNameValue = semanticStringValue[charsetNameEquivalence]
)

func NewCharsetNameValue(value string) CharsetNameValue {
	return CharsetNameValue{StringValue: basetypes.NewStringValue(value)}
}

// charsetNameEquivalent ignores letter case and treats `utf8` as `utf8mb3`, which is what
// MySQL 8.0 and later report for it.
func charsetNameEquivalent(a, b string) bool {
	return normalizeCharsetName(a) == normalizeCharsetName(b)
}

func normalizeCharsetName(name string) string {
	name = strings.ToLower(name)
	if name == "utf8" {
		return "utf8mb3"
	}
	if rest, ok := strings.CutPrefix(name, "utf8_"); ok {
		return "utf8mb3_" + rest
	}
	return name
}

// Global variable values.

type globalVariableValueEquivalence struct{}

func (globalVariableValueEquivalence) typeName() string { return "GlobalVariableValueType" }
func (globalVariableValueEquivalence) equivalent(a, b string) bool {
	return globalVariableValueEquivalent(a, b)
}

type (
	GlobalVariableValueType  = semanticStringType[globalVariableValueEquivalence]
	GlobalVariableValueValue = semanticStringValue[globalVariableValueEquivalence]
)

func NewGlobalVariableValueValue(value string) GlobalVariableValueValue {
	return GlobalVariableValueValue{StringValue: basetypes.NewStringValue(value)}
}

// wordListPattern matches a value made only of words separated by commas, such as an enum
// value or `sql_mode`. Only such values are compared ignoring letter case and order, so that
// case-sensitive values such as paths are never treated as equal.
var wordListPattern = regexp.MustCompile(`^[A-Za-z0-9_,]*$`)

// globalVariableValueEquivalent treats `ON`/`TRUE` as `1` and `OFF`/`FALSE` as `0`, which is
// what MySQL reports for boolean variables, and compares comma separated words ignoring
// letter case and order, as MySQL normalizes both for enum and set variables.
func globalVariableValueEquivalent(a, b string) bool {
	if a == b {
		return true
	}
	if normalizeBooleanValue(a) == normalizeBooleanValue(b) {
		return true
	}
	if !wordListPattern.MatchString(a) || !wordListPattern.MatchString(b) {
		return false
	}
	wordsA, wordsB := upperWordSet(a), upperWordSet(b)
	if len(wordsA) != len(wordsB) {
		return false
	}
	for word := range wordsA {
		if _, ok := wordsB[word]; !ok {
			return false
		}
	}
	return true
}

func normalizeBooleanValue(value string) string {
	switch strings.ToUpper(value) {
	case "ON", "TRUE":
		return "1"
	case "OFF", "FALSE":
		return "0"
	}
	return value
}

func upperWordSet(value string) map[string]struct{} {
	words := map[string]struct{}{}
	for _, word := range strings.Split(strings.ToUpper(value), ",") {
		words[word] = struct{}{}
	}
	return words
}

// Privilege names.

type privTypeEquivalence struct{}

func (privTypeEquivalence) typeName() string            { return "PrivTypeType" }
func (privTypeEquivalence) equivalent(a, b string) bool { return privTypeEquivalent(a, b) }

type (
	PrivTypeType  = semanticStringType[privTypeEquivalence]
	PrivTypeValue = semanticStringValue[privTypeEquivalence]
)

func NewPrivTypeValue(value string) PrivTypeValue {
	return PrivTypeValue{StringValue: basetypes.NewStringValue(value)}
}

// privTypeEquivalent treats `ALL` as `ALL PRIVILEGES`, which is what `SHOW GRANTS` reports.
func privTypeEquivalent(a, b string) bool {
	return normalizePrivType(a) == normalizePrivType(b)
}

func normalizePrivType(privType string) string {
	privType = strings.ToUpper(privType)
	if privType == "ALL" {
		return "ALL PRIVILEGES"
	}
	return privType
}
