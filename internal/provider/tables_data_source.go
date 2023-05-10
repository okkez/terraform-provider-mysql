package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func NewTablesDataSource() datasource.DataSource {
	return &tablesDataSource{}
}

var (
	_ datasource.DataSource              = &tablesDataSource{}
	_ datasource.DataSourceWithConfigure = &tablesDataSource{}
)

type tablesDataSource struct {
	mysqlConfig *MySQLConfiguration
}

type tablesDataSourceModel struct {
	ID       types.String   `tfsdk:"id"`
	Database types.String   `tfsdk:"database"`
	Pattern  types.String   `tfsdk:"pattern"`
	Tables   []types.String `tfsdk:"tables"`
}

func (d *tablesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tables"
}

func (d *tablesDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The `mysql_tables` data source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"database": schema.StringAttribute{
				MarkdownDescription: "The database name.",
				Required:            true,
			},
			"pattern": schema.StringAttribute{
				MarkdownDescription: "Table name pattern. Show all tables if omitted.",
				Optional:            true,
			},
			"tables": schema.SetAttribute{
				MarkdownDescription: "Table names.",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *tablesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	db, err := getDatabase(ctx, d.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data tablesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	database, err := quoteIdentifier(ctx, db, data.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed quoting identifier", err.Error())
		return
	}

	sql := fmt.Sprintf("SHOW TABLES FROM %s", database)
	var args []interface{}
	if !data.Pattern.IsNull() {
		sql = sql + " LIKE ?"
		args = append(args, data.Pattern.ValueString())
	}

	tflog.Info(ctx, fmt.Sprintf("SQL: %s", sql), map[string]any{"pattern": data.Pattern.ValueString()})

	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError("Failed querying for tables", err.Error())
		return
	}
	defer rows.Close()

	var state tablesDataSourceModel
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			resp.Diagnostics.AddError("Failed scanning MySQL rows", err.Error())
			return
		}
		state.Tables = append(state.Tables, types.StringValue(table))
	}

	state.ID = types.StringValue(fmt.Sprintf("%s:%s", data.Database.ValueString(), data.Pattern.ValueString()))
	state.Database = data.Database
	state.Pattern = data.Pattern

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *tablesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	if mysqlConfig, ok := req.ProviderData.(*MySQLConfiguration); ok {
		d.mysqlConfig = mysqlConfig
	} else {
		resp.Diagnostics.AddError("Failed type assertion", "")
	}
}
