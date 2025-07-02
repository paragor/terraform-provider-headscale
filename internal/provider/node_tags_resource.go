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
var _ resource.Resource = &NodeTagsResource{}
var _ resource.ResourceWithImportState = &NodeTagsResource{}

func NewNodeTagsResource() resource.Resource {
	return &NodeTagsResource{}
}

// NodeTagsResource defines the resource implementation.
type NodeTagsResource struct {
	client v1.HeadscaleServiceClient
}

type NodeTagsResourceModel struct {
	Id     types.Int64 `tfsdk:"id"`
	NodeId types.Int64 `tfsdk:"node_id"`
	Tags   types.Set   `tfsdk:"tags"`
}

func (r *NodeTagsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_tags"
}

func (r *NodeTagsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The node tags resource that create node tags",

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
				Description: "The node tags name.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"tags": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "ACL tags on the node.",
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.RegexMatches(regexp.MustCompile(`tag:[\w-]+`), "tag must follow scheme of `tag:<value>`"),
					),
				},
			},
		},
	}
}

func (r *NodeTagsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NodeTagsResource) readComputedFields(
	ctx context.Context,
	node *v1.Node,
	data *NodeTagsResourceModel,
) diag.Diagnostics {
	forcedTags := node.GetForcedTags()
	if forcedTags == nil {
		forcedTags = make([]string, 0)
	}
	tags, diags := types.SetValueFrom(ctx, types.StringType, forcedTags)
	data.Tags = tags
	return diags
}

func (r *NodeTagsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NodeTagsResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}
	data.Id = data.NodeId
	tags := []string{}
	for _, r := range data.Tags.Elements() {
		//nolint:forcetypeassert
		conv := r.(types.String)
		tags = append(tags, conv.ValueString())
	}

	response, err := r.client.SetTags(ctx, &v1.SetTagsRequest{
		NodeId: uint64(data.NodeId.ValueInt64()),
		Tags:   tags,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set node tags, got error: %s", err))
		return
	}
	resp.Diagnostics.Append(r.readComputedFields(ctx, response.GetNode(), &data)...)
	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeTagsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NodeTagsResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	response, err := r.client.GetNode(ctx, &v1.GetNodeRequest{NodeId: uint64(data.NodeId.ValueInt64())})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list nodes tags, got error: %s", err))
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

func (r *NodeTagsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NodeTagsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}
	tags := []string{}
	req.Plan.GetAttribute(ctx, path.Root("tags"), &tags)
	response, err := r.client.SetTags(ctx, &v1.SetTagsRequest{
		NodeId: uint64(data.NodeId.ValueInt64()),
		Tags:   tags,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set node tags, got error: %s", err))
		return
	}
	resp.Diagnostics.Append(r.readComputedFields(ctx, response.GetNode(), &data)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeTagsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NodeTagsResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	_, err := r.client.SetTags(ctx, &v1.SetTagsRequest{
		NodeId: uint64(data.NodeId.ValueInt64()),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set node tags, got error: %s", err))
		return
	}

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *NodeTagsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {

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
