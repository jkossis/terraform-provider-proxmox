// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import "testing"

func TestSetDatastoreDataSourceDataMapsAPIFields(t *testing.T) {
	comment := "Terraform managed"
	pruneSchedule := "daily"
	keepWeekly := int64(4)
	verifyNew := true
	apiData := datastoreAPIModel{
		Name:          "backup",
		Path:          "/mnt/datastore/backup",
		Comment:       &comment,
		PruneSchedule: &pruneSchedule,
		KeepWeekly:    &keepWeekly,
		VerifyNew:     &verifyNew,
	}
	var data DatastoreDataSourceModel

	setDatastoreDataSourceData(&data, apiData)
	if got, want := data.Name.ValueString(), "backup"; got != want {
		t.Fatalf("unexpected name: got %q, want %q", got, want)
	}
	if got, want := data.Path.ValueString(), "/mnt/datastore/backup"; got != want {
		t.Fatalf("unexpected path: got %q, want %q", got, want)
	}
	if got, want := data.Comment.ValueString(), "Terraform managed"; got != want {
		t.Fatalf("unexpected comment: got %q, want %q", got, want)
	}
	if got, want := data.PruneSchedule.ValueString(), "daily"; got != want {
		t.Fatalf("unexpected prune_schedule: got %q, want %q", got, want)
	}
	if got, want := data.KeepWeekly.ValueInt64(), int64(4); got != want {
		t.Fatalf("unexpected keep_weekly: got %d, want %d", got, want)
	}
	if !data.VerifyNew.ValueBool() {
		t.Fatal("expected verify_new to be true")
	}
	if !data.Notify.IsNull() {
		t.Fatalf("expected notify to be null, got %#v", data.Notify)
	}
}
