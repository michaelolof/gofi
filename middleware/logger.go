package middleware

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/michaelolof/gofi"
)

// LoggerConfig defines the config for the Logger middleware.
type LoggerConfig struct {
	// Format defines the logging formatting string
	// Optional. Default: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path}\n"
	Format string

	// Output is the log destination
	// Optional. Default: os.Stdout
	Output *log.Logger
}

// LoggerConfigDefault is the default config
var LoggerConfigDefault = LoggerConfig{
	Format: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path}\n",
	Output: log.New(os.Stdout, "", 0),
}

// maxLogField bounds each request-derived field written to the log.
const maxLogField = 512

// Logger creates a new middleware handler.
//
// The logged IP is gofi.ClientIP: the socket peer, or the client resolved by
// a gofi.TrustPolicy middleware registered earlier. Forwarding headers are
// never trusted on their own. When the chain returns an error the logged
// status is the one the default error handler sends (gofi.StatusForError); a
// panic is logged with status 500 and then propagated to the router's
// recovery. Request-derived fields are stripped of control characters and
// truncated.
func Logger(config ...LoggerConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := LoggerConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		if cfg.Format == "" {
			cfg.Format = LoggerConfigDefault.Format
		}
		if cfg.Output == nil {
			cfg.Output = LoggerConfigDefault.Output
		}
	}

	write := func(c gofi.Context, start time.Time, status int) {
		statusCode := strconv.Itoa(status)
		if status == 0 {
			statusCode = "200"
		}
		line := strings.NewReplacer(
			"${time}", time.Now().Format("2006/01/02 - 15:04:05"),
			"${status}", statusCode,
			"${latency}", time.Since(start).String(),
			"${ip}", sanitizeLogField(gofi.ClientIP(c)),
			"${method}", sanitizeLogField(c.Method()),
			"${path}", sanitizeLogField(c.Path()),
		).Replace(cfg.Format)
		cfg.Output.Print(line)
	}

	return func(c gofi.Context) (err error) {
		start := time.Now()
		defer func() {
			if r := recover(); r != nil {
				write(c, start, 500)
				panic(r)
			}
		}()

		// Pass down the chain
		err = c.Next()

		status := c.Request().Context().Response.StatusCode()
		if err != nil {
			status = gofi.StatusForError(err)
		}
		write(c, start, status)
		return err
	}
}

func sanitizeLogField(s string) string {
	if len(s) > maxLogField {
		s = s[:maxLogField] + "..."
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			buf := make([]byte, len(s))
			copy(buf, s)
			for j := i; j < len(buf); j++ {
				if buf[j] < 0x20 || buf[j] == 0x7f {
					buf[j] = ' '
				}
			}
			return string(buf)
		}
	}

	return s
}
