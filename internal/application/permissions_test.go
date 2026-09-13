package application

import "testing"

func TestThreeRoleBoundary(t *testing.T) {
	for _, role := range []Role{RoleViewer, RoleEditor, RoleOwner} {
		for _, cap := range []Capability{CapabilityView, "manage_assets", CapabilityManageLifecycle, CapabilityManageCatalog, CapabilityManageSettings, CapabilityManageMembers, "delete_assets"} {
			want := role == RoleOwner || cap == CapabilityView || role == RoleEditor && (cap == "manage_assets" || cap == CapabilityManageLifecycle)
			if got := (Principal{Role: role}).Can(cap); got != want {
				t.Errorf("%s %s: got %v want %v", role, cap, got, want)
			}
		}
	}
}
