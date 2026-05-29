// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type s3EndpointValidator struct{}

func (v s3EndpointValidator) Description(ctx context.Context) string {
	return "endpoint must be a host name or IP address without a protocol scheme, path, query, fragment, or port"
}

func (v s3EndpointValidator) MarkdownDescription(ctx context.Context) string {
	return "endpoint must be a host name or IP address without a protocol scheme, path, query, fragment, or port"
}

func (v s3EndpointValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if strings.Contains(value, "://") || strings.ContainsAny(value, "/?#") || strings.Contains(value, ":") {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid S3 Endpoint",
			"S3 endpoint must be a host name or IP address only. Do not include a protocol scheme, path, query, fragment, or port; use the port attribute for non-default ports.",
		)
	}
}

type int64RangeValidator struct {
	min         int64
	max         int64
	description string
}

func (v int64RangeValidator) Description(ctx context.Context) string {
	return v.description
}

func (v int64RangeValidator) MarkdownDescription(ctx context.Context) string {
	return v.description
}

func (v int64RangeValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueInt64()
	if value < v.min || value > v.max {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Number",
			fmt.Sprintf("Value must be between %d and %d.", v.min, v.max),
		)
	}
}

type stringListAllowedValuesValidator struct {
	allowed map[string]struct{}
}

func (v stringListAllowedValuesValidator) Description(ctx context.Context) string {
	return "list values must be supported by Proxmox Backup Server"
}

func (v stringListAllowedValuesValidator) MarkdownDescription(ctx context.Context) string {
	return "list values must be supported by Proxmox Backup Server"
}

func (v stringListAllowedValuesValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	for _, element := range req.ConfigValue.Elements() {
		value, ok := element.(types.String)
		if !ok || value.IsNull() || value.IsUnknown() {
			continue
		}
		if _, ok := v.allowed[value.ValueString()]; !ok {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid List Value",
				fmt.Sprintf("Unsupported value %q. Supported values are: skip-if-none-match-header, delete-objects-via-delete-object.", value.ValueString()),
			)
		}
	}
}
