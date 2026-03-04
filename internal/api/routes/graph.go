package routes

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/appsprout/mnemonic/internal/store"
)

type GraphNode struct {
	ID            string   `json:"id"`
	Summary       string   `json:"summary"`
	Salience      float32  `json:"salience"`
	State         string   `json:"state"`
	Concepts      []string `json:"concepts"`
	EmotionalTone string   `json:"emotional_tone,omitempty"`
	Significance  string   `json:"significance,omitempty"`
	Timestamp     string   `json:"timestamp"`
	FilesModified []string `json:"files_modified,omitempty"`
	EventCount    int      `json:"event_count,omitempty"`
}

type GraphEdge struct {
	Source       string  `json:"source"`
	Target       string  `json:"target"`
	Strength     float32 `json:"strength"`
	RelationType string  `json:"relation_type"`
}

type GraphResponse struct {
	Nodes     []GraphNode `json:"nodes"`
	Edges     []GraphEdge `json:"edges"`
	Timestamp string      `json:"timestamp"`
}

func HandleGraph(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("graph data requested")

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		// Parse query params
		limit := 200
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		minSalience := float32(0.0)
		if ms := r.URL.Query().Get("min_salience"); ms != "" {
			if parsed, err := strconv.ParseFloat(ms, 32); err == nil {
				minSalience = float32(parsed)
			}
		}

		minStrength := float32(0.0)
		if ms := r.URL.Query().Get("min_strength"); ms != "" {
			if parsed, err := strconv.ParseFloat(ms, 32); err == nil {
				minStrength = float32(parsed)
			}
		}

		view := r.URL.Query().Get("view")
		if view == "" {
			view = "episodes"
		}

		var resp GraphResponse

		if view == "episodes" {
			// Fetch episodes as nodes
			episodes, err := s.ListEpisodes(ctx, store.EpisodeStateClosed, limit, 0)
			if err != nil {
				log.Error("failed to list episodes for graph", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to list episodes", "STORE_ERROR")
				return
			}

			// Build node set and ID lookup
			nodeIDs := make(map[string]bool, len(episodes))
			var nodes []GraphNode
			for _, ep := range episodes {
				if ep.Salience < minSalience {
					continue
				}
				if strings.TrimSpace(ep.Title) == "" {
					continue
				}
				nodeIDs[ep.ID] = true

				node := GraphNode{
					ID:            ep.ID,
					Summary:       ep.Title,
					Salience:      ep.Salience,
					State:         ep.State,
					Concepts:      ep.Concepts,
					Timestamp:     ep.StartTime.Format(time.RFC3339),
					FilesModified: ep.FilesModified,
					EventCount:    len(ep.RawMemoryIDs),
					EmotionalTone: ep.EmotionalTone,
					Significance:  ep.Outcome,
				}

				nodes = append(nodes, node)
			}

			// Build edges from shared concepts between episodes
			var edges []GraphEdge
			for i := 0; i < len(nodes); i++ {
				for j := i + 1; j < len(nodes); j++ {
					concepts1 := nodes[i].Concepts
					concepts2 := nodes[j].Concepts

					// Find shared concepts
					sharedCount := 0
					conceptMap := make(map[string]bool)
					for _, c := range concepts1 {
						conceptMap[c] = true
					}
					for _, c := range concepts2 {
						if conceptMap[c] {
							sharedCount++
						}
					}

					// Create edge if 2+ shared concepts and strength meets threshold
					if sharedCount >= 2 {
						strength := float32(sharedCount) / float32(len(concepts1)+len(concepts2)) * 2
						if strength > 1.0 {
							strength = 1.0
						}
						if strength >= minStrength {
							edges = append(edges, GraphEdge{
								Source:       nodes[i].ID,
								Target:       nodes[j].ID,
								Strength:     strength,
								RelationType: "shared_concepts",
							})
						}
					}
				}
			}

			if nodes == nil {
				nodes = []GraphNode{}
			}
			if edges == nil {
				edges = []GraphEdge{}
			}

			resp = GraphResponse{
				Nodes:     nodes,
				Edges:     edges,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}
		} else {
			// Original behavior: memories view
			// Get memories (nodes)
			memories, err := s.ListMemories(ctx, "", limit, 0)
			if err != nil {
				log.Error("failed to list memories for graph", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to list memories", "STORE_ERROR")
				return
			}

			// Build node set and ID lookup — skip empty summaries
			nodeIDs := make(map[string]bool, len(memories))
			var nodes []GraphNode
			for _, mem := range memories {
				if mem.Salience < minSalience {
					continue
				}
				if strings.TrimSpace(mem.Summary) == "" {
					continue
				}
				nodeIDs[mem.ID] = true

				node := GraphNode{
					ID:        mem.ID,
					Summary:   mem.Summary,
					Salience:  mem.Salience,
					State:     mem.State,
					Concepts:  mem.Concepts,
					Timestamp: mem.Timestamp.Format(time.RFC3339),
				}

				// Try to get memory attributes for emotional tone and significance
				attrs, attrErr := s.GetMemoryAttributes(ctx, mem.ID)
				if attrErr == nil {
					node.EmotionalTone = attrs.EmotionalTone
					node.Significance = attrs.Significance
				}

				nodes = append(nodes, node)
			}

			// Get all associations (edges)
			associations, err := s.ListAllAssociations(ctx)
			if err != nil {
				log.Error("failed to list associations for graph", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to list associations", "STORE_ERROR")
				return
			}

			// Filter to only edges where both endpoints are in our node set and meet strength threshold
			var edges []GraphEdge
			for _, assoc := range associations {
				if nodeIDs[assoc.SourceID] && nodeIDs[assoc.TargetID] && assoc.Strength >= minStrength {
					edges = append(edges, GraphEdge{
						Source:       assoc.SourceID,
						Target:       assoc.TargetID,
						Strength:     assoc.Strength,
						RelationType: assoc.RelationType,
					})
				}
			}

			resp = GraphResponse{
				Nodes:     nodes,
				Edges:     edges,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
