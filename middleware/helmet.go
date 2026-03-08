package middleware

import (
	"strconv"

	"github.com/michaelolof/gofi"
)

// HelmetConfig defines the config for the Helmet middleware.
type HelmetConfig struct {
	// XSSProtection enables the X-XSS-Protection header.
	// Optional. Default: "0"
	XSSProtection string

	// ContentTypeNosniff enables the X-Content-Type-Options header.
	// Optional. Default: "nosniff"
	ContentTypeNosniff string

	// XFrameOptions enables the X-Frame-Options header.
	// Optional. Default: "SAMEORIGIN"
	XFrameOptions string

	// HSTSMaxAge enables the Strict-Transport-Security header.
	// Optional. Default: 0
	HSTSMaxAge int

	// HSTSExcludeSubdomains excludes subdomains from HSTS.
	// Optional. Default: false
	HSTSExcludeSubdomains bool

	// HSTSPreload enables the HSTS preload flag.
	// Optional. Default: false
	HSTSPreload bool

	// ContentSecurityPolicy sets the Content-Security-Policy header.
	// Optional. Default: ""
	ContentSecurityPolicy string

	// CSPReportOnly sets the Content-Security-Policy-Report-Only header instead.
	// Optional. Default: false
	CSPReportOnly bool

	// ReferrerPolicy sets the Referrer-Policy header.
	// Optional. Default: "no-referrer"
	ReferrerPolicy string

	// CrossOriginEmbedderPolicy sets the Cross-Origin-Embedder-Policy header.
	// Optional. Default: "require-corp"
	CrossOriginEmbedderPolicy string

	// CrossOriginOpenerPolicy sets the Cross-Origin-Opener-Policy header.
	// Optional. Default: "same-origin"
	CrossOriginOpenerPolicy string

	// CrossOriginResourcePolicy sets the Cross-Origin-Resource-Policy header.
	// Optional. Default: "same-origin"
	CrossOriginResourcePolicy string

	// OriginAgentCluster sets the Origin-Agent-Cluster header.
	// Optional. Default: "?1"
	OriginAgentCluster string

	// XDNSPrefetchControl sets the X-DNS-Prefetch-Control header.
	// Optional. Default: "off"
	XDNSPrefetchControl string

	// XDownloadOptions sets the X-Download-Options header.
	// Optional. Default: "noopen"
	XDownloadOptions string

	// XPermittedCrossDomainPolicies sets the X-Permitted-Cross-Domain-Policies header.
	// Optional. Default: "none"
	XPermittedCrossDomainPolicies string
}

// HelmetConfigDefault is the default config
var HelmetConfigDefault = HelmetConfig{
	XSSProtection:                 "0",
	ContentTypeNosniff:            "nosniff",
	XFrameOptions:                 "SAMEORIGIN",
	HSTSMaxAge:                    0,
	HSTSExcludeSubdomains:         false,
	HSTSPreload:                   false,
	ContentSecurityPolicy:         "",
	CSPReportOnly:                 false,
	ReferrerPolicy:                "no-referrer",
	CrossOriginEmbedderPolicy:     "require-corp",
	CrossOriginOpenerPolicy:       "same-origin",
	CrossOriginResourcePolicy:     "same-origin",
	OriginAgentCluster:            "?1",
	XDNSPrefetchControl:           "off",
	XDownloadOptions:              "noopen",
	XPermittedCrossDomainPolicies: "none",
}

