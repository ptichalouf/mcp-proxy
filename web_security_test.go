package main

import "testing"

func TestWebManagementWithoutTokensRequiresLoopback(t *testing.T) {
	for _, tc := range []struct {
		addr  string
		allow bool
	}{
		{"127.0.0.1:9090", true},
		{"[::1]:9090", true},
		{"localhost:9090", true},
		{":9090", false},
		{"0.0.0.0:9090", false},
		{"[::]:9090", false},
		{"192.0.2.10:9090", false},
		{"bad-address", false},
	} {
		err := validateWebManagementAccess(true, tc.addr, nil)
		if (err == nil) != tc.allow {
			t.Errorf("addr=%q allowed=%v want %v", tc.addr, err == nil, tc.allow)
		}
	}
	if err := validateWebManagementAccess(false, ":9090", nil); err != nil {
		t.Fatalf("web disabled must not change existing proxy listener: %v", err)
	}
	if err := validateWebManagementAccess(true, ":9090", []string{"secret"}); err != nil {
		t.Fatalf("authenticated remote web listener should be allowed: %v", err)
	}
}
