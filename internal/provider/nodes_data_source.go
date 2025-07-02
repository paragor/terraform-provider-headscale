// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &NodesDataSource{}

func NewNodesDataSource() datasource.DataSource {
	return &NodesDataSource{}
}

// NodesDataSource defines the data source implementation.
type NodesDataSource struct {
	client v1.HeadscaleServiceClient
}

// NodesDataSourceModel describes the data source data model.
type NodesDataSourceModel struct {
	Nodes []NodeModel `tfsdk:"nodes"`
}
type NodeModel struct {
	Id     types.Int64  `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	UserId types.Int64  `tfsdk:"user_id"`
}

func (d *NodesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nodes"
}

func (d *NodesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Nodes data source",

		Attributes: map[string]schema.Attribute{
			"nodes": schema.ListNestedAttribute{
				MarkdownDescription: "Nodes identifier",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Computed:    true,
							Description: "The id of the device",
						},
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The device's name.",
						},
						"user_id": schema.Int64Attribute{
							Computed:    true,
							Description: "The ID of the user who owns the device.",
						},
					},
				},
			},
		},
	}
}

func (d *NodesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*HeadscaleProviderConfiguration)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *HeadscaleProviderConfiguration, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = config.client
}

func (d *NodesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NodesDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	response, err := d.client.ListNodes(ctx, &v1.ListNodesRequest{})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list nodes, got error: %s", err))
		return
	}
	nodes := response.GetNodes()
	if nodes == nil {
		resp.Diagnostics.AddError("Client Error", "Headscale return null instead of list of nodes")
		return
	}

	result := make([]NodeModel, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, NodeModel{
			Id:     types.Int64Value(int64(node.GetId())),
			Name:   types.StringValue(node.GetName()),
			UserId: types.Int64Value(int64(node.GetUser().GetId())),
		})
	}
	data.Nodes = result

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
