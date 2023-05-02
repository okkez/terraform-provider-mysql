package provider

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	helper "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"golang.org/x/net/proxy"
)

// Ensure mysqlProvider satisfies various provider interfaces.
var _ provider.Provider = &mysqlProvider{}

// mysqlProvider defines the provider implementation.
type mysqlProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// mysqlProviderModel describes the provider data model.
type mysqlProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

type OneConnection struct {
	Db      *sql.DB
	Version *version.Version
}

type MySQLConfiguration struct {
	Config                 *mysql.Config
	MaxConnLifetime        time.Duration
	MaxOpenConns           int
	ConnectRetryTimeoutSec time.Duration
}

var (
	connectionCacheMtx sync.Mutex
	connectionCache    map[string]*OneConnection
)

func init() {
	connectionCacheMtx.Lock()
	defer connectionCacheMtx.Unlock()

	connectionCache = map[string]*OneConnection{}
}

func (p *mysqlProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "mysql"
	resp.Version = p.version
}

func (p *mysqlProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Example provider attribute",
				Optional:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *mysqlProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data mysqlProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Unknown MySQL endpoint", "")
	}
	if data.Username.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("username"), "Unknown MySQL username", "")
	}
	if data.Password.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("password"), "Unknown MySQL password", "")
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Set provider configurations using environment variables
	endpoint := os.Getenv("MYSQL_ENDPOINT")
	username := os.Getenv("MYSQL_USERNAME")
	password := os.Getenv("MYSQL_PASSWORD")

	if !data.Endpoint.IsNull() {
		endpoint = data.Endpoint.ValueString()
	}
	if !data.Username.IsNull() {
		username = data.Username.ValueString()
	}
	if !data.Password.IsNull() {
		password = data.Password.ValueString()
	}

	ctx = tflog.SetField(ctx, "mysql_endpoint", endpoint)
	ctx = tflog.SetField(ctx, "mysql_username", username)
	ctx = tflog.SetField(ctx, "mysql_password", password)
	ctx = tflog.MaskFieldValuesWithFieldKeys(ctx, "mysql_password")

	tflog.Debug(ctx, "Creating MySQL client")

	conf := mysql.Config{
		User:                    username,
		Passwd:                  password,
		Net:                     "tcp",
		Addr:                    endpoint,
		TLSConfig:               "false",
		AllowNativePasswords:    true,
		AllowCleartextPasswords: false,
		InterpolateParams:       true,
		Params:                  map[string]string{},
	}

	dialer, err := makeDialer(&data)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("proxy"),
			"Failed making dialer",
			fmt.Sprintf("%v", err),
		)
		return
	}

	mysql.RegisterDialContext("tcp", func(ctx context.Context, network string) (net.Conn, error) {
		return dialer.Dial("tcp", network)
	})

	mysqlConf := &MySQLConfiguration{
		Config:                 &conf,
		MaxConnLifetime:        time.Duration(8*60*60) * time.Second,
		MaxOpenConns:           5,
		ConnectRetryTimeoutSec: time.Duration(300) * time.Second,
	}

	resp.DataSourceData = mysqlConf
	resp.ResourceData = mysqlConf
}

func (p *mysqlProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewDatabaseResource,
		NewRoleResource,
		NewUserResource,
		NewDefaultRolesResource,
		NewGlobalVariableResource,
		NewGrantRoleResource,
		NewGrantPrivilegeResource,
	}
}

func (p *mysqlProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewTablesDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &mysqlProvider{
			version: version,
		}
	}
}

func makeDialer(data *mysqlProviderModel) (proxy.Dialer, error) {
	proxyFromEnv := proxy.FromEnvironment()

	// TODO implement.
	if data == nil {
		return nil, fmt.Errorf("error")
	}
	fmt.Printf("%+v\n", data)
	// if !data.Proxy.IsNull() {
	// 	proxyURL, err := url.Parse(data.Proxy.ValueString())
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	proxyDialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return proxyDialer, nil
	// }

	return proxyFromEnv, nil
}

