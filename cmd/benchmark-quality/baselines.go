package main

import (
	"context"
	"log/slog"
	"sort"

	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
)

// rankedResult holds a memory ID and its retrieval score.
type rankedResult struct {
	MemoryID string
	Score    float64
}

// Retriever retrieves ranked memories for a query.
type Retriever interface {
	Name() string
	Retrieve(ctx context.Context, query string, limit int) ([]rankedResult, error)
}

// --- FTS-Only Baseline ---
// Uses SQLite FTS5 with BM25 ranking. No embeddings, no graph traversal.
// This represents the simplest keyword search approach.

type ftsRetriever struct {
	s store.Store
}

func newFTSRetriever(s store.Store) Retriever {
	return &ftsRetriever{s: s}
}

func (r *ftsRetriever) Name() string { return "FTS5 (BM25)" }

func (r *ftsRetriever) Retrieve(ctx context.Context, query string, limit int) ([]rankedResult, error) {
	mems, err := r.s.SearchByFullText(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	results := make([]rankedResult, len(mems))
	for i, m := range mems {
		// FTS5 results are BM25-ranked by the database. Use reciprocal rank as score.
		results[i] = rankedResult{MemoryID: m.ID, Score: 1.0 / float64(i+1)}
	}
	return results, nil
}

// --- Vector-Only Baseline ---
// Uses embedding cosine similarity only. No keyword search, no graph traversal.
// This represents the standard semantic search approach (Chroma, Pinecone, etc).

type vectorRetriever struct {
	s store.Store
	p llm.Provider
}

func newVectorRetriever(s store.Store, p llm.Provider) Retriever {
	return &vectorRetriever{s: s, p: p}
}

func (r *vectorRetriever) Name() string { return "Vector (Cosine)" }

func (r *vectorRetriever) Retrieve(ctx context.Context, query string, limit int) ([]rankedResult, error) {
	emb, err := r.p.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	res, err := r.s.SearchByEmbedding(ctx, emb, limit)
	if err != nil {
		return nil, err
	}
	results := make([]rankedResult, len(res))
	for i, rr := range res {
		results[i] = rankedResult{MemoryID: rr.Memory.ID, Score: float64(rr.Score)}
	}
	return results, nil
}

// --- Hybrid Baseline (Reciprocal Rank Fusion) ---
// Combines FTS5 + Vector results using RRF, the industry-standard fusion method.
// This is what most production RAG systems use (LangChain, LlamaIndex, etc).
// Reference: Cormack, Clarke, Buettcher (2009). "Reciprocal Rank Fusion outperforms
// Condorcet and individual Rank Learning Methods."

type hybridRetriever struct {
	s store.Store
	p llm.Provider
	k float64 // RRF constant (typically 60)
}

func newHybridRetriever(s store.Store, p llm.Provider) Retriever {
	return &hybridRetriever{s: s, p: p, k: 60}
}

func (r *hybridRetriever) Name() string { return "Hybrid (RRF)" }

func (r *hybridRetriever) Retrieve(ctx context.Context, query string, limit int) ([]rankedResult, error) {
	// Fetch extra candidates from each source for better fusion.
	fetchLimit := limit * 3
	if fetchLimit < 15 {
		fetchLimit = 15
	}

	ftsMemories, err := r.s.SearchByFullText(ctx, query, fetchLimit)
	if err != nil {
		return nil, err
	}

	emb, err := r.p.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	vecResults, err := r.s.SearchByEmbedding(ctx, emb, fetchLimit)
	if err != nil {
		return nil, err
	}

	// Reciprocal Rank Fusion: score(d) = Σ 1/(k + rank_i(d))
	scores := make(map[string]float64)
	for i, m := range ftsMemories {
		scores[m.ID] += 1.0 / (r.k + float64(i+1))
	}
	for i, vr := range vecResults {
		scores[vr.Memory.ID] += 1.0 / (r.k + float64(i+1))
	}

	ranked := make([]rankedResult, 0, len(scores))
	for id, score := range scores {
		ranked = append(ranked, rankedResult{MemoryID: id, Score: score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked, nil
}

// --- Mnemonic Retriever ---
// Wraps the full Mnemonic retrieval agent. Configurable name allows
// testing different configurations (e.g., with/without spread activation).

type mnemonicRetriever struct {
	agent *retrieval.RetrievalAgent
	label string
}

func newMnemonicRetriever(s store.Store, p llm.Provider, cfg retrieval.RetrievalConfig, log *slog.Logger, name string) Retriever {
	return &mnemonicRetriever{
		agent: retrieval.NewRetrievalAgent(s, p, cfg, log),
		label: name,
	}
}

func (r *mnemonicRetriever) Name() string { return r.label }

func (r *mnemonicRetriever) Retrieve(ctx context.Context, query string, limit int) ([]rankedResult, error) {
	resp, err := r.agent.Query(ctx, retrieval.QueryRequest{
		Query:      query,
		MaxResults: limit,
	})
	if err != nil {
		return nil, err
	}
	results := make([]rankedResult, len(resp.Memories))
	for i, m := range resp.Memories {
		results[i] = rankedResult{MemoryID: m.Memory.ID, Score: float64(m.Score)}
	}
	return results, nil
}
