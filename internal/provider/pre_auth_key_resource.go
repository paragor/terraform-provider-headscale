// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &PreAuthKeyResource{}
var _ resource.ResourceWithImportState = &PreAuthKeyResource{}

func NewPreAuthKeyResource() resource.Resource {
	return &PreAuthKeyResource{}
}

// PreAuthKeyResource defines the resource implementation.
type PreAuthKeyResource struct {
	client v1.HeadscaleServiceClient
}

type PreAuthKeyResourceModel struct {
	Id        types.Int64  `tfsdk:"id"`
	UserId    types.Int64  `tfsdk:"user_id"`
	Reusable  types.Bool   `tfsdk:"reusable"`
	Ephemeral types.Bool   `tfsdk:"ephemeral"`
	Ttl       types.String `tfsdk:"ttl"`
	ACLTags   types.Set    `tfsdk:"acl_tags"`

	Expired types.Bool `tfsdk:"expired"`

	CreatedAt  types.String `tfsdk:"created_at"`
	Expiration types.String `tfsdk:"expiration"`
	Key        types.String `tfsdk:"key"`
}

func (r *PreAuthKeyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pre_auth_key"
}

func (r *PreAuthKeyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The pre auth key resource allows you to create a pre auth key that can be used to register a new device on the Headscale instance. By default keys that are created with this resource will be not reusable, not ephemeral, and expire in 1 hour. Keys cannot be modified, so any change to the input on this resource will cause the key to be expired and a new key to be created.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "ID of resources",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.Int64Attribute{
				MarkdownDescription: "User Id",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"reusable": schema.BoolAttribute{
				MarkdownDescription: "Define option for reuse pre auth key",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"ephemeral": schema.BoolAttribute{
				MarkdownDescription: "Define pre auth key as ephemeral",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},

			"ttl": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: `The time until the key expires. Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". Defaults to "1h"`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^\d+(ns|us|µs|ms|s|m|h)$`), `Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h"`),
				},
			},
			"acl_tags": schema.SetAttribute{
				Computed:    true,
				Optional:    true,
				ElementType: types.StringType,
				Description: "ACL tags on the pre auth key.",
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.RegexMatches(regexp.MustCompile(`tag:[\w-]+`), "tag must follow scheme of `tag:<value>`"),
					),
				},
			},

			"expired": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "expiration of pre auth key",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Computed:    true,
				Description: "The pre auth key.",
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"expiration": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "expiration of pre auth key",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "time of creation pre auth key",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *PreAuthKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PreAuthKeyResource) readComputedFields(key *v1.PreAuthKey, data *PreAuthKeyResourceModel) {
	data.CreatedAt = types.StringValue(key.GetCreatedAt().AsTime().Format(time.RFC3339))
	data.Expiration = types.StringValue(key.GetExpiration().AsTime().Format(time.RFC3339))
	data.Key = types.StringValue(key.GetKey())
	data.Id = types.Int64Value(int64(key.GetId()))
	data.Expired = types.BoolValue(time.Now().After(key.Expiration.AsTime()))
	data.Reusable = types.BoolValue(key.GetReusable())
	data.Ephemeral = types.BoolValue(key.GetEphemeral())
}

func (r *PreAuthKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PreAuthKeyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	aclTags := []string{}
	for _, r := range data.ACLTags.Elements() {
		//nolint:forcetypeassert
		conv := r.(types.String)
		aclTags = append(aclTags, conv.ValueString())
	}

	var expiration *timestamppb.Timestamp
	if data.Ttl.IsNull() {
		expiration = timestamppb.New(time.Now().Add(1 * time.Hour))
	} else {
		ttl, err := time.ParseDuration(data.Ttl.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Parse TTL Error", fmt.Sprintf("Unable to parse ttl, got error: %s", err))
			return
		}
		expiration = timestamppb.New(time.Now().Add(ttl))
	}
	response, err := r.client.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
		User:       uint64(data.UserId.ValueInt64()),
		Reusable:   data.Reusable.ValueBool(),
		Ephemeral:  data.Ephemeral.ValueBool(),
		Expiration: expiration,
		AclTags:    aclTags,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create pre auth key, got error: %s", err))
		return
	}
	r.readComputedFields(response.PreAuthKey, &data)
	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PreAuthKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PreAuthKeyResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	response, err := r.client.ListPreAuthKeys(ctx, &v1.ListPreAuthKeysRequest{User: uint64(data.UserId.ValueInt64())})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list pre auth keys, got error: %s", err))
		return
	}

	id := uint64(data.Id.ValueInt64())
	var preAuthKey *v1.PreAuthKey
	for _, found := range response.PreAuthKeys {
		if found.Id != id {
			continue
		}
		preAuthKey = found
	}
	if preAuthKey == nil {
		resp.Diagnostics.AddError("Pre auth key not found", "Pre auth key not found")
		resp.State.RemoveResource(ctx)
		return
	}
	r.readComputedFields(preAuthKey, &data)
	data.Reusable = types.BoolValue(preAuthKey.GetReusable())
	data.Ephemeral = types.BoolValue(preAuthKey.GetEphemeral())

	preAuthKey.GetAclTags()
	aclTags, diags := types.SetValueFrom(ctx, types.StringType, preAuthKey.GetAclTags())
	resp.Diagnostics.Append(diags...)
	data.ACLTags = aclTags

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if time.Now().After(preAuthKey.GetExpiration().AsTime()) {
		resp.State.RemoveResource(ctx)
	}
}

func (r *PreAuthKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Error updating pre auth key",
		"Pre auth keys cannot be updated",
	)
}

func (r *PreAuthKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PreAuthKeyResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.ExpirePreAuthKey(ctx, &v1.ExpirePreAuthKeyRequest{
		User: uint64(data.UserId.ValueInt64()),
		Key:  data.Key.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete pre auth key, got error: %s", err))
		return
	}
}

func (r *PreAuthKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, ",")

	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: user_id,key_id. Got: %q", req.ID),
		)
		return
	}
	userId, err := strconv.Atoi(idParts[0])
	if err != nil {
		resp.Diagnostics.AddError(
			"Fail to parse user id",
			fmt.Sprintf("Fail to parse user id: %s", err.Error()),
		)
		return
	}
	keyId, err := strconv.Atoi(idParts[1])
	if err != nil {
		resp.Diagnostics.AddError(
			"Fail to parse key id",
			fmt.Sprintf("Fail to parse key id: %s", err.Error()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), types.Int64Value(int64(userId)))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(int64(keyId)))...)
}
