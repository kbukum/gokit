package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	scalar "github.com/MarceloPetrucio/go-scalar-api-reference"

	"github.com/kbukum/gokit/logger"
)

// APIDoc describes a single OpenAPI specification to serve via Scalar.
type APIDoc struct {
	// Title shown in the browser tab (defaults to "API Reference").
	Title string
	// SpecPath is the route where the raw spec is served (e.g. "/api-spec.json").
	SpecPath string
	// Spec is the raw OpenAPI content (JSON or YAML).
	Spec []byte
	// ContentType for the spec response (default: "application/json").
	ContentType string
	// UIPath is the route for the interactive docs page (e.g. "/docs").
	UIPath string
	// Host overrides the "host" field in the spec at startup (e.g. "api.example.com:8080").
	Host string
	// BasePath overrides the "basePath" field in the spec at startup (e.g. "/api/v1").
	BasePath string
	// DarkMode enables dark theme (default: true).
	DarkMode *bool
	// HideAI hides Scalar's built-in AI assistant button.
	HideAI bool
	// CustomCSS allows injecting additional CSS into the docs page.
	CustomCSS string
	// Theme overrides the Scalar theme (e.g. "default", "moon", "purple", "deepSpace").
	Theme string
}

// MountDocs registers interactive API documentation routes powered by Scalar.
// Each APIDoc produces two endpoints: a raw spec and a rendered reference page.
// If Host or BasePath are set, the embedded spec is patched at startup so
// documentation always reflects the running environment.
//
// Example:
//
//	server.MountDocs(engine, server.APIDoc{
//	    Title:    "My Service API",
//	    SpecPath: "/api-spec.json",
//	    Spec:     specJSON,
//	    UIPath:   "/docs",
//	    Host:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
//	    BasePath: "/api/v1",
//	    HideAI:   true,
//	})
func MountDocs(engine *gin.Engine, docs ...APIDoc) {
	for i := range docs {
		d := &docs[i]

		if d.Title == "" {
			d.Title = "API Reference"
		}
		if d.ContentType == "" {
			d.ContentType = "application/json"
		}

		darkMode := true
		if d.DarkMode != nil {
			darkMode = *d.DarkMode
		}

		var css string
		if d.HideAI {
			css = `button.ai-widget-trigger, .scalar-ai { display: none !important; }`
		}
		if d.CustomCSS != "" {
			css = css + "\n" + d.CustomCSS
		}

		spec := patchSpec(d.Spec, d.Host, d.BasePath, d.ContentType)

		engine.GET(d.SpecPath, func(c *gin.Context) {
			c.Data(http.StatusOK, d.ContentType, spec)
		})

		opts := &scalar.Options{
			SpecContent: string(spec),
			DarkMode:    darkMode,
			CustomOptions: scalar.CustomOptions{
				PageTitle: d.Title,
			},
		}
		if css != "" {
			opts.CustomCss = css
		}
		if d.Theme != "" {
			opts.Theme = scalar.ThemeId(d.Theme)
		}

		htmlContent, err := scalar.ApiReferenceHTML(opts)
		if err != nil {
			logger.Error("failed to render API docs", map[string]interface{}{
				"path":  d.UIPath,
				"error": err.Error(),
			})
			continue
		}

		html := []byte(htmlContent)
		engine.GET(d.UIPath, func(c *gin.Context) {
			c.Data(http.StatusOK, "text/html; charset=utf-8", html)
		})

		logger.Debug("API docs mounted", map[string]interface{}{
			"ui": d.UIPath, "spec": d.SpecPath,
		})
	}
}

// patchSpec overrides host and basePath in an OpenAPI spec if values are provided.
func patchSpec(spec []byte, host, basePath, contentType string) []byte {
	if host == "" && basePath == "" {
		return spec
	}

	if strings.Contains(contentType, "json") {
		return patchJSON(spec, host, basePath)
	}
	return patchYAML(spec, host, basePath)
}

func patchJSON(spec []byte, host, basePath string) []byte {
	var doc map[string]interface{}
	if err := json.Unmarshal(spec, &doc); err != nil {
		return spec
	}
	if host != "" {
		doc["host"] = host
	}
	if basePath != "" {
		doc["basePath"] = basePath
	}
	patched, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		return spec
	}
	return patched
}

func patchYAML(spec []byte, host, basePath string) []byte {
	lines := strings.Split(string(spec), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case host != "" && strings.HasPrefix(trimmed, "host:"):
			result = append(result, "host: "+host)
		case basePath != "" && strings.HasPrefix(trimmed, "basePath:"):
			result = append(result, "basePath: "+basePath)
		default:
			result = append(result, line)
		}
	}
	return []byte(strings.Join(result, "\n"))
}
