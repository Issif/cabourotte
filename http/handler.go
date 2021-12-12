package http

import (
	"bytes"
	"crypto/subtle"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/mcorbin/cabourotte/healthcheck"
)

// BasicResponse a type for HTTP responses
type BasicResponse struct {
	Message string `json:"message"`
}

// addCheck adds a periodic healthcheck to the healthcheck component.
func (c *Component) addCheck(ec echo.Context, check healthcheck.Healthcheck) error {
	check.SetSource(healthcheck.SourceAPI)
	err := c.healthcheck.AddCheck(check)
	if err != nil {
		return err
	}
	return nil
}

//go:embed assets
var embededFiles embed.FS

// oneOff executes an one-off healthcheck and returns its result
func (c *Component) oneOff(ec echo.Context, check healthcheck.Healthcheck) error {
	c.Logger.Info(fmt.Sprintf("Executing one-off healthcheck %s", check.Base().Name))
	err := check.Initialize()
	if err != nil {
		msg := fmt.Sprintf("Fail to initialize one off healthcheck %s: %s", check.Base().Name, err.Error())
		c.Logger.Error(msg)
		return ec.JSON(http.StatusInternalServerError, &BasicResponse{Message: msg})
	}
	start := time.Now()
	err = check.Execute()
	duration := time.Since(start)
	result := healthcheck.NewResult(
		check,
		duration.Seconds(),
		err)
	result.Source = "one-off"
	if err != nil {
		msg := fmt.Sprintf("Execution of one off healthcheck %s failed: %s", check.Base().Name, err.Error())
		c.Logger.Error(msg)
		return ec.JSON(http.StatusCreated, result)
	}
	msg := fmt.Sprintf("One-off healthcheck %s successfully executed", check.Base().Name)
	c.Logger.Info(msg)
	return ec.JSON(http.StatusCreated, result)
}

func (c *Component) addCheckError(ec echo.Context, healthcheck healthcheck.Healthcheck, err error) error {
	msg := fmt.Sprintf("Fail to start the healthcheck %s: %s", healthcheck.Base().Name, err.Error())
	c.Logger.Error(msg)
	return ec.JSON(http.StatusInternalServerError, &BasicResponse{Message: msg})
}

// handleCheck handles new healthchecks requests
func (c *Component) handleCheck(ec echo.Context, healthcheck healthcheck.Healthcheck) error {
	if healthcheck.Base().OneOff {
		return c.oneOff(ec, healthcheck)
	}
	err := c.addCheck(ec, healthcheck)
	if err != nil {
		return c.addCheckError(ec, healthcheck, err)
	}
	return ec.JSON(http.StatusCreated, &BasicResponse{Message: "Healthcheck successfully added"})
}

