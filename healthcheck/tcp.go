package healthcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"gopkg.in/tomb.v2"
)

// TCPHealthcheckConfiguration defines a TCP healthcheck configuration
type TCPHealthcheckConfiguration struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// can be an IP or a domain
	Target   string   `json:"target"`
	Port     uint     `json:"port"`
	Timeout  Duration `json:"timeout"`
	Interval Duration `json:"interval"`
	OneOff   bool     `json:"one-off"`
}

// GetName returns the name configured in the configuration
func (c *TCPHealthcheckConfiguration) GetName() string {
	return c.Name
}

// ValidateTCPConfig validates the healthcheck configuration
func ValidateTCPConfig(config *TCPHealthcheckConfiguration) error {
	if config.Name == "" {
		return errors.New("The healthcheck name is missing")
	}
	if config.Target == "" {
		return errors.New("The healthcheck target is missing")
	}
	if config.Port == 0 {
		return errors.New("The healthcheck port is missing")
	}
	if config.Timeout == 0 {
		return errors.New("The healthcheck timeout is missing")
	}
	if config.Interval < Duration(2*time.Second) {
		return errors.New("The healthcheck interval should be greater than 2 second")
	}
	if config.Interval < config.Timeout {
		return errors.New("The healthcheck interval should be greater than the timeout")
	}
	return nil
}

// TCPHealthcheck defines a TCP healthcheck
type TCPHealthcheck struct {
	Logger *zap.Logger
	Config *TCPHealthcheckConfiguration
	URL    string

	Tick *time.Ticker
	t    tomb.Tomb
}

// buildURL build the target URL for the TCP healthcheck, depending of its
// configuration
func (h *TCPHealthcheck) buildURL() {
	h.URL = net.JoinHostPort(h.Config.Target, fmt.Sprintf("%d", h.Config.Port))
}

// Name returns the healthcheck identifier.
func (h *TCPHealthcheck) Name() string {
	return h.Config.Name
}

// Initialize the healthcheck.
func (h *TCPHealthcheck) Initialize() error {
	h.buildURL()
	return nil
}

// Interval Get the interval.
func (h *TCPHealthcheck) Interval() Duration {
	return h.Config.Interval
}

// GetConfig get the config
func (h *TCPHealthcheck) GetConfig() interface{} {
	return h.Config
}

// OneOff returns true if the healthcheck if a one-off check
func (h *TCPHealthcheck) OneOff() bool {
	return h.Config.OneOff

}

// LogError logs an error with context
func (h *TCPHealthcheck) LogError(err error, message string) {
	h.Logger.Error(err.Error(),
		zap.String("extra", message),
		zap.String("target", h.Config.Target),
		zap.Uint("port", h.Config.Port),
		zap.String("name", h.Config.Name))
}

// LogDebug logs a message with context
func (h *TCPHealthcheck) LogDebug(message string) {
	h.Logger.Debug(message,
		zap.String("target", h.Config.Target),
		zap.Uint("port", h.Config.Port),
		zap.String("name", h.Config.Name))
}

// LogInfo logs a message with context
func (h *TCPHealthcheck) LogInfo(message string) {
	h.Logger.Info(message,
		zap.String("target", h.Config.Target),
		zap.Uint("port", h.Config.Port),
		zap.String("name", h.Config.Name))
}

// Execute executes an healthcheck on the given target
func (h *TCPHealthcheck) Execute() error {
	h.LogDebug("start executing healthcheck")
	ctx := h.t.Context(nil)
	dialer := net.Dialer{}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(h.Config.Timeout))
	defer cancel()
	conn, err := dialer.DialContext(timeoutCtx, "tcp", h.URL)
	if err != nil {
		return errors.Wrapf(err, "TCP connection failed on %s", h.URL)
	}
	err = conn.Close()
	if err != nil {
		return errors.Wrapf(err, "Unable to close TCP connection")
	}
	return nil
}

// NewTCPHealthcheck creates a TCP healthcheck from a logger and a configuration
func NewTCPHealthcheck(logger *zap.Logger, config *TCPHealthcheckConfiguration) *TCPHealthcheck {
	return &TCPHealthcheck{
		Logger: logger,
		Config: config,
	}
}

// MarshalJSON marshal to json a dns healthcheck
func (h *TCPHealthcheck) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.Config)
}
