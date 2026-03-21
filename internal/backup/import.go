package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

type ImportMode string

const (
	ModeMerge   ImportMode = "merge"
	ModeReplace ImportMode = "replace"
)

type ImportResult struct {
	MemoriesImported     int
	AssociationsImported int
	RawMemoriesImported  int
	SkippedDuplicates    int
	Errors               []string
	Duration             time.Duration
}

func ImportFromJSON(ctx context.Context, s store.Store, filePath string, mode ImportMode) (*ImportResult, error) {
	startTime := time.Now()
	result := &ImportResult{
		Errors: []string{},
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read import file: %w", err)
	}

	var exportData ExportData
	if err := json.Unmarshal(data, &exportData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal export data: %w", err)
	}

	if mode == ModeReplace {
		result.Errors = append(result.Errors, "warning: replace mode not fully implemented for safety reasons; using merge mode instead")
	}

	// Import memories
	for _, memory := range exportData.Memories {
		// Ensure raw_id is never empty — use id as fallback
		if memory.RawID == "" {
			memory.RawID = memory.ID
		}
		if err := s.WriteMemory(ctx, memory); err != nil {
			result.SkippedDuplicates++
		} else {
			result.MemoriesImported++
		}
	}

	// Import associations
	for _, association := range exportData.Associations {
		if err := s.CreateAssociation(ctx, association); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to import association: %v", err))
		} else {
			result.AssociationsImported++
		}
	}

	// Import raw memories
	for _, rawMemory := range exportData.RawMemories {
		if err := s.WriteRaw(ctx, rawMemory); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to import raw memory: %v", err))
		} else {
			result.RawMemoriesImported++
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}
