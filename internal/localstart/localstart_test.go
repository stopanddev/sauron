package localstart

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSystemdUnit(t *testing.T) {
	if err := validateSystemdUnit("tiamat.service"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "bad;rm", "unit name", "../x"} {
		if err := validateSystemdUnit(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestNewMutuallyExclusive(t *testing.T) {
	_, err := New("/tmp/a.sh", "u.service", false, "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewScriptExecutable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := New(p, "", false, "")
	if err != nil || c == nil {
		t.Fatalf("New: %v, c=%v", err, c)
	}
	if !c.CanStart() || c.CanStop() {
		t.Fatalf("CanStart=%v CanStop=%v", c.CanStart(), c.CanStop())
	}
}

func TestCanStopWithUnitOnly(t *testing.T) {
	c, err := New("", "tiamat.service", false, "")
	if err != nil || c == nil {
		t.Fatalf("New: %v", err)
	}
	if !c.CanStart() || !c.CanStop() {
		t.Fatalf("unit-only: CanStart=%v CanStop=%v", c.CanStart(), c.CanStop())
	}
}

func TestStopScriptOverridesSystemctlStop(t *testing.T) {
	dir := t.TempDir()
	stop := filepath.Join(dir, "stop.sh")
	if err := os.WriteFile(stop, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := New("", "tiamat.service", false, stop)
	if err != nil || c == nil {
		t.Fatalf("New: %v", err)
	}
	if c.stopScript == "" {
		t.Fatal("expected stop script set")
	}
}
