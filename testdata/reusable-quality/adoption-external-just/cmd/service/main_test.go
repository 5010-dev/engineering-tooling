package main

import "testing"

func TestServiceName(t *testing.T) {
	t.Parallel()
	if got := serviceName(); got != "workflow-owned-just-adoption" {
		t.Fatalf("serviceName() = %q", got)
	}
}
