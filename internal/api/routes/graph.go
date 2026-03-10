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

			// edgeKey deduplicates edges between the same pair of episodes.
			type edgeKey struct{ a, b string }
			makeKey := func(a, b string) edgeKey {
				if a > b {
					a, b = b, a
				}
				return edgeKey{a, b}
			}
			bestEdge := make(map[edgeKey]GraphEdge)
			addEdge := func(e GraphEdge) {
				k := makeKey(e.Source, e.Target)
				if existing, ok := bestEdge[k]; !ok || e.Strength > existing.Strength {
					bestEdge[k] = e
				}
			}

			// --- Edge source 1: Associations between episode member memories ---
			// Build a map from memory ID -> episode ID for all member memories.
			memToEpisode := make(map[string]string)
			for _, ep := range episodes {
				if !nodeIDs[ep.ID] {
					continue
				}
				for _, mid := range ep.MemoryIDs {
					memToEpisode[mid] = ep.ID
				}
				for _, rid := range ep.RawMemoryIDs {
					memToEpisode[rid] = ep.ID
				}
			}

			// Fetch all associations and promote to episode-level edges.
			associations, assocErr := s.ListAllAssociations(ctx)
			if assocErr != nil {
				log.Warn("failed to fetch associations for episode graph", "error", assocErr)
			} else {
				for _, assoc := range associations {
					srcEp := memToEpisode[assoc.SourceID]
					tgtEp := memToEpisode[assoc.TargetID]
					if srcEp == "" || tgtEp == "" || srcEp == tgtEp {
						continue
					}
					if !nodeIDs[srcEp] || !nodeIDs[tgtEp] {
						continue
					}
					addEdge(GraphEdge{
						Source:       srcEp,
						Target:       tgtEp,
						Strength:     assoc.Strength,
						RelationType: assoc.RelationType,
					})
				}
			}

			// --- Edge source 2: Temporal proximity ---
			// Episodes within 30 minutes of each other get a temporal edge.
			const temporalWindow = 30 * time.Minute
			// Build a time-sorted index from nodeIDs-filtered episodes.
			type epTime struct {
				id    string
				start time.Time
				end   time.Time
			}
			var timeSorted []epTime
			for _, ep := range episodes {
				if !nodeIDs[ep.ID] {
					continue
				}
				timeSorted = append(timeSorted, epTime{ep.ID, ep.StartTime, ep.EndTime})
			}
			for i := 0; i < len(timeSorted); i++ {
				for j := i + 1; j < len(timeSorted); j++ {
					gap := timeSorted[j].start.Sub(timeSorted[i].end)
					if gap > temporalWindow {
						break
					}
					if gap < 0 {
						gap = 0
					}
					// Strength decays linearly from 0.8 (overlap) to 0.2 (30min apart)
					strength := float32(0.8 - 0.6*(float64(gap)/float64(temporalWindow)))
					addEdge(GraphEdge{
						Source:       timeSorted[i].id,
						Target:       timeSorted[j].id,
						Strength:     strength,
						RelationType: "temporal",
					})
				}
			}

			// --- Edge source 3: Shared concepts (fallback, lowered to 1+ shared) ---
			for i := 0; i < len(nodes); i++ {
				for j := i + 1; j < len(nodes); j++ {
					conceptMap := make(map[string]bool, len(nodes[i].Concepts))
					for _, c := range nodes[i].Concepts {
						conceptMap[c] = true
					}
					sharedCount := 0
					for _, c := range nodes[j].Concepts {
						if conceptMap[c] {
							sharedCount++
						}
					}
					if sharedCount >= 1 {
						total := len(nodes[i].Concepts) + len(nodes[j].Concepts)
						strength := float32(sharedCount) / float32(total) * 2
						if strength > 1.0 {
							strength = 1.0
						}
						addEdge(GraphEdge{
							Source:       nodes[i].ID,
							Target:       nodes[j].ID,
							Strength:     strength,
							RelationType: "similar",
						})
					}
				}
			}

			// Collect edges that meet the strength threshold.
			var edges []GraphEdge
			for _, e := range bestEdge {
				if e.Strength >= minStrength {
					edges = append(edges, e)
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