// handlers configures the handlers for the http server component
func (c *Component) handlers() {
	fsys, _ := fs.Sub(embededFiles, "assets")
	c.Server.Use(c.countResponse)
	if c.Config.BasicAuth.Username != "" {
		c.Server.Use(middleware.BasicAuth(func(username, password string, ctx echo.Context) (bool, error) {
			if subtle.ConstantTimeCompare([]byte(username),
				[]byte(c.Config.BasicAuth.Username)) == 1 &&
				subtle.ConstantTimeCompare([]byte(password),
					[]byte(c.Config.BasicAuth.Password)) == 1 {
				return true, nil
			}
			c.Logger.Error("Invalid Basic Auth credentials")
			return false, nil
		}))
	}
	echo.NotFoundHandler = func(ec echo.Context) error {
		return ec.JSON(http.StatusNotFound, &BasicResponse{Message: "not found"})
	}
	var bulkLock sync.RWMutex
	if !c.Config.DisableHealthcheckAPI {
		c.Server.POST("/healthcheck/dns", func(ec echo.Context) error {
			var config healthcheck.DNSHealthcheckConfiguration
			if err := ec.Bind(&config); err != nil {
				msg := fmt.Sprintf("Fail to create the dns healthcheck. Invalid JSON: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			err := config.Validate()
			if err != nil {
				msg := fmt.Sprintf("Invalid healthcheck configuration: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			healthcheck := healthcheck.NewDNSHealthcheck(c.Logger, &config)
			return c.handleCheck(ec, healthcheck)
		})

		c.Server.POST("/healthcheck/tcp", func(ec echo.Context) error {
			var config healthcheck.TCPHealthcheckConfiguration
			if err := ec.Bind(&config); err != nil {
				msg := fmt.Sprintf("Fail to create the TCP healthcheck. Invalid JSON: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			err := config.Validate()
			if err != nil {
				msg := fmt.Sprintf("Invalid healthcheck configuration: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			healthcheck := healthcheck.NewTCPHealthcheck(c.Logger, &config)
			return c.handleCheck(ec, healthcheck)
		})

		c.Server.POST("/healthcheck/tls", func(ec echo.Context) error {
			var config healthcheck.TLSHealthcheckConfiguration
			if err := ec.Bind(&config); err != nil {
				msg := fmt.Sprintf("Fail to create the TLS healthcheck. Invalid JSON: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			err := config.Validate()
			if err != nil {
				msg := fmt.Sprintf("Invalid healthcheck configuration: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			healthcheck := healthcheck.NewTLSHealthcheck(c.Logger, &config)
			return c.handleCheck(ec, healthcheck)
		})

		c.Server.POST("/healthcheck/http", func(ec echo.Context) error {
			var config healthcheck.HTTPHealthcheckConfiguration
			if err := ec.Bind(&config); err != nil {
				msg := fmt.Sprintf("Fail to create the HTTP healthcheck. Invalid JSON: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			err := config.Validate()
			if err != nil {
				msg := fmt.Sprintf("Invalid healthcheck configuration: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			healthcheck := healthcheck.NewHTTPHealthcheck(c.Logger, &config)
			return c.handleCheck(ec, healthcheck)
		})

		c.Server.POST("/healthcheck/command", func(ec echo.Context) error {
			var config healthcheck.CommandHealthcheckConfiguration
			if err := ec.Bind(&config); err != nil {
				msg := fmt.Sprintf("Fail to create the Command healthcheck. Invalid JSON: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			err := config.Validate()
			if err != nil {
				msg := fmt.Sprintf("Invalid healthcheck configuration: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			healthcheck := healthcheck.NewCommandHealthcheck(c.Logger, &config)
			return c.handleCheck(ec, healthcheck)
		})

		c.Server.POST("/healthcheck/bulk", func(ec echo.Context) error {
			bulkLock.Lock()
			defer bulkLock.Unlock()
			var payload BulkPayload
			newChecks := make(map[string]bool)
			oldChecks := c.healthcheck.SourceChecksNames(healthcheck.SourceAPI)
			if err := ec.Bind(&payload); err != nil {
				msg := fmt.Sprintf("Fail to add healthchecks. Invalid JSON: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			err := payload.Validate()
			if err != nil {
				msg := fmt.Sprintf("Fail to validate healthchecks configuration: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusBadRequest, &BasicResponse{Message: msg})
			}
			for i := range payload.HTTPChecks {
				config := payload.HTTPChecks[i]
				healthcheck := healthcheck.NewHTTPHealthcheck(c.Logger, &config)
				err := c.addCheck(ec, healthcheck)
				if err != nil {
					return c.addCheckError(ec, healthcheck, err)
				}
				newChecks[config.Base.Name] = true
			}
			for i := range payload.TCPChecks {
				config := payload.TCPChecks[i]
				healthcheck := healthcheck.NewTCPHealthcheck(c.Logger, &config)
				err := c.addCheck(ec, healthcheck)
				if err != nil {
					return c.addCheckError(ec, healthcheck, err)
				}
				newChecks[config.Base.Name] = true
			}
			for i := range payload.DNSChecks {
				config := payload.DNSChecks[i]
				healthcheck := healthcheck.NewDNSHealthcheck(c.Logger, &config)
				err := c.addCheck(ec, healthcheck)
				if err != nil {
					return c.addCheckError(ec, healthcheck, err)
				}
				newChecks[config.Base.Name] = true
			}
			for i := range payload.TLSChecks {
				config := payload.TLSChecks[i]
				healthcheck := healthcheck.NewTLSHealthcheck(c.Logger, &config)
				err := c.addCheck(ec, healthcheck)
				if err != nil {
					return c.addCheckError(ec, healthcheck, err)
				}
				newChecks[config.Base.Name] = true
			}
			for i := range payload.CommandChecks {
				config := payload.CommandChecks[i]
				healthcheck := healthcheck.NewCommandHealthcheck(c.Logger, &config)
				err := c.addCheck(ec, healthcheck)
				if err != nil {
					return c.addCheckError(ec, healthcheck, err)
				}
				newChecks[config.Base.Name] = true
			}
			err = c.healthcheck.RemoveNonConfiguredHealthchecks(oldChecks, newChecks)
			if err != nil {
				c.Logger.Error(err.Error())
				return ec.JSON(http.StatusInternalServerError, &BasicResponse{Message: err.Error()})
			}
			return ec.JSON(http.StatusCreated, &BasicResponse{Message: "Healthchecks successfully added"})
		})

		c.Server.GET("/healthcheck", func(ec echo.Context) error {
			return ec.JSON(http.StatusOK, c.healthcheck.ListChecks())
		})
		c.Server.GET("/healthcheck/:name", func(ec echo.Context) error {
			name := ec.Param("name")
			healthcheck, err := c.healthcheck.GetCheck(name)
			if err != nil {
				return ec.JSON(http.StatusNotFound, &BasicResponse{Message: err.Error()})
			}
			return ec.JSON(http.StatusOK, healthcheck)
		})

		c.Server.DELETE("/healthcheck/:name", func(ec echo.Context) error {
			name := ec.Param("name")
			c.Logger.Info(fmt.Sprintf("Deleting healthcheck %s", name))
			err := c.healthcheck.RemoveCheck(name)
			if err != nil {
				msg := fmt.Sprintf("Fail to start the healthcheck: %s", err.Error())
				c.Logger.Error(msg)
				return ec.JSON(http.StatusInternalServerError, &BasicResponse{Message: msg})
			}
			return ec.JSON(http.StatusOK, &BasicResponse{Message: fmt.Sprintf("Successfully deleted healthcheck %s", name)})
		})
	}
	if !c.Config.DisableResultAPI {
		c.Server.GET("/result", func(ec echo.Context) error {
			return ec.JSON(http.StatusOK, c.MemoryStore.List())
		})
		c.Server.GET("/result/:name", func(ec echo.Context) error {
			name := ec.Param("name")
			result, err := c.MemoryStore.Get(name)
			if err != nil {
				return ec.JSON(http.StatusNotFound, &BasicResponse{Message: err.Error()})
			}
			return ec.JSON(http.StatusOK, result)

		})
		c.Server.GET("/frontend", func(ec echo.Context) error {
			err := ec.Redirect(http.StatusFound, "/frontend/index.html")
			return err
		})
		c.Server.GET("/frontend/*", func(ec echo.Context) error {
			path := strings.TrimPrefix(ec.Request().URL.Path, "/frontend/")

			if path == "" {
				path = "index.html"
			}

			c.Logger.Info(path)
			file, err := fsys.Open(path)
			if err != nil {
				return ec.JSON(http.StatusInternalServerError, &BasicResponse{Message: err.Error()})
			}
			stat, err := file.Stat()
			if err != nil {
				return ec.JSON(http.StatusInternalServerError, &BasicResponse{Message: err.Error()})
			}
			size := stat.Size()
			buffer := make([]byte, size)
			_, err = file.Read(buffer)
			if err != nil {
				return ec.JSON(http.StatusInternalServerError, &BasicResponse{Message: err.Error()})
			}
			if path == "index.html" {
				tmpl := template.New("frontend")
				tmpl.Funcs(template.FuncMap{
					"last": func(x int, a interface{}) bool {
						return x == reflect.ValueOf(a).Len()-1
					},
					"mod": func(i, j int) int { return i % j },
					"formatts": func(ts int64) string {
						tm := time.Unix(ts, 0)
						return tm.Format("2006/01/02 15:04:05")
					},
				})

				tmpl, err = tmpl.Parse(string(buffer))
				if err != nil {
					c.Logger.Error(err.Error())
					return ec.JSON(http.StatusInternalServerError, &BasicResponse{Message: err.Error()})
				}
				var tmplBytes bytes.Buffer
				if err := tmpl.Execute(&tmplBytes, c.MemoryStore.List()); err != nil {
					c.Logger.Error(err.Error())
					return ec.JSON(http.StatusInternalServerError, &BasicResponse{Message: err.Error()})
				}
				return ec.HTML(http.StatusOK, tmplBytes.String())

			} else {
				return ec.HTML(http.StatusOK, string(buffer))
			}

		})
	}

	c.Server.GET("/health", func(ec echo.Context) error {
		return ec.JSON(http.StatusOK, "ok")
	})

	c.Server.GET("/healthz", func(ec echo.Context) error {
		return ec.JSON(http.StatusOK, "ok")
	})

	c.Server.GET("/metrics", echo.WrapHandler(c.Prometheus.Handler()))
	if c.DiscoveryConfig.Kubernetes.Pod.Enabled || c.DiscoveryConfig.Kubernetes.Service.Enabled || c.DiscoveryConfig.Kubernetes.CRD.Enabled {
		c.Server.GET("/discovery/kubernetes/metrics", echo.WrapHandler(promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{})))
	}
}
