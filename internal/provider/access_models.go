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
	UserID  string                   `json:"userid"`
	Enable  *proxmoxBackupServerBool `json:"enable,omitempty"`
	Comment string                   `json:"comment,omitempty"`
}

type aclAPIModel struct {
	Path      string                   `json:"path"`
	AuthID    string                   `json:"auth-id,omitempty"`
	UGID      string                   `json:"ugid,omitempty"`
	Role      string                   `json:"role,omitempty"`
	RoleID    string                   `json:"roleid,omitempty"`
	Propagate *proxmoxBackupServerBool `json:"propagate,omitempty"`
	Delete    *proxmoxBackupServerBool `json:"delete,omitempty"`
}

type userTokenAPIModel struct {
	TokenName string                   `json:"tokenid,omitempty"`
	Enable    *proxmoxBackupServerBool `json:"enable,omitempty"`
	Comment   string                   `json:"comment,omitempty"`
	Value     string                   `json:"value,omitempty"`
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

func aclEntryID(path, authID, role string) string {
	return path + "|" + authID + "|" + role
}

func aclAPIAuthID(apiData aclAPIModel) string {
	if apiData.AuthID != "" {
		return apiData.AuthID
	}
	return apiData.UGID
}

func aclAPIRole(apiData aclAPIModel) string {
	if apiData.Role != "" {
		return apiData.Role
	}
	return apiData.RoleID
}
