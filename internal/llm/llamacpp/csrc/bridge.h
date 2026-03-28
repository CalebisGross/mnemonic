#ifndef MNM_LLAMACPP_BRIDGE_H
#define MNM_LLAMACPP_BRIDGE_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

// Opaque handle for a loaded model + context
typedef struct mnm_model mnm_model;

typedef struct {
    int context_size;
    int gpu_layers;
    int threads;
    int batch_size;
} mnm_model_params;

typedef struct {
    char *text;           // generated text (caller must free with mnm_free_string)
    int   prompt_tokens;
    int   completion_tokens;
    float mean_prob;      // mean probability of chosen tokens (0-1, higher = more confident)
    float min_prob;       // minimum probability of any chosen token (0-1, lowest confidence point)
} mnm_completion_result;

typedef struct {
    float *data;   // embedding vector (caller must free with mnm_free_floats)
    int    dims;
} mnm_embedding_result;

// Load a GGUF model. Returns NULL on failure.
mnm_model *mnm_load_model(const char *path, mnm_model_params params);

// Free a loaded model.
void mnm_free_model(mnm_model *m);

// Run text completion. grammar may be NULL for unconstrained generation.
// stop is a NULL-terminated array of stop strings (may be NULL).
mnm_completion_result mnm_complete(
    mnm_model  *m,
    const char *prompt,
    int         max_tokens,
    float       temperature,
    float       top_p,
    const char *grammar,
    const char **stop,
    int         n_stop
);

// Generate an embedding vector for the given text.
mnm_embedding_result mnm_embed(mnm_model *m, const char *text);

// Free helpers
void mnm_free_string(char *s);
void mnm_free_floats(float *f);

#ifdef __cplusplus
}
#endif

#endif // MNM_LLAMACPP_BRIDGE_H
