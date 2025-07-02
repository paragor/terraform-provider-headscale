// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"github.com/paragor/terraform-provider-headscale/internal/headscaleclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Ensure HeadscaleProvider satisfies various provider interfaces.
var _ provider.Provider = &HeadscaleProvider{}

// HeadscaleProvider defines the provider implementation.
type HeadscaleProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

type HeadscaleProviderConfiguration struct {
	client v1.HeadscaleServiceClient
}

// HeadscaleProviderModel describes the provider data model.
type HeadscaleProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	ApiKey   types.String `tfsdk:"api_key"`
	TLS      *struct {
		Insecure      types.Bool   `tfsdk:"insecure"`
		CaPem         types.String `tfsdk:"ca_pem"`
		ClientCertPem types.String `tfsdk:"client_cert_pem"`
		ClientKeyPem  types.String `tfsdk:"client_key_pem"`
	} `tfsdk:"tls"`
}

func (p *HeadscaleProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "headscale"
	resp.Version = p.version
}

func (p *HeadscaleProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: `
GRPC endpoint, for example:
 - "foo.googleapis.com:8080"
 - "dns:///foo.googleapis.com:8080"
 - "dns:///foo.googleapis.com"
 - "dns:///10.0.0.213:8080"
 - "dns:///%5B2001:db8:85a3:8d3:1319:8a2e:370:7348%5D:443"
 - "dns://8.8.8.8/foo.googleapis.com:8080"
 - "dns://8.8.8.8/foo.googleapis.com"
 - "unix:///path/to/socket"

If it is not set, provider try to take it from env "HEADSCALE_ENDPOINT"
`,
				Optional: true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: `
API key token optional.
If it is not set, provider try to take it from env "HEADSCALE_API_KEY"
`,
				Optional: true,
			},
			"tls": schema.MapNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"insecure": schema.BoolAttribute{
							MarkdownDescription: `
Configure connection to use insecure tls connection. 
If it is not set, provider try to take it from env "HEADSCALE_TLS_INSECURE"
`,
							Optional: true,
						},
						"ca_pem": schema.StringAttribute{
							MarkdownDescription: `
Configure connection to use tls CA certificate in PEM format. 
If it is not set, provider try to take file from env "HEADSCALE_TLS_CA_PATH" and read it
`,
							Optional: true,
						},
						"client_cert_pem": schema.StringAttribute{
							MarkdownDescription: `
Configure connection to use client certificate in PEM format. 
If it is not set, provider try to take file from env "HEADSCALE_TLS_CLIENT_CERT_PATH" and read it
`,
							Optional: true,
						},
						"client_key_pem": schema.StringAttribute{
							MarkdownDescription: `
Configure connection to use client certificate in PEM format.
If it is not set, provider try to take file from env "HEADSCALE_TLS_CLIENT_KEY_PATH" and read it
`,
							Optional: true,
						},
					},
				},
				MarkdownDescription: "Configure TLS connection",
			},
		},
	}
}

