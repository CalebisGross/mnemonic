package routes

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// --- Evolution endpoint ---

type evolutionResponse struct {
	Principles []principle         `json:"principles"`
	Strategies map[string]strategy `json:"strategies"`
	Patches    []patch             `json:"patches"`
	Timestamp  string              `json:"timestamp"`
}

type principle struct {
	ID         string  `json:"id" yaml:"id"`
	Text       string  `json:"text" yaml:"text"`
	Source     string  `json:"source" yaml:"source"`
	Confidence float64 `json:"confidence" yaml:"confidence"`
	Created    string  `json:"created,omitempty" yaml:"created"`
}

type strategy struct {
	Steps []string `json:"steps" yaml:"steps"`
	Tips  []string `json:"tips,omitempty" yaml:"tips"`
}

type patch struct {
	ID          string `json:"id" yaml:"id"`
	Action      string `json:"action,omitempty" yaml:"action"`
	Content     string `json:"content,omitempty" yaml:"content"`
	Instruction string `json:"instruction,omitempty" yaml:"instruction"`
	Reason      string `json:"reason,omitempty" yaml:"reason"`
	Created     string `json:"created,omitempty" yaml:"created"`
}

// HandleAgentEvolution returns a handler that reads evolution YAML files.
func HandleAgentEvolution(evolutionDir string, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("agent evolution requested")

		resp := evolutionResponse{
			Principles: []principle{},
			Strategies: map[string]strategy{},
			Patches:    []patch{},
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		}

		// Read principles
		if data, err := os.ReadFile(filepath.Join(evolutionDir, "principles.yaml")); err == nil {
			var doc struct {
				Principles []principle `yaml:"principles"`
			}
			if err := yaml.Unmarshal(data, &doc); err == nil && doc.Principles != nil {
				resp.Principles = doc.Principles
			}
		}

		// Read strategies
		if data, err := os.ReadFile(filepath.Join(evolutionDir, "strategies.yaml")); err == nil {
			var doc struct {
				Strategies map[string]strategy `yaml:"strategies"`
			}
			if err := yaml.Unmarshal(data, &doc); err == nil && doc.Strategies != nil {
				resp.Strategies = doc.Strategies
			}
		}

		// Read patches (supports both "patches" and "prompt_patches" YAML keys)
		if data, err := os.ReadFile(filepath.Join(evolutionDir, "prompt_patches.yaml")); err == nil {
			var doc struct {
				Patches       []patch `yaml:"patches"`
				PromptPatches []patch `yaml:"prompt_patches"`
			}
			if err := yaml.Unmarshal(data, &doc); err == nil {
				if doc.Patches != nil {
					resp.Patches = doc.Patches
				} else if doc.PromptPatches != nil {
					resp.Patches = doc.PromptPatches
				}
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// --- Changelog endpoint ---

type changelogResponse struct {
	Raw       string           `json:"raw"`
	Entries   []changelogEntry `json:"entries"`
	Timestamp string           `json:"timestamp"`
}

type changelogEntry struct {
	Date      string `json:"date"`
	Title     string `json:"title"`
	Rationale string `json:"rationale"`
}

// HandleAgentChangelog returns a handler that reads and parses changelog.md.
func HandleAgentChangelog(evolutionDir string, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("agent changelog requested")

		resp := changelogResponse{
			Entries:   []changelogEntry{},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		data, err := os.ReadFile(filepath.Join(evolutionDir, "changelog.md"))
		if err != nil {
			writeJSON(w, http.StatusOK, resp)
			return
		}

		resp.Raw = string(data)

		// Parse markdown: split on "## " for date sections, "### " for entries
		lines := strings.Split(resp.Raw, "\n")
		var currentDate string
		var currentEntry *changelogEntry

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "## ") {
				// Save previous entry
				if currentEntry != nil {
					resp.Entries = append(resp.Entries, *currentEntry)
					currentEntry = nil
				}
				currentDate = strings.TrimPrefix(trimmed, "## ")
			} else if strings.HasPrefix(trimmed, "### ") {
				// Save previous entry
				if currentEntry != nil {
					resp.Entries = append(resp.Entries, *currentEntry)
				}
				currentEntry = &changelogEntry{
					Date:  currentDate,
					Title: strings.TrimPrefix(trimmed, "### "),
				}
			} else if strings.HasPrefix(trimmed, "**") && strings.Contains(trimmed, "**") && currentDate != "" {
				// Bold lines also serve as entry titles (common changelog format)
				if currentEntry != nil {
					resp.Entries = append(resp.Entries, *currentEntry)
				}
				title := trimmed
				// Strip leading/trailing ** markers
				title = strings.TrimPrefix(title, "**")
				if idx := strings.Index(title, "**"); idx >= 0 {
					title = title[:idx]
				}
				currentEntry = &changelogEntry{
					Date:  currentDate,
					Title: strings.TrimSpace(title),
				}
			} else if currentEntry != nil && trimmed != "" {
				// Accumulate rationale text
				text := strings.TrimPrefix(trimmed, "- ")
				text = strings.TrimPrefix(text, "* ")
				if currentEntry.Rationale != "" {
					currentEntry.Rationale += " "
				}
				currentEntry.Rationale += text
			}
		}
		if currentEntry != nil {
			resp.Entries = append(resp.Entries, *currentEntry)
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// --- Sessions endpoint ---

type sessionsResponse struct {
	Sessions  []sessionRecord `json:"sessions"`
	Stats     sessionStats    `json:"stats"`
	Timestamp string          `json:"timestamp"`
}

type sessionRecord struct {
	ID      string       `json:"id"`
	Started string       `json:"started"`
	Model   string       `json:"model"`
	Tasks   []taskRecord `json:"tasks"`
}

type taskRecord struct {
	Description string  `json:"description"`
	Started     string  `json:"started"`
	DurationMS  int64   `json:"duration_ms"`
	CostUSD     float64 `json:"cost_usd"`
	Turns       int     `json:"turns"`
	Evolved     bool    `json:"evolved"`
	ConvID      string  `json:"conv_id,omitempty"`
}

type sessionStats struct {
	SessionCount int     `json:"session_count"`
	TotalTasks   int     `json:"total_tasks"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	AvgTurns     float64 `json:"avg_turns"`
}

// HandleAgentSessions returns a handler that reads sessions.json.
func HandleAgentSessions(evolutionDir string, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("agent sessions requested")

		resp := sessionsResponse{
			Sessions:  []sessionRecord{},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		data, err := os.ReadFile(filepath.Join(evolutionDir, "sessions.json"))
		if err != nil {
			writeJSON(w, http.StatusOK, resp)
			return
		}

		var doc struct {
			Sessions []sessionRecord `json:"sessions"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			log.Warn("failed to parse sessions.json", "error", err)
			writeJSON(w, http.StatusOK, resp)
			return
		}
		if doc.Sessions != nil {
			resp.Sessions = doc.Sessions
		}

		// Compute aggregates
		var totalTurns int
		for _, s := range resp.Sessions {
			resp.Stats.SessionCount++
			for _, t := range s.Tasks {
				resp.Stats.TotalTasks++
				resp.Stats.TotalCostUSD += t.CostUSD
				totalTurns += t.Turns
			}
		}
		if resp.Stats.TotalTasks > 0 {
			resp.Stats.AvgTurns = float64(totalTurns) / float64(resp.Stats.TotalTasks)
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// --- Config endpoint ---

// HandleAgentConfig returns the agent chat configuration for the frontend.
func HandleAgentConfig(webPort int, log *slog.Logger) http.HandlerFunc {
	type agentConfigResponse struct {
		ChatEnabled bool `json:"chat_enabled"`
		WebPort     int  `json:"web_port,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, agentConfigResponse{
			ChatEnabled: webPort > 0,
			WebPort:     webPort,
		})
	}
}
