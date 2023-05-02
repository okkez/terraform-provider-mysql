package utils

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
)

func IDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "user identifier",
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

func NameAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "",
		Required:            true,
		Validators: []validator.String{
			stringvalidator.LengthAtMost(32),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

func HostAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "",
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString("%"),
		Validators: []validator.String{
			stringvalidator.LengthAtMost(255),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}
