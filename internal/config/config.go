package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds process configuration from the environment.
type Config struct {
	Listen        string
	TiamatBaseURL string
	HubToken      string

	// Optional same-host start/stop (at most one of start script or systemd unit for start).
	TiamatStartScript   string
	TiamatSystemdUnit   string
	SystemdUserScope    bool
	TiamatStopScript    string
}

func envTruthy(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	listen := strings.TrimSpace(os.Getenv("SAURON_LISTEN"))
	if listen == "" {
		listen = strings.TrimSpace(os.Getenv("PORT"))
	}
	if listen == "" {
		listen = ":8081"
	} else if !strings.Contains(listen, ":") {
		listen = ":" + listen
	}

	base := strings.TrimSpace(os.Getenv("TIAMAT_BASE_URL"))
	if base == "" {
		return Config{}, fmt.Errorf("TIAMAT_BASE_URL is required")
	}
	base = strings.TrimRight(base, "/")

	script := strings.TrimSpace(os.Getenv("SAURON_TIAMAT_START_SCRIPT"))
	unit := strings.TrimSpace(os.Getenv("SAURON_TIAMAT_SYSTEMD_UNIT"))
	if script != "" && unit != "" {
		return Config{}, fmt.Errorf("set only one of SAURON_TIAMAT_START_SCRIPT or SAURON_TIAMAT_SYSTEMD_UNIT")
	}

	return Config{
		Listen:              listen,
		TiamatBaseURL:       base,
		HubToken:            strings.TrimSpace(os.Getenv("TIAMAT_HUB_TOKEN")),
		TiamatStartScript:   script,
		TiamatSystemdUnit:   unit,
		SystemdUserScope:    envTruthy("SAURON_SYSTEMD_USER"),
		TiamatStopScript:    strings.TrimSpace(os.Getenv("SAURON_TIAMAT_STOP_SCRIPT")),
	}, nil
}
