package routes

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/updater"
)

// UpdateCheckResponse is the JSON response for the update check endpoint.
type UpdateCheckResponse struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url"`
}

// UpdateResponse is the JSON response for the update endpoint.
type UpdateResponse struct {
	Status          string `json:"status"`
	PreviousVersion string `json:"previous_version,omitempty"`
	NewVersion      string `json:"new_version,omitempty"`
	RestartPending  bool   `json:"restart_pending"`
	Message         string `json:"message,omitempty"`
}

// ServiceRestarter can restart the daemon service after an update.
// Restart must be safe to call from within the running daemon — it should
// spawn the restart asynchronously (e.g. via systemctl restart) so the
// current process can finish responding before being killed.
type ServiceRestarter interface {
	IsInstalled() bool
	Restart() error
}

// HandleUpdateCheck returns an HTTP handler that checks for available updates
// by querying the GitHub Releases API. No authentication required.
func HandleUpdateCheck(version string, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("update check requested")

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		info, err := updater.CheckForUpdate(ctx, version)
		if err != nil {
			log.Error("update check failed", "error", err)
			writeError(w, http.StatusBadGateway, "failed to check for updates: "+err.Error(), "UPDATE_CHECK_ERROR")
			return
		}

		resp := UpdateCheckResponse{
			CurrentVersion:  info.CurrentVersion,
			LatestVersion:   info.LatestVersion,
			UpdateAvailable: info.UpdateAvailable,
			ReleaseURL:      info.ReleaseURL,
		}

		log.Info("update check completed", "current", info.CurrentVersion, "latest", info.LatestVersion, "available", info.UpdateAvailable)
		writeJSON(w, http.StatusOK, resp)
	}
}

// PIDRestartFunc is a fallback restart function for when no platform service
// manager is installed. It receives the binary path and config path, spawns a
// background process to restart the daemon, and returns.
type PIDRestartFunc func(execPath, configPath string) error

// HandleUpdate returns an HTTP handler that downloads and installs an available update.
// It tries the platform service manager first, then falls back to PID-based restart.
func HandleUpdate(version string, svc ServiceRestarter, pidRestart PIDRestartFunc, configPath string, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("update requested via API")

		// Use a generous timeout for download (5 minutes)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		info, err := updater.CheckForUpdate(ctx, version)
		if err != nil {
			log.Error("update check failed", "error", err)
			writeError(w, http.StatusBadGateway, "failed to check for updates: "+err.Error(), "UPDATE_CHECK_ERROR")
			return
		}

		if !info.UpdateAvailable {
			resp := UpdateResponse{
				Status:  "up_to_date",
				Message: "already running the latest version",
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}

		result, err := updater.PerformUpdate(ctx, info)
		if err != nil {
			log.Error("update failed", "error", err)
			writeError(w, http.StatusInternalServerError, "update failed: "+err.Error(), "UPDATE_ERROR")
			return
		}

		log.Info("update installed", "previous", result.PreviousVersion, "new", result.NewVersion, "binary", result.BinaryPath)

		// Determine restart strategy: service manager first, then PID fallback
		useServiceManager := svc != nil && svc.IsInstalled()
		canRestart := useServiceManager || pidRestart != nil

		resp := UpdateResponse{
			Status:          "updated",
			PreviousVersion: result.PreviousVersion,
			NewVersion:      result.NewVersion,
			RestartPending:  canRestart,
		}

		if !canRestart {
			resp.Message = "update installed — restart the daemon manually to use the new version"
		}

		// Send response before restarting
		writeJSON(w, http.StatusOK, resp)

		// Restart the daemon in the background.
		if useServiceManager {
			go func() {
				time.Sleep(500 * time.Millisecond)
				log.Info("restarting daemon via service manager")
				if err := svc.Restart(); err != nil {
					log.Error("failed to restart daemon via service manager", "error", err)
				}
			}()
		} else if pidRestart != nil {
			go func() {
				time.Sleep(500 * time.Millisecond)
				log.Info("restarting daemon via PID fallback")
				if err := pidRestart(result.BinaryPath, configPath); err != nil {
					log.Error("failed to restart daemon via PID fallback", "error", err)
				}
			}()
		}
	}
}
