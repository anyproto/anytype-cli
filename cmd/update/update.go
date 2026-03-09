package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
	"github.com/spf13/cobra"
)

const (
	githubOwner = "anyproto"
	githubRepo  = "anytype-cli"
)

func NewUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update to the latest version",
		Long:  "Download and install the latest version of the Anytype CLI from GitHub releases.",
		RunE:  runUpdate,
	}
}

func runUpdate(cmd *cobra.Command, args []string) error {
	output.Info("Checking for updates...")

	latest, err := getLatestVersion()
	if err != nil {
		return output.Error("Failed to check latest version: %w", err)
	}

	current := core.GetVersion()

	currentBase := current
	if idx := strings.Index(current, "-"); idx != -1 {
		currentBase = current[:idx]
	}

	if currentBase >= latest {
		output.Info("Already up to date (%s)", current)
		return nil
	}

	output.Info("Updating from %s to %s...", current, latest)

	if err := downloadAndInstall(latest); err != nil {
		return output.Error("update failed: %w", err)
	}

	output.Success("Successfully updated to %s", latest)
	output.Info("If the service is installed, restart it with: anytype service restart")
	output.Info("Otherwise, restart your terminal or run 'anytype' to use the new version")
	return nil
}

func getLatestVersion() (string, error) {
	resp, err := githubAPI("GET", "/releases/latest", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", handleAPIError(resp)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", output.Error("Failed to parse release: %w", err)
	}

	return release.TagName, nil
}

func downloadAndInstall(version string) error {
	tempDir, err := os.MkdirTemp("", "anytype-update-*")
	if err != nil {
		return output.Error("Failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, getArchiveName(version))
	if err := downloadRelease(version, archivePath); err != nil {
		return err
	}

	if err := extractArchive(archivePath, tempDir); err != nil {
		return output.Error("Failed to extract: %w", err)
	}

	binaryName := "anytype"
	if runtime.GOOS == "windows" {
		binaryName = "anytype.exe"
	}

	newBinary := filepath.Join(tempDir, binaryName)
	if _, err := os.Stat(newBinary); err != nil {
		return output.Error("binary not found in archive (expected %s)", binaryName)
	}

	if err := replaceBinary(newBinary); err != nil {
		return output.Error("Failed to install: %w", err)
	}

	return nil
}

func getArchiveName(version string) string {
	base := fmt.Sprintf("anytype-cli-%s-%s-%s", version, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		return base + ".zip"
	}
	return base + ".tar.gz"
}

func downloadRelease(version, destination string) error {
	archiveName := filepath.Base(destination)
	output.Info("Downloading %s...", archiveName)

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return downloadViaAPI(version, archiveName, destination)
	}

	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		githubOwner, githubRepo, version, archiveName)

	return downloadFile(url, destination, "")
}

func downloadViaAPI(version, filename, destination string) error {
	resp, err := githubAPI("GET", fmt.Sprintf("/releases/tags/%s", version), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var release struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return output.Error("Failed to parse release: %w", err)
	}

	var assetURL string
	for _, asset := range release.Assets {
		if asset.Name == filename {
			assetURL = asset.URL
			break
		}
	}
	if assetURL == "" {
		return output.Error("release asset %s not found", filename)
	}

	return downloadFile(assetURL, destination, os.Getenv("GITHUB_TOKEN"))
}

func downloadFile(url, destination, token string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if token != "" {
		req.Header.Set("Authorization", "token "+token)
		req.Header.Set("Accept", "application/octet-stream")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return output.Error("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractArchive(archivePath, destDir string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destDir)
	}
	return extractTarGz(archivePath, destDir)
}

func extractTarGz(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("illegal file path: %s", header.Name)
		}
		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, header.FileInfo().Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.Contains(f.Name, "..") {
			return fmt.Errorf("illegal file path: %s", f.Name)
		}
		target := filepath.Join(destDir, f.Name)

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, f.Mode())
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		if err := writeFile(target, rc, f.Mode()); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}
	return nil
}

func writeFile(path string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, r); err != nil {
		return err
	}

	return os.Chmod(path, mode)
}

func replaceBinary(newBinary string) error {
	if err := os.Chmod(newBinary, 0755); err != nil {
		return err
	}

	currentBinary, err := os.Executable()
	if err != nil {
		return err
	}
	currentBinary, err = filepath.EvalSymlinks(currentBinary)
	if err != nil {
		return err
	}

	if err := os.Rename(newBinary, currentBinary); err != nil {
		if runtime.GOOS == "windows" {
			// Windows locks running exe, so we can't overwrite.
			// Rename the current binary first, then replace with the new one.
			current := core.GetVersion()
			oldBinary := currentBinary + "_" + current
			if err := os.Rename(currentBinary, oldBinary); err != nil {
				return output.Error("failed to move current binary (permission denied). Try running as administrator: %w", err)
			}
			if err := os.Rename(newBinary, currentBinary); err != nil {
				// Attempt to restore the original
				_ = os.Rename(oldBinary, currentBinary)
				return output.Error("failed to install new binary: %w", err)
			}

			return nil
		} else {
			cmd := exec.Command("mv", newBinary, currentBinary)
			if err := cmd.Run(); err != nil {
				return output.Error("failed to replace binary at %s (permission denied). To get the latest version, reinstall from repository: https://github.com/%s/%s", currentBinary, githubOwner, githubRepo)
			}
		}
	}

	return nil
}

func githubAPI(method, endpoint string, body io.Reader) (*http.Response, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s%s", githubOwner, githubRepo, endpoint)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	return http.DefaultClient.Do(req)
}

func handleAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return output.Error("API error %d: %s", resp.StatusCode, string(body))
}
