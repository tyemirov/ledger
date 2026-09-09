package controlplane

import (
	"embed"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const browserAssetPrefix = "/assets/ledger/"

//go:embed web
var browserFiles embed.FS

type BrowserConfig struct {
	Description    string
	TAuthURL       string
	GoogleClientID string
	LoginPath      string
	LogoutPath     string
	NoncePath      string
	SessionPath    string
}

func (configuration BrowserConfig) validate(publicOrigin string, authTenantID string) error {
	for _, value := range []string{
		publicOrigin,
		authTenantID,
		configuration.Description,
		configuration.TAuthURL,
		configuration.GoogleClientID,
		configuration.LoginPath,
		configuration.LogoutPath,
		configuration.NoncePath,
		configuration.SessionPath,
	} {
		if strings.TrimSpace(value) == "" {
			return errors.New("control_plane_invalid_browser_configuration")
		}
	}
	return nil
}

func (handler *Handler) workspace(response http.ResponseWriter, _ *http.Request) {
	handler.serveBrowserFile(response, "web/index.html", "text/html; charset=utf-8")
}

func (handler *Handler) workspaceAsset(response http.ResponseWriter, request *http.Request) {
	name := strings.TrimPrefix(request.URL.Path, browserAssetPrefix)
	contentType := "text/javascript; charset=utf-8"
	if name == "styles.css" {
		contentType = "text/css; charset=utf-8"
	}
	switch name {
	case "styles.css", "js/alpine-runtime.js", "js/app.js", "js/client.js", "js/constants.js", "js/contracts.js":
		handler.serveBrowserFile(response, "web/"+name, contentType)
	default:
		http.NotFound(response, request)
	}
}

func (handler *Handler) serveBrowserFile(response http.ResponseWriter, name string, contentType string) {
	content, err := browserFiles.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("embedded browser file %q is unavailable", name))
	}
	response.Header().Set("Content-Type", contentType)
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(content)
}

func (handler *Handler) browserConfiguration(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", "application/yaml")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write([]byte(renderBrowserConfiguration(handler.publicOrigin, handler.authTenantID, handler.browser)))
}

func renderBrowserConfiguration(publicOrigin string, authTenantID string, configuration BrowserConfig) string {
	var result strings.Builder
	result.WriteString("environments:\n")
	result.WriteString("  - description: ")
	result.WriteString(strconv.Quote(configuration.Description))
	result.WriteString("\n    origins:\n      - ")
	result.WriteString(strconv.Quote(publicOrigin))
	result.WriteString("\n    auth:\n      tauthUrl: ")
	result.WriteString(strconv.Quote(configuration.TAuthURL))
	result.WriteString("\n      tenantId: ")
	result.WriteString(strconv.Quote(authTenantID))
	result.WriteString("\n      logoutPath: ")
	result.WriteString(strconv.Quote(configuration.LogoutPath))
	result.WriteString("\n      sessionPath: ")
	result.WriteString(strconv.Quote(configuration.SessionPath))
	result.WriteString("\n      providers:\n        google:\n          enabled: true\n          clientId: ")
	result.WriteString(strconv.Quote(configuration.GoogleClientID))
	result.WriteString("\n          loginPath: ")
	result.WriteString(strconv.Quote(configuration.LoginPath))
	result.WriteString("\n          noncePath: ")
	result.WriteString(strconv.Quote(configuration.NoncePath))
	result.WriteString("\n        apple:\n          enabled: false")
	result.WriteString("\n        password:\n          enabled: false")
	result.WriteString("\n")
	return result.String()
}
