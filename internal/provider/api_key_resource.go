// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ApiKeyResource{}

func NewApiKeyResource() resource.Resource {
	return &ApiKeyResource{}
}

// ApiKeyResource defines the resource implementation.
type ApiKeyResource struct {
	client v1.HeadscaleServiceClient
}

type ApiKeyResourceModel struct {
	Id  types.String `tfsdk:"id"`
	Ttl types.String `tfsdk:"ttl"`

	Expired types.Bool `tfsdk:"expired"`

	CreatedAt  types.String `tfsdk:"created_at"`
	Expiration types.String `tfsdk:"expiration"`
	Key        types.String `tfsdk:"key"`
}

func (r *ApiKeyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *ApiKeyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The api key resource allows you to make a api calls to headscale as admin",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of resources",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"ttl": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: `The time until the key expires. Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". Defaults to "2160h" that equal 90 days`,
				Default:             stringdefault.StaticString("2160h"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^\d+(ns|us|µs|ms|s|m|h)$`), `Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h"`),
				},
			},
			"expired": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "expiration of api key",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Computed:    true,
				Description: "The api key.",
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"expiration": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "expiration of api key",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "time of creation api key",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ApiKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ApiKeyResource) readComputedFields(
	prefix string,
	listResponse *v1.ListApiKeysResponse,
	data *ApiKeyResourceModel,
) (isFound bool) {
	var apiKey *v1.ApiKey
	for _, found := range listResponse.GetApiKeys() {
		if prefix == found.GetPrefix() {
			apiKey = found
			break
		}
	}
	if apiKey == nil {
		return false
	}

	data.Expiration = types.StringValue(apiKey.GetExpiration().AsTime().Format(time.RFC3339))
	data.CreatedAt = types.StringValue(apiKey.GetCreatedAt().AsTime().Format(time.RFC3339))
	data.Expired = types.BoolValue(time.Now().After(apiKey.Expiration.AsTime()))
	return true
}

func (r *ApiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ApiKeyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	ttl, err := time.ParseDuration(data.Ttl.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Parse TTL Error", fmt.Sprintf("Unable to parse ttl, got error: %s", err))
		return
	}
	response, err := r.client.CreateApiKey(ctx, &v1.CreateApiKeyRequest{
		Expiration: timestamppb.New(time.Now().Add(ttl)),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create api key, got error: %s", err))
		return
	}
	keyPrefix := strings.Split(response.GetApiKey(), ".")[0]
	data.Key = types.StringValue(response.GetApiKey())
	data.Id = types.StringValue(keyPrefix)

	listResponse, err := r.client.ListApiKeys(ctx, &v1.ListApiKeysRequest{})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf(
			"Unable to list api keys after creation, got error: %s", err),
		)
		return
	}
	if isFound := r.readComputedFields(data.Id.ValueString(), listResponse, &data); !isFound {
		resp.Diagnostics.AddError("Client Error", "Unable to found api key after creation")
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ApiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ApiKeyResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	listResponse, err := r.client.ListApiKeys(ctx, &v1.ListApiKeysRequest{})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf(
			"Unable to list api keys, got error: %s", err),
		)
		return
	}
	if isFound := r.readComputedFields(data.Id.ValueString(), listResponse, &data); !isFound {
		resp.State.RemoveResource(ctx)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if data.Expired.ValueBool() {
		resp.State.RemoveResource(ctx)
	}
}

func (r *ApiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Error updating api key",
		"api keys cannot be updated",
	)
}

func (r *ApiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ApiKeyResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.DeleteApiKey(ctx, &v1.DeleteApiKeyRequest{
		Prefix: data.Id.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete api key, got error: %s", err))
		return
	}
}
