package provider

import (
	"context"

	tfdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	tffunction "github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/path"
	tfprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	tfschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
)

type ConductorProviderModel struct {
	Endpoint      tftypes.String `tfsdk:"endpoint"`
	CustomHeaders tftypes.Map    `tfsdk:"custom_headers"`
}

type ConductorProvider struct {
	client *conductorHttpClient
}

var _ tfprovider.Provider = &ConductorProvider{}
var _ tfprovider.ProviderWithFunctions = &ConductorProvider{}

func New() func() tfprovider.Provider {
	return func() tfprovider.Provider {
		return &ConductorProvider{}
	}
}

func (p *ConductorProvider) Metadata(ctx context.Context, req tfprovider.MetadataRequest, resp *tfprovider.MetadataResponse) {
	resp.TypeName = "conductor"
}

func (p *ConductorProvider) Schema(ctx context.Context, req tfprovider.SchemaRequest, resp *tfprovider.SchemaResponse) {
	resp.Schema = tfschema.Schema{
		Description:         "The Conductor Provider used create resource on conductor platform",
		MarkdownDescription: "The Conductor Provider used create resource on conductor platform\nSee Conductor OSS reference: https://github.com/conductor-oss/conductor",
		Attributes: map[string]tfschema.Attribute{
			"endpoint": tfschema.StringAttribute{
				MarkdownDescription: "Endpoint of the Conductor API, e.g. - http://localhost:6251/",
				Required:            true,
			},
			"custom_headers": tfschema.MapAttribute{
				MarkdownDescription: "Custom http headers to send for every request",
				Optional:            true,
				ElementType:         tftypes.StringType,
			},
		},
	}
}

func (p *ConductorProvider) Configure(ctx context.Context, req tfprovider.ConfigureRequest, resp *tfprovider.ConfigureResponse) {
	var data ConductorProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Endpoint.IsNull() || data.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "endpoint can't be null", "")
		return
	}

	p.client = createConductorHttpClient(data)

	resp.DataSourceData = p // will be usable by DataSources
	resp.ResourceData = p   // will be usable by Resources
}

func (p *ConductorProvider) Resources(ctx context.Context) []func() tfresource.Resource {
	return []func() tfresource.Resource{
		NewTaskDefResource,
		NewWorkflowDefResource,
	}
}

func (p *ConductorProvider) DataSources(ctx context.Context) []func() tfdatasource.DataSource {
	return []func() tfdatasource.DataSource{}
}

func (p *ConductorProvider) Functions(ctx context.Context) []func() tffunction.Function {
	return []func() tffunction.Function{}
}
