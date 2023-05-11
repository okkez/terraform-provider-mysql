package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func NewDatabaseDataSource() datasource.DataSource {
	return &DatabaseDataSource{}
}

var (
	_ datasource.DataSource              = &DatabaseDataSource{}
	_ datasource.DataSourceWithConfigure = &DatabaseDataSource{}
)

type DatabaseDataSource struct {
	mysqlConfig *MySQLConfiguration
}

type DatabaseDataSourceModel struct {
	ID                  types.String `tfsdk:"id"`
	Database            types.String `tfsdk:"database"`
	DefaultCharacterSet types.String `tfsdk:"default_character_set"`
	DefaultCollation    types.String `tfsdk:"default_collation"`
}

func (d *DatabaseDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (d *DatabaseDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The `mysql_database` data source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"database": schema.StringAttribute{
				MarkdownDescription: "The database name.",
				Required:            true,
			},
			"default_character_set": schema.StringAttribute{
				MarkdownDescription: "The default character set.",
				Computed:            true,
			},
			"default_collation": schema.StringAttribute{
				MarkdownDescription: "The default collation.",
				Computed:            true,
			},
		},
	}
}

func (d *DatabaseDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	db, err := getDatabase(ctx, d.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data DatabaseDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sql := `
SELECT
  SCHEMA_NAME
, DEFAULT_CHARACTER_SET_NAME
, DEFAULT_COLLATION_NAME
FROM
  INFORMATION_SCHEMA.SCHEMATA
WHERE
  SCHEMA_NAME = ?
`
	var args []interface{}
	args = append(args, data.Database.ValueString())

	tflog.Info(ctx, fmt.Sprintf("\n%s\n", sql), map[string]any{"args": args})

	var database, defaultCharacterSet, defaultCollation string
	if err := db.QueryRowContext(ctx, sql, args...).Scan(&database, &defaultCharacterSet, &defaultCollation); err != nil {
		resp.Diagnostics.AddError("Failed querying database", err.Error())
		return
	}

	data.ID = types.StringValue(database)
	data.DefaultCharacterSet = types.StringValue(defaultCharacterSet)
	data.DefaultCollation = types.StringValue(defaultCollation)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *DatabaseDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	if mysqlConfig, ok := req.ProviderData.(*MySQLConfiguration); ok {
		d.mysqlConfig = mysqlConfig
	} else {
		resp.Diagnostics.AddError("Failed type assertion", "")
	}
}
