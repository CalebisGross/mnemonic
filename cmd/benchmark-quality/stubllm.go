package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/appsprout-dev/mnemonic/internal/llm"
)

const bowDims = 128

// vocabulary is the fixed bag-of-words vocabulary. Each word maps to a
// fixed dimension in the embedding space. Texts sharing words produce
// similar embeddings, making retrieval and association scores meaningful.
var vocabulary = map[string]int{
	// Languages & runtimes
	"go": 0, "golang": 0, "python": 1, "javascript": 2, "typescript": 3,
	"sql": 4, "bash": 5, "html": 6, "css": 7, "rust": 8, "java": 9,
	// Infrastructure
	"docker": 10, "git": 11, "linux": 12, "macos": 13, "systemd": 14,
	"build": 15, "ci": 16, "deployment": 17, "deploy": 17, "kubernetes": 18,
	// Dev activities
	"debugging": 19, "debug": 19, "testing": 20, "test": 20,
	"refactoring": 21, "refactor": 21, "configuration": 22, "config": 22,
	"migration": 23, "documentation": 24, "review": 25,
	// Code domains
	"api": 26, "database": 27, "db": 27, "sqlite": 27, "postgres": 27, "postgresql": 27,
	"filesystem": 28, "file": 28, "networking": 29, "network": 29, "connection": 29,
	"security": 30, "authentication": 31, "auth": 31, "login": 31, "session": 31,
	"performance": 32, "logging": 33, "log": 33, "ui": 34, "cli": 35,
	"latency": 32, "throughput": 32, "slow": 32, "fast": 32, "speed": 32,
	// Memory system
	"memory": 36, "encoding": 37, "retrieval": 38, "embedding": 39,
	"agent": 40, "llm": 41, "daemon": 42, "mcp": 43, "watcher": 44,
	// Project context — with synonyms
	"decision": 45, "chose": 45, "choose": 45, "selected": 45, "picked": 45, "choice": 45,
	"error": 46, "bug": 46, "issue": 46, "problem": 46, "defect": 46, "incident": 46, "outage": 46,
	"fix": 47, "fixed": 47, "resolve": 47, "resolved": 47, "solution": 47, "repair": 47, "patch": 47, "workaround": 47,
	"insight": 48, "learning": 49, "planning": 50, "research": 51,
	"dependency": 52, "library": 52, "module": 52, "schema": 53, "config_yaml": 54,
	// Common nouns
	"server": 55, "client": 56, "request": 57, "response": 58,
	"cache": 59, "redis": 59, "memcached": 59, "queue": 60, "event": 61, "handler": 62,
	"middleware": 63, "route": 64, "endpoint": 65,
	"function": 66, "method": 67, "interface": 68, "struct": 69,
	"channel": 70, "goroutine": 71, "mutex": 72, "context": 73,
	// Actions
	"create": 74, "read": 75, "update": 76, "delete": 77,
	"query": 78, "search": 79, "filter": 80, "sort": 81,
	"parse": 82, "validate": 83, "transform": 84, "serialize": 85,
	// Qualities — with synonyms
	"nil": 86, "null": 86, "panic": 87, "crash": 87, "failure": 87, "failed": 87, "broken": 87,
	"timeout": 88, "retry": 89, "fallback": 90, "graceful": 91,
	"concurrent": 92, "concurrency": 92, "pool": 92, "async": 93, "sync": 94,
	// Specific to mnemonic
	"spread": 95, "activation": 96, "association": 97, "salience": 98,
	"consolidation": 99, "decay": 100, "dreaming": 101, "abstraction": 102,
	"episoding": 103, "metacognition": 104, "perception": 105,
	"fts5": 106, "bm25": 107, "cosine": 108, "similarity": 109,
	// General — with synonyms
	"pattern": 110, "principle": 111, "rule": 111, "guideline": 111, "axiom": 112,
	"graph": 113, "node": 114, "edge": 115,
	"threshold": 116, "weight": 117, "score": 118,
	"architecture": 119, "design": 120, "tradeoff": 121, "tradeoffs": 121,
	// System noise vocabulary (distinct region)
	"chrome": 122, "browser": 122, "clipboard": 123,
	"desktop": 124, "gnome": 124, "notification": 125,
	"audio": 126, "pipewire": 126, "trash": 127,
}

// wordSplitRe splits text into words for bag-of-words.
var wordSplitRe = regexp.MustCompile(`[a-zA-Z][a-z]*|[A-Z]+`)

// semanticStubProvider implements llm.Provider with deterministic,
// semantically meaningful embeddings and template-based completions.
type semanticStubProvider struct{}