// Helmet creates a new middleware handler
func Helmet(config ...HelmetConfig) gofi.MiddlewareFunc {
	// Set default config
	cfg := HelmetConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		// Only override empty strings with defaults for essential props
		if cfg.XSSProtection == "" {
			cfg.XSSProtection = HelmetConfigDefault.XSSProtection
		}
		if cfg.ContentTypeNosniff == "" {
			cfg.ContentTypeNosniff = HelmetConfigDefault.ContentTypeNosniff
		}
		if cfg.XFrameOptions == "" {
			cfg.XFrameOptions = HelmetConfigDefault.XFrameOptions
		}
		if cfg.ReferrerPolicy == "" {
			cfg.ReferrerPolicy = HelmetConfigDefault.ReferrerPolicy
		}
		if cfg.CrossOriginEmbedderPolicy == "" {
			cfg.CrossOriginEmbedderPolicy = HelmetConfigDefault.CrossOriginEmbedderPolicy
		}
		if cfg.CrossOriginOpenerPolicy == "" {
			cfg.CrossOriginOpenerPolicy = HelmetConfigDefault.CrossOriginOpenerPolicy
		}
		if cfg.CrossOriginResourcePolicy == "" {
			cfg.CrossOriginResourcePolicy = HelmetConfigDefault.CrossOriginResourcePolicy
		}
		if cfg.OriginAgentCluster == "" {
			cfg.OriginAgentCluster = HelmetConfigDefault.OriginAgentCluster
		}
		if cfg.XDNSPrefetchControl == "" {
			cfg.XDNSPrefetchControl = HelmetConfigDefault.XDNSPrefetchControl
		}
		if cfg.XDownloadOptions == "" {
			cfg.XDownloadOptions = HelmetConfigDefault.XDownloadOptions
		}
		if cfg.XPermittedCrossDomainPolicies == "" {
			cfg.XPermittedCrossDomainPolicies = HelmetConfigDefault.XPermittedCrossDomainPolicies
		}
	}

	// Prepare HSTS string
	var hsts string
	if cfg.HSTSMaxAge > 0 {
		hsts = "max-age=" + strconv.Itoa(cfg.HSTSMaxAge) // Fast conversion via internal utils
		if !cfg.HSTSExcludeSubdomains {
			hsts += "; includeSubDomains"
		}
		if cfg.HSTSPreload {
			hsts += "; preload"
		}
	}

	return func(c gofi.Context) error {
		h := c.Writer().Header()

		// Setting security headers
		if cfg.XSSProtection != "" {
			h.Set("X-XSS-Protection", cfg.XSSProtection)
		}
		if cfg.ContentTypeNosniff != "" {
			h.Set("X-Content-Type-Options", cfg.ContentTypeNosniff)
		}
		if cfg.XFrameOptions != "" {
			h.Set("X-Frame-Options", cfg.XFrameOptions)
		}
		if cfg.ReferrerPolicy != "" {
			h.Set("Referrer-Policy", cfg.ReferrerPolicy)
		}
		if cfg.CrossOriginEmbedderPolicy != "" {
			h.Set("Cross-Origin-Embedder-Policy", cfg.CrossOriginEmbedderPolicy)
		}
		if cfg.CrossOriginOpenerPolicy != "" {
			h.Set("Cross-Origin-Opener-Policy", cfg.CrossOriginOpenerPolicy)
		}
		if cfg.CrossOriginResourcePolicy != "" {
			h.Set("Cross-Origin-Resource-Policy", cfg.CrossOriginResourcePolicy)
		}
		if cfg.OriginAgentCluster != "" {
			h.Set("Origin-Agent-Cluster", cfg.OriginAgentCluster)
		}
		if cfg.XDNSPrefetchControl != "" {
			h.Set("X-DNS-Prefetch-Control", cfg.XDNSPrefetchControl)
		}
		if cfg.XDownloadOptions != "" {
			h.Set("X-Download-Options", cfg.XDownloadOptions)
		}
		if cfg.XPermittedCrossDomainPolicies != "" {
			h.Set("X-Permitted-Cross-Domain-Policies", cfg.XPermittedCrossDomainPolicies)
		}
		if hsts != "" {
			h.Set("Strict-Transport-Security", hsts)
		}

		// Content Security Policy
		if cfg.ContentSecurityPolicy != "" {
			if cfg.CSPReportOnly {
				h.Set("Content-Security-Policy-Report-Only", cfg.ContentSecurityPolicy)
			} else {
				h.Set("Content-Security-Policy", cfg.ContentSecurityPolicy)
			}
		}

		return c.Next()
	}
}
