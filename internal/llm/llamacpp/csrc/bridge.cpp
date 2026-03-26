#include "bridge.h"
#include "llama.h"

#include <cmath>
#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>

struct mnm_model {
    struct llama_model   *model;
    struct llama_context *ctx;       // completion context
    struct llama_context *ctx_embd;  // embedding context (mean pooling)
    int                   n_ctx;
    int                   n_threads;
};

// Helper: fill a batch manually (llama_batch_add was removed from public API)
static void batch_add(struct llama_batch &batch, llama_token token, llama_pos pos, llama_seq_id seq_id, bool logits) {
    int i = batch.n_tokens;
    batch.token[i]      = token;
    batch.pos[i]        = pos;
    batch.n_seq_id[i]   = 1;
    batch.seq_id[i][0]  = seq_id;
    batch.logits[i]     = logits ? 1 : 0;
    batch.n_tokens++;
}

static void batch_clear(struct llama_batch &batch) {
    batch.n_tokens = 0;
}

extern "C" {

mnm_model *mnm_load_model(const char *path, mnm_model_params params) {
    llama_backend_init();

    auto mparams = llama_model_default_params();
    mparams.n_gpu_layers = params.gpu_layers;

    struct llama_model *model = llama_model_load_from_file(path, mparams);
    if (!model) {
        return NULL;
    }

    auto cparams = llama_context_default_params();
    cparams.n_ctx   = params.context_size > 0 ? params.context_size : 2048;
    cparams.n_batch = params.batch_size > 0 ? params.batch_size : 512;
    cparams.n_threads       = params.threads > 0 ? params.threads : 4;
    cparams.n_threads_batch = params.threads > 0 ? params.threads : 4;

    struct llama_context *ctx = llama_init_from_model(model, cparams);
    if (!ctx) {
        llama_model_free(model);
        return NULL;
    }

    // Create a separate context for embedding extraction (mean pooling)
    auto eparams = cparams;
    eparams.embeddings  = true;
    eparams.pooling_type = LLAMA_POOLING_TYPE_MEAN;

    struct llama_context *ctx_embd = llama_init_from_model(model, eparams);
    // ctx_embd may be NULL if the model doesn't support embeddings — that's OK

    auto *m = new mnm_model;
    m->model     = model;
    m->ctx       = ctx;
    m->ctx_embd  = ctx_embd;
    m->n_ctx     = cparams.n_ctx;
    m->n_threads = cparams.n_threads;
    return m;
}

void mnm_free_model(mnm_model *m) {
    if (!m) return;
    if (m->ctx_embd) llama_free(m->ctx_embd);
    if (m->ctx)      llama_free(m->ctx);
    if (m->model)    llama_model_free(m->model);
    delete m;
}

mnm_completion_result mnm_complete(
    mnm_model  *m,
    const char *prompt,
    int         max_tokens,
    float       temperature,
    float       top_p,
    const char *grammar,
    const char **stop,
    int         n_stop
) {
    mnm_completion_result result = {NULL, 0, 0};
    if (!m || !prompt) return result;

    const struct llama_vocab *vocab = llama_model_get_vocab(m->model);

    // Tokenize prompt
    int n_prompt = strlen(prompt);
    std::vector<llama_token> tokens(n_prompt + 16);
    int n_tokens = llama_tokenize(vocab, prompt, n_prompt,
                                   tokens.data(), tokens.size(),
                                   false, true);
    if (n_tokens < 0) {
        tokens.resize(-n_tokens);
        n_tokens = llama_tokenize(vocab, prompt, n_prompt,
                                   tokens.data(), tokens.size(),
                                   false, true);
    }
    tokens.resize(n_tokens);
    result.prompt_tokens = n_tokens;

    // Clear memory (KV cache)
    llama_memory_clear(llama_get_memory(m->ctx), true);

    // Decode prompt
    llama_batch batch = llama_batch_init(n_tokens + 1, 0, 1);
    for (int i = 0; i < n_tokens; i++) {
        batch_add(batch, tokens[i], i, 0, i == n_tokens - 1);
    }
    llama_decode(m->ctx, batch);

    // Set up sampler
    auto sparams = llama_sampler_chain_default_params();
    struct llama_sampler *smpl = llama_sampler_chain_init(sparams);

    if (grammar && grammar[0] != '\0') {
        llama_sampler_chain_add(smpl, llama_sampler_init_grammar(vocab, grammar, "root"));
    }

    llama_sampler_chain_add(smpl, llama_sampler_init_top_k(40));
    llama_sampler_chain_add(smpl, llama_sampler_init_top_p(top_p, 1));
    llama_sampler_chain_add(smpl, llama_sampler_init_min_p(0.05f, 1));
    llama_sampler_chain_add(smpl, llama_sampler_init_temp(temperature));
    llama_sampler_chain_add(smpl, llama_sampler_init_dist(LLAMA_DEFAULT_SEED));

    // Generate tokens
    std::string output;
    int n_generated = 0;
    int n_pos = n_tokens;

    for (int i = 0; i < max_tokens; i++) {
        llama_token new_token = llama_sampler_sample(smpl, m->ctx, -1);

        if (llama_vocab_is_eog(vocab, new_token)) {
            break;
        }

        // Decode token to text
        char buf[256];
        int n = llama_token_to_piece(vocab, new_token, buf, sizeof(buf), 0, true);
        if (n > 0) {
            std::string piece(buf, n);
            output += piece;

            // Check stop sequences
            bool should_stop = false;
            for (int s = 0; s < n_stop && stop; s++) {
                if (stop[s] && output.find(stop[s]) != std::string::npos) {
                    size_t pos = output.find(stop[s]);
                    output = output.substr(0, pos);
                    should_stop = true;
                    break;
                }
            }
            if (should_stop) break;
        }

        n_generated++;

        // Prepare next batch (single token)
        batch_clear(batch);
        batch_add(batch, new_token, n_pos++, 0, true);
        llama_decode(m->ctx, batch);
    }

    llama_sampler_free(smpl);
    llama_batch_free(batch);

    result.text = strdup(output.c_str());
    result.completion_tokens = n_generated;
    return result;
}

mnm_embedding_result mnm_embed(mnm_model *m, const char *text) {
    mnm_embedding_result result = {NULL, 0};
    if (!m || !text || !m->ctx_embd) return result;

    const struct llama_vocab *vocab = llama_model_get_vocab(m->model);
    int n_embd = llama_model_n_embd(m->model);

    // Tokenize
    int n_text = strlen(text);
    std::vector<llama_token> tokens(n_text + 16);
    int n_tokens = llama_tokenize(vocab, text, n_text,
                                   tokens.data(), tokens.size(),
                                   false, true);
    if (n_tokens < 0) {
        tokens.resize(-n_tokens);
        n_tokens = llama_tokenize(vocab, text, n_text,
                                   tokens.data(), tokens.size(),
                                   false, true);
    }
    tokens.resize(n_tokens);

    // Use the dedicated embedding context (mean pooling enabled)
    llama_memory_clear(llama_get_memory(m->ctx_embd), true);

    llama_batch batch = llama_batch_init(tokens.size(), 0, 1);
    for (int i = 0; i < n_tokens; i++) {
        batch_add(batch, tokens[i], i, 0, true);
    }

    if (llama_decode(m->ctx_embd, batch) != 0) {
        llama_batch_free(batch);
        return result;
    }
    llama_batch_free(batch);

    // Get mean-pooled embeddings from the embedding context
    float *embd = llama_get_embeddings_seq(m->ctx_embd, 0);

    if (embd) {
        result.data = (float *)malloc(n_embd * sizeof(float));
        memcpy(result.data, embd, n_embd * sizeof(float));
        result.dims = n_embd;

        // L2 normalize
        float norm = 0.0f;
        for (int i = 0; i < n_embd; i++) {
            norm += result.data[i] * result.data[i];
        }
        norm = sqrtf(norm);
        if (norm > 0.0f) {
            for (int i = 0; i < n_embd; i++) {
                result.data[i] /= norm;
            }
        }
    }

    return result;
}

void mnm_free_string(char *s) { free(s); }
void mnm_free_floats(float *f) { free(f); }

} // extern "C"
