# LM Studio Setup

Mnemonic uses a local LLM via [LM Studio](https://lmstudio.ai/) for semantic understanding. This guide walks through the recommended setup.

> **Alternative:** You can use Google Gemini API or any OpenAI-compatible provider instead of LM Studio. Set `llm.endpoint` to your provider's URL and export `LLM_API_KEY` with your API key. Cloud APIs typically use `llm.max_concurrent: 8` (vs 2 for local). If using a cloud provider, skip the LM Studio sections below and go straight to [Configure Mnemonic](#configure-mnemonic).

## Install LM Studio

Download from [lmstudio.ai](https://lmstudio.ai/) and install. LM Studio runs entirely on your machine — no cloud API keys needed.

## Download Models

Mnemonic needs two models: a **chat model** for reasoning and a **embedding model** for vector search.

### Recommended Models

| Purpose | Model | Size | Notes |
|---------|-------|------|-------|
| Chat | `qwen/qwen3.5-9b` | ~5.2 GB (Q4_K_M) | Good balance of quality and speed |
| Embedding | `embeddinggemma-300m-qat` | ~300 MB | Fast, accurate embeddings |

To download: open LM Studio, go to the **Discover** tab, search for each model, and click Download.

**Alternatives:**
- For machines with less RAM (< 16 GB), use a smaller chat model like `qwen2.5-3b`
- For machines with more RAM (32+ GB), try `qwen3.5-14b` for better reasoning
- Any OpenAI-compatible embedding model works for the embedding slot

## Configure the Server

1. In LM Studio, go to the **Developer** tab (or **Local Server** in older versions).

2. **Load both models:**
   - Select your chat model as the primary model
   - Load the embedding model in the embedding slot

3. **Server settings:**
   - Port: `1234` (default)
   - Context length: **16384** (16K) minimum — needed for synthesis with tool-use
   - GPU offload: maximize layers for your hardware
   - Concurrent requests: `2` (matches `llm.max_concurrent` default)

4. Click **Start Server**.

5. Verify it's running:
   ```bash
   curl http://localhost:1234/v1/models
   ```
   You should see both models listed.

## Configure Mnemonic

Edit `config.yaml` to match your LM Studio models:

```yaml
llm:
  endpoint: "http://localhost:1234/v1"
  chat_model: "qwen/qwen3.5-9b"           # must match LM Studio model ID
  embedding_model: "text-embedding-embeddinggemma-300m-qat"
  max_tokens: 4096
  temperature: 0.3
  timeout_sec: 120
  max_concurrent: 2
```

**Finding the model ID:** In LM Studio, the model ID is shown in the server tab when the model is loaded. Use that exact string.

## Verify the Connection

```bash
mnemonic diagnose
```

Look for:
```
LLM: healthy
  Chat model: qwen/qwen3.5-9b
  Embedding model: text-embedding-embeddinggemma-300m-qat
```

## Context Size Recommendations

| Use Case | Minimum Context | Recommended |
|----------|----------------|-------------|
| Basic encoding | 4K | 8K |
| Retrieval with synthesis | 8K | 16K |
| Full tool-use synthesis | 16K | 16K+ |

The retrieval agent uses tool-use, which requires larger context windows. If you see truncated or incomplete synthesis results, increase the context length in LM Studio.

## Performance Tuning

**For faster encoding:**
- Increase `llm.max_concurrent` to 3–4 if your GPU has headroom
- Use a quantized model (Q4_K_M is the sweet spot for quality vs speed)

**For lower resource usage:**
- Set `llm.max_concurrent: 1`
- Use a smaller chat model
- Increase consolidation interval to reduce LLM calls:
  ```yaml
  consolidation:
    interval_hours: 12  # default is 6
  ```

**For better quality:**
- Use a larger chat model (14B+)
- Set `llm.temperature: 0.1` for more deterministic outputs
- Increase context length to 32K if your model supports it

## Embedding Model Changes

If you switch embedding models, existing embeddings become incompatible. The daemon detects this at startup and warns about "embedding model drift." Options:

1. **Stick with the same model** — simplest approach
2. **Re-encode** — run `mnemonic consolidate` to re-process, but existing embeddings won't be updated
3. **Fresh start** — `mnemonic purge` and re-ingest (loses history)

## Troubleshooting

- **"LLM unhealthy"** — LM Studio server not started, or wrong port. Check `curl http://localhost:1234/v1/models`.
- **Slow encoding** — Model too large for available VRAM, causing CPU fallback. Use a smaller quantization or model.
- **Out of memory** — Reduce context length or switch to a smaller model.
- **"connection refused"** — LM Studio app is open but the server tab hasn't been started. Click Start Server.

See [Troubleshooting](troubleshooting.md) for more issues.
