// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &NodeRoutesResource{}
var _ resource.ResourceWithImportState = &NodeRoutesResource{}

func NewNodeRoutesResource() resource.Resource {
	return &NodeRoutesResource{}
}

// NodeRoutesResource defines the resource implementation.
type NodeRoutesResource struct {
	client v1.HeadscaleServiceClient
}

type NodeRoutesResourceModel struct {
	Id     types.Int64 `tfsdk:"id"`
	NodeId types.Int64 `tfsdk:"node_id"`
	Routes types.Set   `tfsdk:"routes"`
}

func (r *NodeRoutesResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_routes"
}

func (r *NodeRoutesResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The resource approves node routes",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "ID of resources",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"node_id": schema.Int64Attribute{
				Required:    true,
				Description: "The node routes name.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"routes": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: `Approved routes on the node. e.g. "10.0.0.0/8" or "192.168.0.0/24"`,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.RegexMatches(regexp.MustCompile(`.+/.+`), "tag must follow scheme of `net/mask`"),
					),
				},
			},
		},
	}
}

func (r *NodeRoutesResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.client = config.client
}

func (r *NodeRoutesResource) readComputedFields(
	ctx context.Context,
	node *v1.Node,
	data *NodeRoutesResourceModel,
) diag.Diagnostics {
	approvedRoutes := node.GetApprovedRoutes()
	if approvedRoutes == nil {
		approvedRoutes = make([]string, 0)
	}
	routes, diags := types.SetValueFrom(ctx, types.StringType, approvedRoutes)
	data.Routes = routes
	return diags
}

func (r *NodeRoutesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NodeRoutesResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}
	data.Id = data.NodeId
	routes := []string{}
	for _, r := range data.Routes.Elements() {
		//nolint:forcetypeassert
		conv := r.(types.String)
		routes = append(routes, conv.ValueString())
	}

	response, err := r.client.SetApprovedRoutes(ctx, &v1.SetApprovedRoutesRequest{
		NodeId: uint64(data.NodeId.ValueInt64()),
		Routes: routes,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set node routes, got error: %s", err))
		return
	}
	resp.Diagnostics.Append(r.readComputedFields(ctx, response.GetNode(), &data)...)
	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeRoutesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NodeRoutesResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	response, err := r.client.GetNode(ctx, &v1.GetNodeRequest{NodeId: uint64(data.NodeId.ValueInt64())})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list nodes routes, got error: %s", err))
		return
	}
	if response.GetNode() == nil {
		resp.Diagnostics.AddError("node not found", "node not found")
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.readComputedFields(ctx, response.GetNode(), &data)...)
	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeRoutesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NodeRoutesResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}
	routes := []string{}
	req.Plan.GetAttribute(ctx, path.Root("routes"), &routes)
	response, err := r.client.SetApprovedRoutes(ctx, &v1.SetApprovedRoutesRequest{
		NodeId: uint64(data.NodeId.ValueInt64()),
		Routes: routes,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set node routes, got error: %s", err))
		return
	}
	resp.Diagnostics.Append(r.readComputedFields(ctx, response.GetNode(), &data)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeRoutesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NodeRoutesResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	_, err := r.client.SetApprovedRoutes(ctx, &v1.SetApprovedRoutesRequest{
		NodeId: uint64(data.NodeId.ValueInt64()),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set node routes, got error: %s", err))
		return
	}

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *NodeRoutesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.Atoi(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Fail to parse node id",
			fmt.Sprintf("Fail to parse node id: %s", err.Error()),
		)
		return
	}
	typedId := types.Int64Value(int64(id))
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), typedId)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), typedId)...)
}
