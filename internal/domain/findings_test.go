package domain

import "testing"

func TestFindingAllowed(t *testing.T) {
	tests := []struct {
		status     string
		action     FindingAction
		staff, con bool
		want       bool
	}{
		{"open", FindingRespond, false, true, true},
		{"in_progress", FindingRespond, false, true, true},
		{"open", FindingRespond, true, false, false}, // staff close instead of responding
		{"responded", FindingRespond, false, true, false},
		{"responded", FindingVerify, true, false, true},
		{"responded", FindingVerify, false, true, false}, // contractors can't verify themselves
		{"open", FindingVerify, true, false, false},
		{"responded", FindingReject, true, false, true},
		{"responded", FindingClose, true, false, true},
		{"closed", FindingClose, true, false, false},
		{"closed", FindingReopen, true, false, true},
		{"closed", FindingEdit, true, false, false},
		{"open", FindingEdit, false, true, false},
	}
	for _, tt := range tests {
		if got := FindingAllowed(tt.status, tt.action, tt.staff, tt.con); got != tt.want {
			t.Errorf("%s/%s staff=%v contractor=%v: got %v", tt.status, tt.action, tt.staff, tt.con, got)
		}
	}
}
