package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// privateStateGetter and privateStateSetter are the parts of the private state of a resource
// used here. The framework does not export its type.
type privateStateGetter interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
}

type privateStateSetter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// savePrivateString saves a string in the private state, which must hold JSON.
func savePrivateString(ctx context.Context, private privateStateSetter, key, value string) diag.Diagnostics {
	encoded, err := json.Marshal(value)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Failed encoding the private state", err.Error())
		return diags
	}
	return private.SetKey(ctx, key, encoded)
}

// loadPrivateString reports false when nothing is saved under the key, as for an imported
// resource or a state written by an earlier version of this provider.
func loadPrivateString(ctx context.Context, private privateStateGetter, key string) (string, bool, diag.Diagnostics) {
	encoded, diags := private.GetKey(ctx, key)
	if diags.HasError() || len(encoded) == 0 {
		return "", false, diags
	}
	var value string
	if err := json.Unmarshal(encoded, &value); err != nil {
		diags.AddError("Failed decoding the private state", err.Error())
		return "", false, diags
	}
	return value, true, diags
}
