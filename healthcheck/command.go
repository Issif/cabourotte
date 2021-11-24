package healthcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// CommandHealthcheckConfiguration defines a COMMAND healthcheck configuration
type CommandHealthcheckConfiguration struct {
	Base      `json:",inline" yaml:",inline"`
	Command   string   `json:"command"`
	Arguments []string `json:"arguments"`
	Timeout   Duration `json:"timeout"`
}

// CommandHealthcheck defines an HTTP healthcheck
type CommandHealthcheck struct {
	Logger *zap.Logger
	Config *CommandHealthcheckConfiguration
	URL    string

	Tick *time.Ticker
}

// Validate validates the healthcheck configuration
func (config *CommandHealthcheckConfiguration) Validate() error {
	if config.Base.Name == "" {
		return errors.New("The healthcheck name is missing")
	}
	if config.Command == "" {
		return errors.New("The healthcheck command is missing")
	}
	if config.Timeout == 0 {
		return errors.New("The healthcheck timeout is missing")
	}
	if !config.Base.OneOff {
		if config.Base.Interval < Duration(2*time.Second) {
			return errors.New("The healthcheck interval should be greater than 2 second")
		}
		if config.Base.Interval < config.Timeout {
			return errors.New("The healthcheck interval should be greater than the timeout")
		}
	}
	return nil
}

// Initialize the healthcheck.
func (h *CommandHealthcheck) Initialize() error {
	return nil
}

// GetConfig get the config
func (h *CommandHealthcheck) GetConfig() interface{} {
	return h.Config
}

// Base get the base configuration
func (h *CommandHealthcheck) Base() Base {
	return h.Config.Base
}

// Summary returns an healthcheck summary
func (h *CommandHealthcheck) Summary() string {
	summary := ""
	if h.Config.Base.Description != "" {
		summary = fmt.Sprintf("%s, command %s", h.Config.Base.Description, h.Config.Command)

	} else {
		summary = fmt.Sprintf("command %s", h.Config.Command)
	}

	return summary
}

// LogError logs an error with context
func (h *CommandHealthcheck) LogError(err error, message string) {
	h.Logger.Error(err.Error(),
		zap.String("extra", message),
		zap.String("command", h.Config.Command),
		zap.String("name", h.Config.Base.Name))
}

// LogDebug logs a message with context
func (h *CommandHealthcheck) LogDebug(message string) {
	h.Logger.Debug(message,
		zap.String("command", h.Config.Command),
		zap.String("name", h.Config.Base.Name))
}

// LogInfo logs a message with context
func (h *CommandHealthcheck) LogInfo(message string) {
	h.Logger.Info(message,
		zap.String("command", h.Config.Command),
		zap.String("name", h.Config.Base.Name))
}

// Execute executes an healthcheck on the given domain
func (h *CommandHealthcheck) Execute() error {
	h.LogDebug("start executing healthcheck")
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(h.Config.Timeout)*time.Second)
	defer cancel()
	var stdErr bytes.Buffer
	cmd := exec.CommandContext(ctx, h.Config.Command, h.Config.Arguments...)
	cmd.Stderr = &stdErr
	if err := cmd.Run(); err != nil {
		var errorMsg string
		exitErr, isExitError := err.(*exec.ExitError)
		if isExitError {
			errorMsg = fmt.Sprintf("The command failed with code=%d, stderr=%s", exitErr.ExitCode(), stdErr.String())
		} else {
			errorMsg = fmt.Sprintf("The command failed, stderr=%s", stdErr.String())
		}
		return errors.Wrapf(err, errorMsg)
	}

	return nil
}

// NewCommandHealthcheck creates a Command healthcheck from a logger and a configuration
func NewCommandHealthcheck(logger *zap.Logger, config *CommandHealthcheckConfiguration) *CommandHealthcheck {
	return &CommandHealthcheck{
		Logger: logger,
		Config: config,
	}
}

// MarshalJSON marshal to json a command healthcheck
func (h *CommandHealthcheck) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.Config)
}