// func connectToMySQL(ctx context.Context, conf *MySQLConfiguration) (*sql.DB, error) {
// 	conn, err := connectToMySQLInternal(ctx, conf)
// 	if err != nil {
// 		return nil, err
// 	}
// 	tflog.Info(ctx, "connect")
// 	return conn.Db, nil
// }

func connectToMySQLInternal(ctx context.Context, conf *MySQLConfiguration) (*OneConnection, error) {
	// This is fine - we'll connect serially, but we don't expect more than
	// 1 or 2 connections starting at once.
	connectionCacheMtx.Lock()
	defer connectionCacheMtx.Unlock()

	dsn := conf.Config.FormatDSN()
	if connectionCache[dsn] != nil {
		return connectionCache[dsn], nil
	}
	var db *sql.DB
	var err error

	driverName := "mysql"
	if conf.Config.Net == "cloudsql" {
		driverName = "cloudsql"
	}
	tflog.Debug(ctx, fmt.Sprintf("Using driverName: %s", driverName))

	// When provisioning a database server there can often be a lag between
	// when Terraform thinks it's available and when it is actually available.
	// This is particularly acute when provisioning a server and then immediately
	// trying to provision a database on it.
	retryError := helper.RetryContext(ctx, conf.ConnectRetryTimeoutSec, func() *helper.RetryError {
		db, err = sql.Open(driverName, dsn)
		if err != nil {
			if mysqlErrorNumber(err) != 0 || ctx.Err() != nil {
				return helper.NonRetryableError(err)
			}
			return helper.RetryableError(err)
		}

		err = db.PingContext(ctx)
		if err != nil {
			if mysqlErrorNumber(err) != 0 || ctx.Err() != nil {
				return helper.NonRetryableError(err)
			}

			return helper.RetryableError(err)
		}

		return nil
	})

	if retryError != nil {
		return nil, fmt.Errorf("could not connect to server: %s", retryError)
	}
	db.SetConnMaxLifetime(conf.MaxConnLifetime)
	db.SetMaxOpenConns(conf.MaxOpenConns)

	currentVersion, err := afterConnectVersion(ctx, db)
	tflog.Info(ctx, currentVersion.String())
	if err != nil {
		return nil, fmt.Errorf("failed running after connect command: %v", err)
	}

	connectionCache[dsn] = &OneConnection{
		Db:      db,
		Version: currentVersion,
	}
	tflog.Info(ctx, "connect internal")
	return connectionCache[dsn], nil
}

func afterConnectVersion(ctx context.Context, db *sql.DB) (*version.Version, error) {
	// Set up env so that we won't create users randomly.
	tflog.Info(ctx, "AAA Running after connect")
	currentVersion, err := serverVersion(db)
	if err != nil {
		return nil, fmt.Errorf("Failed getting server version: %v", err)
	}

	versionMinInclusive, _ := version.NewVersion("5.7.5")
	versionMaxExclusive, _ := version.NewVersion("8.0.0")
	if currentVersion.GreaterThanOrEqual(versionMinInclusive) &&
		currentVersion.LessThan(versionMaxExclusive) {
		// CONCAT and setting works even if there is no value.
		_, err = db.ExecContext(ctx, `SET SESSION sql_mode=CONCAT(@@sql_mode, ',NO_AUTO_CREATE_USER')`)
		if err != nil {
			return nil, fmt.Errorf("failed setting SQL mode: %v", err)
		}
	}

	return currentVersion, nil
}

func serverVersion(db *sql.DB) (*version.Version, error) {
	var versionString string
	err := db.QueryRow("SELECT @@GLOBAL.version").Scan(&versionString)
	if err != nil {
		return nil, err
	}

	versionString = strings.SplitN(versionString, ":", 2)[0]
	return version.NewVersion(versionString)
}

// 0 == not mysql error or not error at all.
func mysqlErrorNumber(err error) uint16 {
	if err == nil {
		return 0
	}
	me, ok := err.(*mysql.MySQLError)
	if !ok {
		return 0
	}
	return me.Number
}
