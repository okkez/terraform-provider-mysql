package utils

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
)

func IDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "The identifier",
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

func NameAttribute(kind string, requireReplace bool) schema.StringAttribute {
	a := schema.StringAttribute{
		MarkdownDescription: fmt.Sprintf("The name of the %s", kind),
		Required:            true,
		Validators: []validator.String{
			stringvalidator.LengthAtMost(32),
		},
	}
	if requireReplace {
		a.PlanModifiers = []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		}
	}
	return a
}

func HostAttribute(kind string, requireReplace bool) schema.StringAttribute {
	a := schema.StringAttribute{
		MarkdownDescription: fmt.Sprintf("The source host of the %s. Defaults to `%%`", kind),
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString("%"),
		Validators: []validator.String{
			stringvalidator.LengthAtMost(255),
		},
	}
	if requireReplace {
		a.PlanModifiers = []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		}
	}
	return a
}
