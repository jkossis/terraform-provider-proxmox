// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestS3EndpointValidator(t *testing.T) {
	tests := map[string]bool{
		"garage":             false,
		"192.0.2.10":         false,
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
