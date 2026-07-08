package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

type ControlPlane struct {
	echo   *echo.Echo
	logger *zap.Logger
}

type VersionInfo struct {
	Version     string    `json:"version"`
	ReleaseDate time.Time `json:"release_date"`
	Assets      []Asset   `json:"assets"`
	Changelog   string    `json:"changelog"`
}

type Asset struct {
	Name               string `json:"name"`
	OS                 string `json:"os"`
	Arch               string `json:"arch"`
	DownloadURL        string `json:"download_url"`
	Size               int64  `json:"size"`
	SHA256Checksum     string `json:"sha256_checksum"`
	SignatureURL       string `json:"signature_url"`
}

func NewControlPlane() (*ControlPlane, error) {
	logger, _ := zap.NewProduction()

	cp := &ControlPlane{
		echo:   echo.New(),
		logger: logger,
	}

	cp.setupRoutes()
	cp.setupMiddleware()

	return cp, nil
}

func (cp *ControlPlane) setupMiddleware() {
	cp.echo.Use(middleware.Logger())
	cp.echo.Use(middleware.Recover())
	cp.echo.Use(middleware.CORS())
}

func (cp *ControlPlane) setupRoutes() {
	cp.echo.GET("/health", cp.healthHandler)
	cp.echo.GET("/version/latest", cp.getLatestVersion)
	cp.echo.GET("/version/:platform/:arch", cp.getVersion)
	cp.echo.GET("/version", cp.getCurrentVersion)
}

func (cp *ControlPlane) healthHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"healthy": true,
		"version": "0.1.0",
	})
}

func (cp *ControlPlane) getLatestVersion(c echo.Context) error {
	repo := os.Getenv("ADM_GITHUB_REPO")
	if repo == "" {
		repo = "agentic-defense-matrix"
	}

	// In production, fetch from GitHub API
	// For now, return mock data
	version := VersionInfo{
		Version:     "0.1.0",
		ReleaseDate: time.Now(),
		Changelog:   "Initial release",
		Assets: []Asset{
			{
				Name:            "adm-gateway-darwin-arm64",
				OS:              "darwin",
				Arch:            "arm64",
				DownloadURL:     fmt.Sprintf("https://github.com/%s/releases/download/v0.1.0/adm-gateway-darwin-arm64", repo),
				Size:            15000000,
				SHA256Checksum:  "abc123...",
				SignatureURL:    fmt.Sprintf("https://github.com/%s/releases/download/v0.1.0/adm-gateway-darwin-arm64.sig", repo),
			},
			{
				Name:            "adm-gateway-linux-amd64",
				OS:              "linux",
				Arch:            "amd64",
				DownloadURL:     fmt.Sprintf("https://github.com/%s/releases/download/v0.1.0/adm-gateway-linux-amd64", repo),
				Size:            14000000,
				SHA256Checksum:  "def456...",
				SignatureURL:    fmt.Sprintf("https://github.com/%s/releases/download/v0.1.0/adm-gateway-linux-amd64.sig", repo),
			},
			{
				Name:            "adm-gateway-windows-amd64.exe",
				OS:              "windows",
				Arch:            "amd64",
				DownloadURL:     fmt.Sprintf("https://github.com/%s/releases/download/v0.1.0/adm-gateway-windows-amd64.exe", repo),
				Size:            16000000,
				SHA256Checksum:  "ghi789...",
				SignatureURL:    fmt.Sprintf("https://github.com/%s/releases/download/v0.1.0/adm-gateway-windows-amd64.exe.sig", repo),
			},
		},
	}

	return c.JSON(http.StatusOK, version)
}

func (cp *ControlPlane) getVersion(c echo.Context) error {
	platform := c.Param("platform")
	arch := c.Param("arch")

	repo := os.Getenv("ADM_GITHUB_REPO")
	if repo == "" {
		repo = "agentic-defense-matrix"
	}

	assetName := fmt.Sprintf("adm-gateway-%s-%s", platform, arch)
	if platform == "windows" {
		assetName += ".exe"
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"version":      "0.1.0",
		"platform":     platform,
		"arch":         arch,
		"download_url": fmt.Sprintf("https://github.com/%s/releases/download/v0.1.0/%s", repo, assetName),
	})
}

func (cp *ControlPlane) getCurrentVersion(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"version": "0.1.0",
	})
}

func (cp *ControlPlane) Start(addr string) error {
	cp.logger.Info("Starting Control Plane", zap.String("addr", addr))

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		cp.echo.Close()
	}()

	return cp.echo.Start(addr)
}

func main() {
	cp, err := NewControlPlane()
	if err != nil {
		panic(err)
	}

	port := os.Getenv("ADM_PORT")
	if port == "" {
		port = "9092"
	}

	if err := cp.Start(":" + port); err != nil {
		cp.logger.Fatal("control plane start failed", zap.Error(err))
	}
}