func (s *semanticStubProvider) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if len(req.Messages) == 0 {
		return llm.CompletionResponse{Content: "", StopReason: "stub"}, nil
	}

	systemPrompt := ""
	userContent := ""
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
		}
		if msg.Role == "user" {
			userContent = msg.Content
		}
	}

	// Detect which agent is calling based on system prompt.
	var content string
	switch {
	case strings.Contains(systemPrompt, "memory encoder"):
		content = stubEncodingResponse(userContent)
	case strings.Contains(systemPrompt, "classifier"):
		content = stubClassificationResponse(userContent)
	case strings.Contains(systemPrompt, "episode synthesizer"):
		content = stubEpisodicResponse(userContent)
	case strings.Contains(systemPrompt, "insight generator"):
		content = stubInsightResponse(userContent)
	case strings.Contains(systemPrompt, "principle synthesizer"):
		content = stubPrincipleResponse(userContent)
	case strings.Contains(systemPrompt, "axiom synthesizer"):
		content = stubAxiomResponse(userContent)
	default:
		content = "{}"
	}

	return llm.CompletionResponse{Content: content, StopReason: "stop"}, nil
}

func (s *semanticStubProvider) Embed(_ context.Context, text string) ([]float32, error) {
	return bowEmbedding(text), nil
}

func (s *semanticStubProvider) BatchEmbed(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, t := range texts {
		results[i] = bowEmbedding(t)
	}
	return results, nil
}

func (s *semanticStubProvider) Health(_ context.Context) error {
	return nil
}

func (s *semanticStubProvider) ModelInfo(_ context.Context) (llm.ModelMetadata, error) {
	return llm.ModelMetadata{Name: "semantic-stub"}, nil
}

// bowEmbedding creates a bag-of-words embedding. Words in the vocabulary
// activate their assigned dimension. Unknown words hash into the space.
// Result is normalized to a unit vector.
func bowEmbedding(text string) []float32 {
	emb := make([]float32, bowDims)
	lower := strings.ToLower(text)
	words := wordSplitRe.FindAllString(lower, -1)

	for _, w := range words {
		if dim, ok := vocabulary[w]; ok {
			emb[dim] += 1.0
		} else {
			// Hash unknown words into the embedding space.
			h := fnv.New32a()
			_, _ = h.Write([]byte(w))
			dim := int(h.Sum32()) % bowDims
			emb[dim] += 0.3 // weaker signal for unknown words
		}
	}

	// Normalize to unit vector.
	var norm float64
	for _, v := range emb {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range emb {
			emb[i] = float32(float64(emb[i]) / norm)
		}
	}
	return emb
}

