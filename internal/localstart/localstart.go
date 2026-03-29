package localstart

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var unitNameRe = regexp.MustCompile(`^[a-zA-Z0-9._@-]+$`)

// Control runs same-host start/stop for Tiamat (script and/or systemd unit).
type Control struct {
	startScript string
	unit        string
	userScope   bool
	stopScript  string
}

// New builds a Control from config, or returns nil if no start/stop path is configured.
func New(startScript, systemdUnit string, systemdUser bool, stopScript string) (*Control, error) {
	if startScript != "" && systemdUnit != "" {
		return nil, fmt.Errorf("localstart: start script and systemd unit are mutually exclusive")
	}
	if startScript == "" && systemdUnit == "" && stopScript == "" {
		return nil, nil
	}
	var c Control
	if startScript != "" {
		if err := validateExecutableScript(startScript, "SAURON_TIAMAT_START_SCRIPT"); err != nil {
			return nil, err
		}
		c.startScript = startScript
	}
	if systemdUnit != "" {
		if err := validateSystemdUnit(systemdUnit); err != nil {
			return nil, err
		}
		c.unit = systemdUnit
	}
	c.userScope = systemdUser
	if stopScript != "" {
		if err := validateExecutableScript(stopScript, "SAURON_TIAMAT_STOP_SCRIPT"); err != nil {
			return nil, err
		}
		c.stopScript = stopScript
	}
	return &c, nil
}

// CanStart is true when start script or systemd unit is configured.
func (c *Control) CanStart() bool {
	return c != nil && (c.startScript != "" || c.unit != "")
}

// CanStop is true when stop script is set, or start uses systemd (systemctl stop same unit).
func (c *Control) CanStop() bool {
	return c != nil && (c.stopScript != "" || c.unit != "")
}

func validateSystemdUnit(name string) error {
	if !unitNameRe.MatchString(name) {
		return fmt.Errorf("invalid SAURON_TIAMAT_SYSTEMD_UNIT %q (allowed: letters, digits, ._@-)", name)
	}
	return nil
}

func validateExecutableScript(p, envKey string) error {
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s must be an absolute path", envKey)
	}
	clean := filepath.Clean(p)
	if clean != p {
		return fmt.Errorf("%s must be a clean absolute path", envKey)
	}
	fi, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("%s: %w", envKey, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("%s: path is a directory", envKey)
	}
	if fi.Mode()&0o111 == 0 {
		return fmt.Errorf("%s: file must be executable (mode +x)", envKey)
	}
	return nil
}

func systemctlPath() string {
	p, err := exec.LookPath("systemctl")
	if err != nil {
		return "/usr/bin/systemctl"
	}
	return p
}

// Start runs systemctl start or the start script under ctx.
func (c *Control) Start(ctx context.Context) error {
	if c == nil || !c.CanStart() {
		return fmt.Errorf("start not configured")
	}
	if c.startScript != "" {
		cmd := exec.CommandContext(ctx, c.startScript)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, truncateOut(out))
		}
		return nil
	}
	args := []string{"start", c.unit}
	if c.userScope {
		args = append([]string{"--user"}, args...)
	}
	cmd := exec.CommandContext(ctx, systemctlPath(), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, truncateOut(out))
	}
	return nil
}

// Stop runs the stop script if set; otherwise systemctl stop when start used a unit.
func (c *Control) Stop(ctx context.Context) error {
	if c == nil || !c.CanStop() {
		return fmt.Errorf("stop not configured")
	}
	if c.stopScript != "" {
		cmd := exec.CommandContext(ctx, c.stopScript)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, truncateOut(out))
		}
		return nil
	}
	args := []string{"stop", c.unit}
	if c.userScope {
		args = append([]string{"--user"}, args...)
	}
	cmd := exec.CommandContext(ctx, systemctlPath(), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, truncateOut(out))
	}
	return nil
}

func truncateOut(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
