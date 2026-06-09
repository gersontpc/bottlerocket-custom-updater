package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	ModeController = "controller"
	ModeWorker     = "worker"
)

type Config struct {
	Mode                    string
	Namespace               string
	NodeName                string
	StateConfigMapName      string
	TargetNodeLabelSelector string

	SSMParameterName string
	AWSRegion        string

	TimeZone            string
	UpdateWindowStart   string
	UpdateWindowEnd     string
	RolloutStart        string
	MinRemainingMinutes int

	MaxConcurrentUpdates      int
	ExcludeFromLBWaitDuration time.Duration
	ControllerPollInterval    time.Duration
	WorkerPollInterval        time.Duration
	DrainTimeout              time.Duration
	PodGracePeriodSeconds     int64
	PostRebootGracePeriod     time.Duration
	RollbackOnFailure         bool

	APIClientBin string
	SignpostBin  string
}

func Load() (Config, error) {
	cfg := Config{
		Mode:                    getenv("RUN_MODE", ModeController),
		Namespace:               getenv("POD_NAMESPACE", "default"),
		NodeName:                getenv("MY_NODE_NAME", ""),
		StateConfigMapName:      getenv("STATE_CONFIGMAP_NAME", "bottlerocket-updater-state"),
		TargetNodeLabelSelector: getenv("TARGET_NODE_LABEL_SELECTOR", ""),
		SSMParameterName:        getenv("SSM_PARAMETER_NAME", ""),
		AWSRegion:               getenv("AWS_REGION", ""),
		TimeZone:                getenv("TIME_ZONE", "UTC"),
		UpdateWindowStart:       getenv("UPDATE_WINDOW_START", "01:00"),
		UpdateWindowEnd:         getenv("UPDATE_WINDOW_END", "05:00"),
		RolloutStart:            getenv("ROLLOUT_START", ""),
		MinRemainingMinutes:     getenvInt("MIN_REMAINING_MINUTES", 30),
		MaxConcurrentUpdates:    getenvInt("MAX_CONCURRENT_UPDATES", 1),
		ControllerPollInterval:  getenvDuration("CONTROLLER_POLL_INTERVAL", 30*time.Second),
		WorkerPollInterval:      getenvDuration("WORKER_POLL_INTERVAL", 30*time.Second),
		DrainTimeout:            getenvDuration("DRAIN_TIMEOUT", 20*time.Minute),
		PodGracePeriodSeconds:   int64(getenvInt("POD_GRACE_PERIOD_SECONDS", 60)),
		PostRebootGracePeriod:   getenvDuration("POST_REBOOT_GRACE_PERIOD", 20*time.Minute),
		RollbackOnFailure:       getenvBool("ROLLBACK_ON_FAILURE", true),
		APIClientBin:            getenv("APICLIENT_BIN", "apiclient"),
		SignpostBin:             getenv("SIGNPOST_BIN", "signpost"),
	}

	if cfg.RolloutStart == "" {
		cfg.RolloutStart = cfg.UpdateWindowStart
	}
	cfg.ExcludeFromLBWaitDuration = time.Duration(getenvInt("EXCLUDE_FROM_LB_WAIT_TIME_IN_SEC", 0)) * time.Second

	if cfg.Mode != ModeController && cfg.Mode != ModeWorker {
		return Config{}, fmt.Errorf("RUN_MODE must be %q or %q", ModeController, ModeWorker)
	}
	if cfg.Mode == ModeWorker && cfg.NodeName == "" {
		return Config{}, fmt.Errorf("MY_NODE_NAME is required in worker mode")
	}
	if cfg.Mode == ModeController && cfg.SSMParameterName == "" {
		return Config{}, fmt.Errorf("SSM_PARAMETER_NAME is required in controller mode")
	}
	if cfg.MaxConcurrentUpdates < 1 {
		return Config{}, fmt.Errorf("MAX_CONCURRENT_UPDATES must be greater than zero")
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