// extractTopConcepts returns the top N vocabulary words found in text,
// ranked by frequency.
func extractTopConcepts(text string, n int) []string {
	lower := strings.ToLower(text)
	words := wordSplitRe.FindAllString(lower, -1)

	// Count vocabulary word hits (deduplicated by dimension to group synonyms).
	type dimCount struct {
		word  string
		dim   int
		count int
	}
	dimCounts := make(map[int]*dimCount)
	for _, w := range words {
		if dim, ok := vocabulary[w]; ok {
			if dc, exists := dimCounts[dim]; exists {
				dc.count++
			} else {
				dimCounts[dim] = &dimCount{word: w, dim: dim, count: 1}
			}
		}
	}

	// Sort by count descending.
	sorted := make([]*dimCount, 0, len(dimCounts))
	for _, dc := range dimCounts {
		sorted = append(sorted, dc)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	result := make([]string, 0, n)
	for i := 0; i < n && i < len(sorted); i++ {
		result = append(result, sorted[i].word)
	}
	return result
}

// truncate returns the first n characters of s, or s if shorter.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// deterministic salience based on vocabulary density.
func computeSalience(text string) float32 {
	lower := strings.ToLower(text)
	words := wordSplitRe.FindAllString(lower, -1)
	if len(words) == 0 {
		return 0.3
	}
	vocabHits := 0
	for _, w := range words {
		if _, ok := vocabulary[w]; ok {
			vocabHits++
		}
	}
	ratio := float32(vocabHits) / float32(len(words))
	// Map ratio to salience range [0.3, 0.9].
	sal := 0.3 + ratio*0.6
	if sal > 0.9 {
		sal = 0.9
	}
	return sal
}

// stubEncodingResponse returns a valid encoding_response JSON.
func stubEncodingResponse(userContent string) string {
	concepts := extractTopConcepts(userContent, 8)
	if len(concepts) == 0 {
		concepts = []string{"general"}
	}

	// Extract the actual content after "CONTENT:" marker.
	content := userContent
	if _, after, found := strings.Cut(userContent, "CONTENT:"); found {
		content = strings.TrimSpace(after)
	}

	summary := truncateStr(content, 100)
	gist := truncateStr(content, 60)
	salience := computeSalience(content)

	// Determine significance from salience.
	significance := "routine"
	if salience > 0.7 {
		significance = "important"
	} else if salience > 0.5 {
		significance = "notable"
	}

	resp := map[string]any{
		"gist":      gist,
		"summary":   summary,
		"content":   truncateStr(content, 500),
		"narrative": fmt.Sprintf("Observed: %s", truncateStr(content, 200)),
		"concepts":  concepts,
		"structured_concepts": map[string]any{
			"topics":    []any{},
			"entities":  []any{},
			"actions":   []any{},
			"causality": []any{},
		},
		"significance":   significance,
		"emotional_tone": "neutral",
		"outcome":        "ongoing",
		"salience":       salience,
	}

	b, _ := json.Marshal(resp)
	return string(b)
}

// stubClassificationResponse returns a valid classification_response JSON.
func stubClassificationResponse(userContent string) string {
	lower := strings.ToLower(userContent)

	relationType := "similar"
	switch {
	case strings.Contains(lower, "caused") || strings.Contains(lower, "because") ||
		strings.Contains(lower, "led to") || strings.Contains(lower, "result"):
		relationType = "caused_by"
	case strings.Contains(lower, "part of") || strings.Contains(lower, "component") ||
		strings.Contains(lower, "belongs"):
		relationType = "part_of"
	case strings.Contains(lower, "contradict") || strings.Contains(lower, "opposite") ||
		strings.Contains(lower, "however"):
		relationType = "contradicts"
	case strings.Contains(lower, "before") || strings.Contains(lower, "after") ||
		strings.Contains(lower, "then") || strings.Contains(lower, "later"):
		relationType = "temporal"
	case strings.Contains(lower, "reinforce") || strings.Contains(lower, "confirm") ||
		strings.Contains(lower, "support"):
		relationType = "reinforces"
	}

	resp := map[string]string{"relation_type": relationType}
	b, _ := json.Marshal(resp)
	return string(b)
}

// stubEpisodicResponse returns a valid episode_synthesis JSON.
func stubEpisodicResponse(userContent string) string {
	concepts := extractTopConcepts(userContent, 5)
	if len(concepts) == 0 {
		concepts = []string{"session"}
	}

	title := fmt.Sprintf("Session: %s", strings.Join(concepts, ", "))
	if len(title) > 80 {
		title = title[:80]
	}

	salience := computeSalience(userContent)

	resp := map[string]any{
		"title":          title,
		"summary":        fmt.Sprintf("Work session involving %s", strings.Join(concepts, ", ")),
		"narrative":      fmt.Sprintf("During this session, activity was observed related to %s.", strings.Join(concepts, ", ")),
		"emotional_tone": "neutral",
		"outcome":        "ongoing",
		"concepts":       concepts,
		"salience":       salience,
	}

	b, _ := json.Marshal(resp)
	return string(b)
}

// stubInsightResponse returns a valid insight_response JSON.
func stubInsightResponse(userContent string) string {
	concepts := extractTopConcepts(userContent, 6)

	// Only generate insight if there's meaningful concept overlap.
	hasInsight := len(concepts) >= 3

	resp := map[string]any{
		"has_insight": hasInsight,
		"title":       "",
		"insight":     "",
		"concepts":    concepts,
		"confidence":  0.0,
	}

	if hasInsight {
		resp["title"] = fmt.Sprintf("Connection: %s", strings.Join(concepts[:3], " + "))
		resp["insight"] = fmt.Sprintf("These memories share a pattern around %s, suggesting a recurring theme in the workflow.", strings.Join(concepts, ", "))
		resp["confidence"] = 0.7
	}

	b, _ := json.Marshal(resp)
	return string(b)
}

// stubPrincipleResponse returns a valid principle_response JSON.
func stubPrincipleResponse(userContent string) string {
	concepts := extractTopConcepts(userContent, 5)

	hasPrinciple := len(concepts) >= 2

	resp := map[string]any{
		"has_principle": hasPrinciple,
		"title":         "",
		"principle":     "",
		"concepts":      concepts,
		"confidence":    0.0,
	}

	if hasPrinciple {
		resp["title"] = fmt.Sprintf("Principle: %s", strings.Join(concepts[:2], " and "))
		resp["principle"] = fmt.Sprintf("When working with %s, consistent patterns emerge around %s.", concepts[0], strings.Join(concepts[1:], " and "))
		resp["confidence"] = 0.6
	}

	b, _ := json.Marshal(resp)
	return string(b)
}

// stubAxiomResponse returns a valid axiom_response JSON.
func stubAxiomResponse(userContent string) string {
	concepts := extractTopConcepts(userContent, 4)

	hasAxiom := len(concepts) >= 3

	resp := map[string]any{
		"has_axiom":  hasAxiom,
		"title":      "",
		"axiom":      "",
		"concepts":   concepts,
		"confidence": 0.0,
	}

	if hasAxiom {
		resp["title"] = fmt.Sprintf("Axiom: %s", concepts[0])
		resp["axiom"] = fmt.Sprintf("Across all observed patterns, %s serves as a fundamental organizing principle.", concepts[0])
		resp["confidence"] = 0.5
	}

	b, _ := json.Marshal(resp)
	return string(b)
}
