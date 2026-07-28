// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestS3EndpointValidator(t *testing.T) {
	tests := map[string]bool{
		"garage":             false,
		"192.0.2.10":         false,
		"2001:db8::10":       false,
		"[2001:db8::10]":     false,
		"http://garage:3900": true,
		"garage:3900":        true,
		"garage/path":        true,
		"garage?debug=1":     true,
	}

	for value, wantError := range tests {
		t.Run(value, func(t *testing.T) {
			var resp validator.StringResponse
			s3EndpointValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("endpoint"),
				ConfigValue: types.StringValue(value),
			}, &resp)

			if gotError := resp.Diagnostics.HasError(); gotError != wantError {
				t.Fatalf("unexpected diagnostics error state: got %t, want %t", gotError, wantError)
			}
		})
	}
}

func TestInt64RangeValidator(t *testing.T) {
	validatorUnderTest := int64RangeValidator{min: 1, max: 10, description: "test range"}
	tests := map[int64]bool{
		1:  false,
		10: false,
		0:  true,
		11: true,
	}

	for value, wantError := range tests {
		t.Run(types.Int64Value(value).String(), func(t *testing.T) {
			var resp validator.Int64Response
			validatorUnderTest.ValidateInt64(context.Background(), validator.Int64Request{
				Path:        path.Root("port"),
				ConfigValue: types.Int64Value(value),
			}, &resp)

			if gotError := resp.Diagnostics.HasError(); gotError != wantError {
				t.Fatalf("unexpected diagnostics error state: got %t, want %t", gotError, wantError)
			}
		})
	}
}

func TestOpenIDCommentValidator(t *testing.T) {
	tests := map[string]bool{
		"Managed by Terraform":        false,
		"internal spaces are allowed": false,
		"":                            true,
		"   ":                         true,
		" leading":                    true,
		"trailing ":                   true,
		"\tcomment":                   true,
	}

	for value, wantError := range tests {
		t.Run(value, func(t *testing.T) {
			var resp validator.StringResponse
			openIDCommentValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("comment"),
				ConfigValue: types.StringValue(value),
			}, &resp)

			if gotError := resp.Diagnostics.HasError(); gotError != wantError {
				t.Fatalf("unexpected diagnostics error state: got %t, want %t", gotError, wantError)
			}
		})
	}

	for name, value := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			var resp validator.StringResponse
			openIDCommentValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("comment"),
				ConfigValue: value,
			}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %#v", resp.Diagnostics)
			}
		})
	}
}

func TestACLUGIDTypeValidation(t *testing.T) {
	var resourceSchemaResponse resource.SchemaResponse
	NewACLResource().Schema(context.Background(), resource.SchemaRequest{}, &resourceSchemaResponse)
	resourceAttribute, ok := resourceSchemaResponse.Schema.Attributes["ugid_type"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatal("ACL resource ugid_type is not a string attribute")
	}

	var dataSourceSchemaResponse datasource.SchemaResponse
	NewACLDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &dataSourceSchemaResponse)
	dataSourceAttribute, ok := dataSourceSchemaResponse.Schema.Attributes["ugid_type"].(datasourceschema.StringAttribute)
	if !ok {
		t.Fatal("ACL data source ugid_type is not a string attribute")
	}

	for name, validators := range map[string][]validator.String{
		"resource":    resourceAttribute.Validators,
		"data source": dataSourceAttribute.Validators,
	} {
		t.Run(name, func(t *testing.T) {
			if len(validators) == 0 {
				t.Fatal("ugid_type has no validators")
			}
			for _, value := range []string{"invalid", "token"} {
				for _, validatorUnderTest := range validators {
					var resp validator.StringResponse
					validatorUnderTest.ValidateString(context.Background(), validator.StringRequest{
						Path:        path.Root("ugid_type"),
						ConfigValue: types.StringValue(value),
					}, &resp)
					if !resp.Diagnostics.HasError() {
						t.Fatalf("invalid ACL subject type %q produced no diagnostics", value)
					}
				}
			}

			for _, value := range []string{"user", "group"} {
				for _, validatorUnderTest := range validators {
					var resp validator.StringResponse
					validatorUnderTest.ValidateString(context.Background(), validator.StringRequest{
						Path:        path.Root("ugid_type"),
						ConfigValue: types.StringValue(value),
					}, &resp)
					if resp.Diagnostics.HasError() {
						t.Fatalf("valid ACL subject type %q produced diagnostics: %#v", value, resp.Diagnostics)
					}
				}
			}
		})
	}
}

func TestStringListAllowedValuesValidator(t *testing.T) {
	validatorUnderTest := stringListAllowedValuesValidator{allowed: map[string]struct{}{
		"skip-if-none-match-header": {},
	}}
	tests := map[string]bool{
		"skip-if-none-match-header": false,
		"unsupported":               true,
	}

	for value, wantError := range tests {
		t.Run(value, func(t *testing.T) {
			var resp validator.ListResponse
			validatorUnderTest.ValidateList(context.Background(), validator.ListRequest{
				Path: path.Root("provider_quirks"),
				ConfigValue: types.ListValueMust(types.StringType, []attr.Value{
					types.StringValue(value),
				}),
			}, &resp)

			if gotError := resp.Diagnostics.HasError(); gotError != wantError {
				t.Fatalf("unexpected diagnostics error state: got %t, want %t", gotError, wantError)
			}
		})
	}
}