func (p *HeadscaleProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data HeadscaleProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	target := os.Getenv("HEADSCALE_ENDPOINT")
	if !data.Endpoint.IsNull() {
		target = data.Endpoint.ValueString()
	}
	if target == "" {
		resp.Diagnostics.AddError("endpoint is not set", "provider's attribute 'endpoint' is not configured")
		return
	}
	connOpts := []grpc.DialOption{}

	apiKey := os.Getenv("HEADSCALE_API_KEY")
	if !data.ApiKey.IsNull() {
		apiKey = data.ApiKey.ValueString()
	}
	if apiKey != "" {
		connOpts = append(connOpts, grpc.WithPerRPCCredentials(headscaleclient.NewGRPCTokenAuth(apiKey)))
	}

	insecure := false
	if data.TLS != nil && !data.TLS.Insecure.IsNull() {
		insecure = data.TLS.Insecure.ValueBool()
	} else {
		insecureEnv := os.Getenv("HEADSCALE_TLS_INSECURE")
		if insecureEnv != "" {
			result, err := strconv.ParseBool(insecureEnv)
			if err != nil {
				resp.Diagnostics.AddError(
					"Fail to parse env HEADSCALE_TLS_INSECURE",
					fmt.Sprintf("Fail to parse env HEADSCALE_TLS_INSECURE: %s", err.Error()),
				)
			}
			insecure = result
		}
	}

	tlsConfig := &tls.Config{
		//nolint:gosec
		InsecureSkipVerify: insecure,
	}

	caPemPathEnv := os.Getenv("HEADSCALE_TLS_CA_PATH")
	if data.TLS != nil && !data.TLS.CaPem.IsNull() {
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM([]byte(data.TLS.CaPem.ValueString())); !ok {
			resp.Diagnostics.AddError("Fail to decode tls.ca_pem", "Fail to decode tls.ca_pem")
			return
		}
		tlsConfig.RootCAs = certPool
	} else if caPemPathEnv != "" {
		cert, err := os.ReadFile(caPemPathEnv)
		if err != nil {
			resp.Diagnostics.AddError(
				"Fail to read HEADSCALE_TLS_CA_PATH",
				fmt.Sprintf("Fail to read ca pem from HEADSCALE_TLS_CA_PATH (%s): %s", caPemPathEnv, err),
			)
			return
		}
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM(cert); !ok {
			resp.Diagnostics.AddError("Fail to decode tls.HEADSCALE_TLS_CA_PATH", "Fail to decode HEADSCALE_TLS_CA_PATH")
			return
		}
		tlsConfig.RootCAs = certPool
	}

	var tlsClientCertPem []byte
	var tlsClientKeyPem []byte

	tlsClientCertPathEnv := os.Getenv("HEADSCALE_TLS_CLIENT_CERT_PATH")
	if data.TLS != nil && !data.TLS.ClientCertPem.IsNull() {
		tlsClientCertPem = []byte(data.TLS.ClientCertPem.ValueString())
	} else if tlsClientCertPathEnv != "" {
		var err error
		tlsClientCertPem, err = os.ReadFile(tlsClientCertPathEnv)
		if err != nil {
			resp.Diagnostics.AddError(
				"Fail to read HEADSCALE_TLS_CLIENT_CERT_PATH",
				fmt.Sprintf(
					"Fail to client cert pem from HEADSCALE_TLS_CLIENT_CERT_PATH (%s): %s",
					tlsClientCertPathEnv,
					err,
				),
			)
			return
		}
	}

	tlsClientKeyPemPathEnv := os.Getenv("HEADSCALE_TLS_CLIENT_KEY_PATH")
	if data.TLS != nil && !data.TLS.ClientCertPem.IsNull() {
		tlsClientKeyPem = []byte(data.TLS.ClientKeyPem.ValueString())
	} else if tlsClientKeyPemPathEnv != "" {
		var err error
		tlsClientKeyPem, err = os.ReadFile(tlsClientKeyPemPathEnv)
		if err != nil {
			resp.Diagnostics.AddError(
				"Fail to read HEADSCALE_TLS_CLIENT_KEY_PATH",
				fmt.Sprintf(
					"Fail to client cert pem from HEADSCALE_TLS_CLIENT_KEY_PATH (%s): %s",
					tlsClientKeyPemPathEnv,
					err,
				),
			)
			return
		}
	}

	if len(tlsClientCertPem) > 0 || len(tlsClientKeyPem) > 0 {
		cert, err := tls.X509KeyPair(tlsClientCertPem, tlsClientKeyPem)
		if err != nil {
			resp.Diagnostics.AddError(
				"Fail to read build tls client key pair",
				fmt.Sprintf("Fail to read build tls client key pair: %s", err),
			)
			return
		}
		tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
	}

	connOpts = append(connOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))

	conn, err := grpc.NewClient(target, connOpts...)
	if err != nil {
		resp.Diagnostics.AddError("Create GRPC client error", fmt.Sprintf("cant create grpc client, got error: %s", err.Error()))
		return
	}

	config := &HeadscaleProviderConfiguration{
		client: v1.NewHeadscaleServiceClient(conn),
	}
	resp.DataSourceData = config
	resp.ResourceData = config
}

func (p *HeadscaleProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPreAuthKeyResource,
		NewApiKeyResource,
		NewUserResource,
		NewNodeTagsResource,
		NewNodeRoutesResource,
	}
}

func (p *HeadscaleProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewNodesDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &HeadscaleProvider{
			version: version,
		}
	}
}
