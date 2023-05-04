package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type UserModel struct {
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
}

type RoleModel struct {
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
}

var RoleTypes = map[string]attr.Type{
	"name": types.StringType,
	"host": types.StringType,
}

func NewRole(name, host string) RoleModel {
	return RoleModel{
		Name: types.StringValue(name),
		Host: types.StringValue(host),
	}
}

func (r *RoleModel) GetName() string {
	return r.Name.ValueString()
}

func (r *RoleModel) GetHost() string {
	return r.Host.ValueString()
}
