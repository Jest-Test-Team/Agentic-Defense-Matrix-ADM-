package autoupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"go.uber.org/zap"
)

const (
	currentVersion = "0.1.0"
	checkInterval  = 1 * time.Hour
)

type Client struct {
	repoOwner  string
	repoName   string
	httpClient *http.Client
	logger     *zap.Logger
	enabled    bool
}

type VersionInfo struct {
	Version     string  `json:"version"`
	ReleaseDate string  `json:"release_date"`
	Assets      []Asset `json:"assets"`
	Changelog   string  `json:"changelog"`
}

type Asset struct {
	Name            string `json:"name"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	DownloadURL     string `json:"download_url"`
	Size            int64  `json:"size"`
	SHA256Checksum  string `json:"sha256_checksum"`
	SignatureURL    string `json:"signature_url"`
}

func NewClient(repoOwner, repoName string, logger *zap.Logger) *Client {
	return &Client{
		repoOwner: repoOwner,
		repoName:  repoName,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:  logger,
		enabled: true,
	}
}

func (c *Client) CurrentVersion() string {
	return currentVersion
}

func (c *Client) StartBackgroundCheck() {
	if !c.enabled {
		return
	}

	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for range ticker.C {
			if err := c.CheckAndUpdate(); err != nil {
				c.logger.Debug("Auto-update check failed", zap.Error(err))
			}
		}
	}()
}

func (c *Client) CheckAndUpdate() error {
	latest, err := c.GetLatestVersion()
	if err != nil {
		return fmt.Errorf("fetch latest version: %w", err)
	}

	if latest.Version == currentVersion {
		c.logger.Debug("Already on latest version", zap.String("version", currentVersion))
		return nil
	}

	c.logger.Info("New version available",
		zap.String("current", currentVersion),
		zap.String("latest", latest.Version),
	)

	asset, err := c.findAsset(latest)
	if err != nil {
		return fmt.Errorf("find asset: %w", err)
	}

	if err := c.downloadAndVerify(asset); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	c.logger.Info("Update downloaded successfully",
		zap.String("version", latest.Version),
		zap.String("file", asset.Name),
	)

	return nil
}

func (c *Client) GetLatestVersion() (*VersionInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest",
		c.repoOwner, c.repoName)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release struct {
		TagName    string `json:"tag_name"`
		Name       string `json:"name"`
		CreatedAt  string `json:"created_at"`
		Body       string `json:"body"`
		Assets     []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	info := &VersionInfo{
		Version:     release.TagName,
		ReleaseDate: release.CreatedAt,
		Changelog:   release.Body,
	}

	for _, a := range release.Assets {
		osName, arch := parseAssetName(a.Name)
		if osName != "" {
			info.Assets = append(info.Assets, Asset{
				Name:        a.Name,
				OS:          osName,
				Arch:        arch,
				DownloadURL: a.BrowserDownloadURL,
				Size:        a.Size,
			})
		}
	}

	return info, nil
}

func (c *Client) findAsset(version *VersionInfo) (*Asset, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	for i := range version.Assets {
		a := &version.Assets[i]
		if a.OS == goos && a.Arch == goarch {
			return a, nil
		}
	}

	return nil, fmt.Errorf("no asset found for %s/%s", goos, goarch)
}

func (c *Client) downloadAndVerify(asset *Asset) error {
	resp, err := c.httpClient.Get(asset.DownloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "adm-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))

	if asset.SHA256Checksum != "" && actualHash != asset.SHA256Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", asset.SHA256Checksum, actualHash)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return err
	}

	if err := tmpFile.Chmod(0755); err != nil {
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	backupPath := binaryPath + ".bak"
	if err := os.Rename(binaryPath, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), binaryPath); err != nil {
		os.Rename(backupPath, binaryPath)
		return fmt.Errorf("install new binary: %w", err)
	}

	c.logger.Info("Binary updated, restart required",
		zap.String("path", binaryPath),
	)

	return nil
}

func (c *Client) RestartService() error {
	serviceName := "adm-gateway"

	switch runtime.GOOS {
	case "linux":
		return exec.Command("systemctl", "restart", serviceName).Run()
	case "darwin":
		return exec.Command("launchctl", "kickstart", "-k", "system/"+serviceName).Run()
	case "windows":
		return exec.Command("powershell", "-Command",
			fmt.Sprintf("Restart-Service -Name %s", serviceName)).Run()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func parseAssetName(name string) (os, arch string) {
	switch {
	case contains(name, "linux") && contains(name, "amd64"):
		return "linux", "amd64"
	case contains(name, "darwin") && contains(name, "arm64"):
		return "darwin", "arm64"
	case contains(name, "darwin") && contains(name, "amd64"):
		return "darwin", "amd64"
	case contains(name, "windows") && contains(name, "amd64"):
		return "windows", "amd64"
	}
	return "", ""
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
