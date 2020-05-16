package healthcheck

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"net"

	"gopkg.in/tomb.v2"
)

// DNSHealthcheckConfiguration defines a DNS healthcheck configuration
type DNSHealthcheckConfiguration struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Domain      string   `json:"domain"`
	Interval    Duration `json:"interval"`
	OneOff      bool     `json:"one-off"`
}

// DNSHealthcheck defines an HTTP healthcheck
type DNSHealthcheck struct {
	Logger     *zap.Logger
	ChanResult chan *Result
	Config     *DNSHealthcheckConfiguration
	URL        string

	Tick *time.Ticker
	t    tomb.Tomb
}

// ValidateDNSConfig validates the healthcheck configuration
func ValidateDNSConfig(config *DNSHealthcheckConfiguration) error {
	if config.Name == "" {
		return errors.New("The healthcheck name is missing")
	}
	if config.Domain == "" {
		return errors.New("The healthcheck domain is missing")
	}
	if config.Interval < 5 {
		return errors.New("The healthcheck interval should be greater than 5")
	}
	return nil
}

// Initialize the healthcheck.
func (h *DNSHealthcheck) Initialize() error {
	return nil
}

// Name returns the healthcheck identifier.
func (h *DNSHealthcheck) Name() string {
	return h.Config.Name
}

// OneOff returns true if the healthcheck if a one-off check
func (h *DNSHealthcheck) OneOff() bool {
	return h.Config.OneOff

}

// Start an Healthcheck, which will be periodically executed after a
// given interval of time
func (h *DNSHealthcheck) Start(chanResult chan *Result) error {
	h.LogInfo("Starting healthcheck")
	h.ChanResult = chanResult
	h.Tick = time.NewTicker(time.Duration(h.Config.Interval))
	h.t.Go(func() error {
		for {
			select {
			case <-h.Tick.C:
				err := h.Execute()
				result := NewResult(h, err)
				h.ChanResult <- result
			case <-h.t.Dying():
				return nil
			}
		}
	})
	return nil
}

// LogError logs an error with context
func (h *DNSHealthcheck) LogError(err error, message string) {
	h.Logger.Error(err.Error(),
		zap.String("extra", message),
		zap.String("domain", h.Config.Domain),
		zap.String("name", h.Config.Name))
}

// LogDebug logs a message with context
func (h *DNSHealthcheck) LogDebug(message string) {
	h.Logger.Debug(message,
		zap.String("domain", h.Config.Domain),
		zap.String("name", h.Config.Name))
}

// LogInfo logs a message with context
func (h *DNSHealthcheck) LogInfo(message string) {
	h.Logger.Info(message,
		zap.String("domain", h.Config.Domain),
		zap.String("name", h.Config.Name))
}

// Stop an Healthcheck
func (h *DNSHealthcheck) Stop() error {
	h.Tick.Stop()
	h.t.Kill(nil)
	h.t.Wait()
	return nil

}

// Execute executes an healthcheck on the given domain
func (h *DNSHealthcheck) Execute() error {
	h.LogDebug("start executing healthcheck")
	_, err := net.LookupIP(h.Config.Domain)
	if err != nil {
		return errors.Wrapf(err, "Fail to lookup IP for domain")
	}
	return nil
}

// NewDNSHealthcheck creates a DNS healthcheck from a logger and a configuration
func NewDNSHealthcheck(logger *zap.Logger, config *DNSHealthcheckConfiguration) *DNSHealthcheck {
	return &DNSHealthcheck{
		Logger: logger,
		Config: config,
	}
}

// MarshalJSON marshal to json a dns healthcheck
func (h DNSHealthcheck) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.Config)
}
