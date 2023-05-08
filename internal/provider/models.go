package provider

import (
	"fmt"
	"math/rand"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type UserModel struct {
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
}

func NewUser(name, host string) UserModel {
	return UserModel{
		Name: types.StringValue(name),
		Host: types.StringValue(host),
	}
}

func NewRandomUser(prefix, host string) UserModel {
	return NewUser(fmt.Sprintf("%s-%04d", prefix, rand.Intn(1000)), host)
}

func (u *UserModel) GetName() string {
	return u.Name.ValueString()
}

func (u *UserModel) GetHost() string {
	return u.Host.ValueString()
}

func (u *UserModel) GetID() string {
	return fmt.Sprintf("%s@%s", u.GetName(), u.GetHost())
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

func NewRandomRole(prefix, host string) RoleModel {
	return NewRole(fmt.Sprintf("%s-%04d", prefix, rand.Intn(1000)), host)
}

func (r *RoleModel) GetName() string {
	return r.Name.ValueString()
}

func (r *RoleModel) GetHost() string {
	return r.Host.ValueString()
}

func (r *RoleModel) GetID() string {
	return fmt.Sprintf("%s@%s", r.GetName(), r.GetHost())
}
