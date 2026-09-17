// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type proxmoxBackupServerBool bool

func (b *proxmoxBackupServerBool) UnmarshalJSON(data []byte) error {
	var boolValue bool
	if err := json.Unmarshal(data, &boolValue); err == nil {
		*b = proxmoxBackupServerBool(boolValue)
		return nil
	}

	var intValue int
	if err := json.Unmarshal(data, &intValue); err == nil {
		*b = proxmoxBackupServerBool(intValue != 0)
		return nil
	}

	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		parsed, parseErr := strconv.ParseBool(stringValue)
		if parseErr != nil {
			intValue, intErr := strconv.Atoi(stringValue)
			if intErr != nil {
				return parseErr
			}
			parsed = intValue != 0
		}
		*b = proxmoxBackupServerBool(parsed)
		return nil
	}

	return fmt.Errorf("invalid Proxmox Backup Server boolean value %s", string(data))
}

func (b proxmoxBackupServerBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(b))
}

type userAPIModel struct {
	UserID    string                   `json:"userid"`
	Enable    *proxmoxBackupServerBool `json:"enable,omitempty"`
	Comment   *string                  `json:"comment,omitempty"`
	Email     *string                  `json:"email,omitempty"`
	Firstname *string                  `json:"firstname,omitempty"`
	Lastname  *string                  `json:"lastname,omitempty"`
	Expire    *int64                   `json:"expire,omitempty"`
	Password  *string                  `json:"password,omitempty"` // Write-only: never returned by the API.
	Digest    *string                  `json:"digest,omitempty"`
	Delete    []string                 `json:"delete,omitempty"`
}

type aclAPIModel struct {
	Path      string                   `json:"path"`
	AuthID    string                   `json:"auth-id,omitempty"` // Legacy response alias.
	UGID      string                   `json:"ugid,omitempty"`
	UGIDType  string                   `json:"ugid_type,omitempty"`
	Role      string                   `json:"role,omitempty"` // Legacy response alias.
	RoleID    string                   `json:"roleid,omitempty"`
	Propagate *proxmoxBackupServerBool `json:"propagate,omitempty"`
}

type aclMutationModel struct {
	Path      string                   `json:"path"`
	AuthID    string                   `json:"auth-id,omitempty"`
	Group     string                   `json:"group,omitempty"`
	Role      string                   `json:"role,omitempty"`
	Propagate *proxmoxBackupServerBool `json:"propagate,omitempty"`
	Delete    *proxmoxBackupServerBool `json:"delete,omitempty"`
	Digest    *string                  `json:"digest,omitempty"`
}

type userTokenAPIModel struct {
	TokenName  string                   `json:"tokenid,omitempty"`
	Enable     *proxmoxBackupServerBool `json:"enable,omitempty"`
	Comment    *string                  `json:"comment,omitempty"`
	Expire     *int64                   `json:"expire,omitempty"`
	Value      string                   `json:"value,omitempty"`
	Secret     string                   `json:"secret,omitempty"`
	Regenerate *proxmoxBackupServerBool `json:"regenerate,omitempty"`
	Delete     []string                 `json:"delete,omitempty"`
	Digest     *string                  `json:"digest,omitempty"`
}

type openIDRealmAPIModel struct {
	Realm         string                   `json:"realm"`
	IssuerURL     string                   `json:"issuer-url"`
	ClientID      string                   `json:"client-id"`
	Audiences     *string                  `json:"audiences,omitempty"`
	ClientKey     *string                  `json:"client-key,omitempty"`
	Scopes        *string                  `json:"scopes,omitempty"`
	ACRValues     *string                  `json:"acr-values,omitempty"`
	Prompt        *string                  `json:"prompt,omitempty"`
	Comment       *string                  `json:"comment,omitempty"`
	AutoCreate    *proxmoxBackupServerBool `json:"autocreate,omitempty"`
	UsernameClaim *string                  `json:"username-claim,omitempty"`
	Default       *proxmoxBackupServerBool `json:"default,omitempty"`
}

func accessBoolPointerValue(value *proxmoxBackupServerBool) types.Bool {
	if value == nil {
		return types.BoolValue(true)
	}
	return types.BoolValue(bool(*value))
}

func accessBoolPointer(value types.Bool) *proxmoxBackupServerBool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := proxmoxBackupServerBool(value.ValueBool())
	return &v
}

func accessStringPointer(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := value.ValueString()
	return &v
}

func accessInt64Pointer(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := value.ValueInt64()
	return &v
}

func accessStringValue(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func accessInt64Value(value *int64) types.Int64 {
	if value == nil {
		return types.Int64Value(0)
	}
	return types.Int64Value(*value)
}

func aclEntryID(path, authID, role string) string {
	return path + "|" + authID + "|" + role
}

func aclAPIAuthID(apiData aclAPIModel) string {
	if apiData.UGID != "" {
		return apiData.UGID
	}
	return apiData.AuthID
}

func aclAPIUGIDType(apiData aclAPIModel) string {
	if apiData.UGIDType == "" {
		return "user"
	}
	return apiData.UGIDType
}

func aclAPIRole(apiData aclAPIModel) string {
	if apiData.RoleID != "" {
		return apiData.RoleID
	}
	return apiData.Role
}
