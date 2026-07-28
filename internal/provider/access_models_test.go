// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProxmoxBackupServerBoolUnmarshal(t *testing.T) {
	tests := map[string]bool{
		`true`:  true,
		`false`: false,
		`1`:     true,
		`0`:     false,
		`"1"`:   true,
		`"0"`:   false,
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			var got proxmoxBackupServerBool
			if err := json.Unmarshal([]byte(input), &got); err != nil {
				t.Fatalf("Unmarshal returned error: %s", err)
			}
			if bool(got) != want {
				t.Fatalf("unexpected value: got %t, want %t", got, want)
			}
		})
	}
}

func TestUserAPIModelPreservesPresentEmptyStringsAndUserPayload(t *testing.T) {
	var apiData userAPIModel
	if err := json.Unmarshal([]byte(`{"userid":"alice@pbs","comment":"","email":"","firstname":"","lastname":"","expire":0}`), &apiData); err != nil {
		t.Fatalf("json.Unmarshal returned error: %s", err)
	}
	for name, value := range map[string]*string{
		"comment":   apiData.Comment,
		"email":     apiData.Email,
		"firstname": apiData.Firstname,
		"lastname":  apiData.Lastname,
	} {
		if value == nil || *value != "" {
			t.Fatalf("API-present empty %s was not preserved: %#v", name, value)
		}
	}
	if apiData.Expire == nil || *apiData.Expire != 0 {
		t.Fatalf("unexpected API expire: %#v", apiData.Expire)
	}

	payload := userPayload(UserResourceModel{
		UserID:    types.StringValue("alice@pbs"),
		Comment:   types.StringValue(""),
		Email:     types.StringValue(""),
		Firstname: types.StringValue(""),
		Lastname:  types.StringValue(""),
		Expire:    types.Int64Value(0),
	})
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %s", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("json.Unmarshal payload returned error: %s", err)
	}
	for _, name := range []string{"comment", "email", "firstname", "lastname", "expire"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("payload omitted explicitly configured empty/default field %q: %s", name, encoded)
		}
	}
}

func TestUserDeletedFieldsUsesRepeatedDeleteValues(t *testing.T) {
	plan := UserResourceModel{
		Comment:   types.StringNull(),
		Email:     types.StringNull(),
		Firstname: types.StringNull(),
		Lastname:  types.StringNull(),
	}
	state := UserResourceModel{
		Comment:   types.StringValue("comment"),
		Email:     types.StringValue("alice@example.com"),
		Firstname: types.StringValue("Alice"),
		Lastname:  types.StringValue("Example"),
	}
	if got, want := userDeletedFields(plan, state), []string{"comment", "email", "firstname", "lastname"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deleted fields: got %#v, want %#v", got, want)
	}
}

func TestACLPayloadUsesWireTypedSubjectAndRecoveryID(t *testing.T) {
	data := ACLResourceModel{
		Path:     types.StringValue("/"),
		AuthID:   types.StringValue("backup-admins"),
		UGIDType: types.StringValue("group"),
		Role:     types.StringValue("Audit"),
	}
	payload := aclPayloadWithDigest(data, false, stringValuePointer("fresh-digest"))
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %s", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("json.Unmarshal payload returned error: %s", err)
	}
	for name, want := range map[string]string{"group": "backup-admins", "role": "Audit", "digest": "fresh-digest"} {
		if got := fields[name]; got != want {
			t.Fatalf("unexpected ACL payload %s: got %#v, want %q", name, got, want)
		}
	}
	for _, responseAlias := range []string{"ugid", "ugid_type", "ugid-type", "roleid"} {
		if _, ok := fields[responseAlias]; ok {
			t.Fatalf("ACL mutation payload unexpectedly used response alias %q", responseAlias)
		}
	}
	userPayload := aclPayloadWithDigest(ACLResourceModel{
		Path: types.StringValue("/"), AuthID: types.StringValue("alice@pbs"),
		UGIDType: types.StringValue("user"), Role: types.StringValue("Audit"),
	}, false, stringValuePointer("fresh-digest"))
	userEncoded, err := json.Marshal(userPayload)
	if err != nil {
		t.Fatalf("json.Marshal user ACL payload returned error: %s", err)
	}
	var userFields map[string]any
	if err := json.Unmarshal(userEncoded, &userFields); err != nil {
		t.Fatalf("json.Unmarshal user ACL payload returned error: %s", err)
	}
	if got, want := userFields["auth-id"], "alice@pbs"; got != want {
		t.Fatalf("unexpected user ACL auth-id: got %#v, want %q", got, want)
	}
	if _, ok := userFields["group"]; ok {
		t.Fatal("user ACL mutation payload unexpectedly used group")
	}

	recovery := aclRecoveryState(data)
	if got, want := recovery.ID.ValueString(), "/|group|backup-admins|Audit"; got != want {
		t.Fatalf("unexpected group recovery ID: got %q, want %q", got, want)
	}
	if got, want := aclEntryIDForType("/", "alice@pbs", "Audit", "user"), "/|alice@pbs|Audit"; got != want {
		t.Fatalf("unexpected backward-compatible user ID: got %q, want %q", got, want)
	}
}

func TestUserTokenExpiryRegenerationAndSecretSafety(t *testing.T) {
	data := UserTokenResourceModel{
		Comment:    types.StringNull(),
		Expire:     types.Int64Value(123),
		Regenerate: types.BoolValue(true),
	}
	payload := userTokenMutationPayload(data, stringValuePointer("fresh-digest"), true)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %s", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("json.Unmarshal payload returned error: %s", err)
	}
	if got := fields["delete"]; !reflect.DeepEqual(got, []any{"comment"}) {
		t.Fatalf("unexpected token delete values: %#v", got)
	}
	if got, want := fields["expire"], float64(123); got != want {
		t.Fatalf("unexpected token expiry: got %#v, want %v", got, want)
	}
	if got := fields["regenerate"]; got != true {
		t.Fatalf("unexpected regenerate payload: %#v", got)
	}
	if userTokenRegenerateEdge(types.BoolValue(true), types.BoolValue(true)) {
		t.Fatal("unchanged true regenerate edge unexpectedly triggered")
	}
	if !userTokenRegenerateEdge(types.BoolValue(true), types.BoolValue(false)) {
		t.Fatal("false-to-true regenerate edge did not trigger")
	}
	if userTokenRegenerateEdge(types.BoolValue(false), types.BoolValue(true)) {
		t.Fatal("true-to-false regenerate edge unexpectedly triggered")
	}
	if got, want := userTokenSecret(userTokenAPIModel{Secret: "new-secret", Value: "old-secret"}), "new-secret"; got != want {
		t.Fatalf("secret response did not prefer secret field: got %q, want %q", got, want)
	}

	apiErr := &proxmoxBackupServerAPIError{method: "PUT", path: "/api2/json/access/users/alice/token/test", status: "500 Internal Server Error", body: `{"secret":"new-secret"}`}
	diagnostic := userTokenError(apiErr, types.StringValue("old-secret"), types.StringValue("new-secret"))
	if strings.Contains(diagnostic, "old-secret") || strings.Contains(diagnostic, "new-secret") {
		t.Fatalf("token diagnostic leaked a secret: %q", diagnostic)
	}
	if strings.Contains(userTokenError(errors.New("secret=old-secret"), types.StringValue("old-secret")), "old-secret") {
		t.Fatal("generic token diagnostic leaked the old secret")
	}
}

func stringValuePointer(value string) *string {
	return &value
}
