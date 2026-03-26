#!/usr/bin/env python3
"""Export a fine-tuned Felix-LM v3 checkpoint to GGUF format.

Preserves the full architecture including spokes by using a custom
"felix" architecture tag. Requires a llama.cpp build with Felix support
to load the resulting GGUF.

Transformations applied:
  1. Strip _orig_mod. prefix (torch.compile artifact)
  2. Absorb depth-extended RoPE into Q/K weights (so llama.cpp uses standard RoPE)
  3. Fuse embed_proj into input embedding
  4. Untie embeddings (separate token_embd and output weights)
  5. Rename tensors to GGUF convention
  6. Export spoke tensors with felix-specific names

Usage:
    python training/scripts/export_gguf.py \\
        --checkpoint checkpoints/v3_mnemonic_100m_ft/last.pt \\
        --output models/felix-encoder-v1.gguf

Requires:
    pip install gguf
"""

import argparse
import json
import math
import sys
from pathlib import Path

import numpy as np
import torch

try:
    from gguf import GGUFWriter
except ImportError:
    print("ERROR: pip install gguf")
    sys.exit(1)

# Add training scripts to path for config access
TRAINING_DIR = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(TRAINING_DIR / "scripts"))


# --- Felix-LM v3 100M config (must match training) ---

def get_config():
    """Return the v3_mnemonic_100m config."""
    return {
        "vocab_size": 32768,
        "d_embed": 512,
        "num_layers": 20,
        "num_heads": 8,
        "num_spokes": 4,
        "spoke_rank": 64,
        "ffn_mult": 4,
        "rope_base": 10000.0,
        "rope_helical_turns": 2,
        "rope_depth_alpha": 1.0,
        "embed_proj": True,
        "tie_embeddings": True,
    }


# --- Step 1: Strip torch.compile prefix ---

def strip_compile_prefix(state_dict):
    """Remove _orig_mod. prefix from all keys."""
    new_sd = {}
    stripped = 0
    for k, v in state_dict.items():
        if k.startswith("_orig_mod."):
            new_sd[k[len("_orig_mod."):]] = v
            stripped += 1
        else:
            new_sd[k] = v
    if stripped:
        print(f"  Stripped _orig_mod. prefix from {stripped} keys")
    return new_sd


# --- Step 2: Absorb depth-extended RoPE into Q/K weights ---

def absorb_depth_rope(state_dict, config):
    """Pre-rotate Q and K projection weights to absorb depth RoPE.

    After this, standard RoPE (position-only) produces identical results.
    The depth component (layer_idx * theta_depth) is baked into the weights.
    """
    d_embed = config["d_embed"]
    num_heads = config["num_heads"]
    head_dim = d_embed // num_heads
    half_dim = head_dim // 2
    num_layers = config["num_layers"]
    helical_turns = config["rope_helical_turns"]
    depth_alpha = config["rope_depth_alpha"]

    if depth_alpha == 0:
        print("  RoPE depth_alpha=0, no absorption needed")
        return state_dict

    theta_depth = depth_alpha * (2.0 * math.pi * helical_turns / num_layers)
    print(f"  Absorbing depth RoPE: theta_depth={theta_depth:.4f} rad ({math.degrees(theta_depth):.1f} deg/layer)")

    for layer_idx in range(num_layers):
        angle = layer_idx * theta_depth

        if abs(angle) < 1e-10:
            continue  # layer 0, no rotation needed

        cos_a = math.cos(angle)
        sin_a = math.sin(angle)

        for proj in ["attn.q_proj.weight", "attn.k_proj.weight"]:
            key = f"layers.{layer_idx}.{proj}"
            W = state_dict[key].float()  # [out_dim, in_dim]

            # Reshape to [num_heads, head_dim, in_dim]
            W_heads = W.view(num_heads, head_dim, -1)

            # Rotate pairs of output dimensions
            for pair_idx in range(half_dim):
                d0 = 2 * pair_idx
                d1 = 2 * pair_idx + 1

                row0 = W_heads[:, d0, :].clone()
                row1 = W_heads[:, d1, :].clone()

                W_heads[:, d0, :] = cos_a * row0 - sin_a * row1
                W_heads[:, d1, :] = sin_a * row0 + cos_a * row1

            state_dict[key] = W_heads.view(W.shape).to(W.dtype)

    print(f"  Absorbed depth RoPE for {num_layers} layers")
    return state_dict


# --- Step 3: Fuse embed_proj and untie embeddings ---

def fuse_embed_proj(state_dict, config):
    """Fuse embed_proj into input embedding and create separate output weight."""
    d_embed = config["d_embed"]

    if not config["embed_proj"]:
        # No embed_proj, just handle logit scale for output
        emb = state_dict["embedding.weight"].float()
        logit_scale = d_embed ** -0.5
        state_dict["output.weight"] = (emb * logit_scale).half()
        state_dict["token_embd.weight"] = emb.half()
        del state_dict["embedding.weight"]
        return state_dict

    emb = state_dict["embedding.weight"].float()       # [V, d]
    proj_w = state_dict["embed_proj.weight"].float()    # [d, d]
    proj_b = state_dict["embed_proj.bias"].float()      # [d]

    # Fused input embedding: emb @ proj_w^T + proj_b
    fused_emb = emb @ proj_w.T + proj_b.unsqueeze(0)   # [V, d]

    # Output weight: original embedding * logit_scale (used for logits)
    logit_scale = d_embed ** -0.5
    output_weight = emb * logit_scale                   # [V, d]

    state_dict["token_embd.weight"] = fused_emb.half()
    state_dict["output.weight"] = output_weight.half()

    # Remove consumed keys
    del state_dict["embedding.weight"]
    del state_dict["embed_proj.weight"]
    del state_dict["embed_proj.bias"]

    print(f"  Fused embed_proj into token_embd ({fused_emb.shape})")
    print(f"  Created separate output weight with logit_scale={logit_scale:.6f}")
    return state_dict


# --- Step 4: Rename tensors ---

def rename_tensors(state_dict, config):
    """Rename Felix state_dict keys to GGUF tensor names.

    Hub (backbone) tensors use standard LLaMA-like names.
    Spoke tensors use felix-specific names.
    """
    num_layers = config["num_layers"]
    name_map = {}

    # Already renamed by fuse_embed_proj
    name_map["token_embd.weight"] = "token_embd.weight"
    name_map["output.weight"] = "output.weight"
    name_map["output_norm.weight"] = "output_norm.weight"

    for i in range(num_layers):
        p = f"layers.{i}"
        b = f"blk.{i}"

        # Attention
        name_map[f"{p}.norm1.weight"] = f"{b}.attn_norm.weight"
        name_map[f"{p}.attn.q_proj.weight"] = f"{b}.attn_q.weight"
        name_map[f"{p}.attn.k_proj.weight"] = f"{b}.attn_k.weight"
        name_map[f"{p}.attn.v_proj.weight"] = f"{b}.attn_v.weight"
        name_map[f"{p}.attn.out_proj.weight"] = f"{b}.attn_output.weight"

        # FFN (SwiGLU)
        name_map[f"{p}.norm2.weight"] = f"{b}.ffn_norm.weight"
        name_map[f"{p}.ffn.w_gate.weight"] = f"{b}.ffn_gate.weight"
        name_map[f"{p}.ffn.w_up.weight"] = f"{b}.ffn_up.weight"
        name_map[f"{p}.ffn.w_down.weight"] = f"{b}.ffn_down.weight"

    # Spokes — felix-specific tensor names
    for key in list(state_dict.keys()):
        if key.startswith("spokes."):
            # spokes.{layer_idx}.{param} -> blk.{layer_idx}.spoke.{param}
            parts = key.split(".", 2)  # ["spokes", "idx", "rest"]
            spoke_layer_idx = parts[1]
            param_path = parts[2]
            gguf_name = f"blk.{spoke_layer_idx}.spoke.{param_path}"
            name_map[key] = gguf_name

    # Apply renames
    renamed = {}
    unmapped = []
    for key, tensor in state_dict.items():
        if key in name_map:
            renamed[name_map[key]] = tensor
        else:
            unmapped.append(key)

    if unmapped:
        print(f"  WARNING: {len(unmapped)} unmapped tensors:")
        for k in unmapped:
            print(f"    {k}")

    print(f"  Renamed {len(renamed)} tensors")
    return renamed


# --- Step 5: Report spoke gate values ---

def report_spoke_gates(state_dict):
    """Print spoke gate values for quality assessment."""
    gates = {}
    for key, tensor in state_dict.items():
        if "gate_bias" in key:
            layer_idx = key.split(".")[1]
            gate_val = torch.sigmoid(tensor).item()
            gates[int(layer_idx)] = gate_val

    if gates:
        print(f"\n  Spoke gates (sigmoid of gate_bias):")
        for idx in sorted(gates.keys()):
            bar = "#" * int(gates[idx] * 40)
            print(f"    Layer {idx:2d}: {gates[idx]:.3f} {bar}")
        print(f"    Mean gate: {sum(gates.values()) / len(gates):.3f}")
    return gates


# --- Step 6: Write GGUF ---

def write_gguf(tensors, config, tokenizer_path, output_path, context_length=4096):
    """Write tensors and metadata to a GGUF file."""
    d_embed = config["d_embed"]
    num_heads = config["num_heads"]
    head_dim = d_embed // num_heads

    writer = GGUFWriter(str(output_path), arch="felix")

    # Model metadata
    writer.add_name("felix-lm-v3-100m-ft")
    writer.add_context_length(context_length)
    writer.add_embedding_length(d_embed)
    writer.add_block_count(config["num_layers"])
    writer.add_head_count(num_heads)
    writer.add_head_count_kv(num_heads)  # no GQA
    writer.add_feed_forward_length(d_embed * config["ffn_mult"])
    writer.add_rope_dimension_count(head_dim)
    writer.add_rope_freq_base(config["rope_base"])
    writer.add_layer_norm_rms_eps(1e-6)

    # Felix-specific metadata
    writer.add_uint32("felix.num_spokes", config["num_spokes"])
    writer.add_uint32("felix.spoke_rank", config["spoke_rank"])

    # Tokenizer
    _write_tokenizer(writer, tokenizer_path, config)

    # Tensors (preserve dtype — F32 for norms/scalars, F16 for weights)
    for name, tensor in tensors.items():
        if tensor.dtype == torch.float32:
            data = tensor.numpy().astype(np.float32)
        else:
            data = tensor.numpy().astype(np.float16)
        writer.add_tensor(name, data)

    writer.write_header_to_file()
    writer.write_kv_data_to_file()
    writer.write_tensors_to_file()
    writer.close()

    file_size = output_path.stat().st_size / (1024 * 1024)
    print(f"\n  Written: {output_path} ({file_size:.1f} MB)")
    print(f"  Tensors: {len(tensors)}")
    print(f"  Context: {context_length}")


def _write_tokenizer(writer, tokenizer_path, config):
    """Write BPE tokenizer data to GGUF."""
    tok_json = json.load(open(tokenizer_path / "tokenizer.json"))
    tok_config = json.load(open(tokenizer_path / "config.json"))

    vocab_size = config["vocab_size"]

    # Extract vocab (token -> id mapping)
    vocab = tok_json.get("model", {}).get("vocab", {})

    # Build token list ordered by id
    tokens = [""] * vocab_size
    for token, idx in vocab.items():
        if idx < vocab_size:
            tokens[idx] = token

    # Extract merges (convert pairs to space-separated strings if needed)
    raw_merges = tok_json.get("model", {}).get("merges", [])
    merges = [" ".join(m) if isinstance(m, list) else m for m in raw_merges]

    # Token types per llama.cpp enum:
    #   0=undefined, 1=normal, 2=unknown, 3=control, 4=user_defined, 5=unused, 6=byte
    token_types = [1] * vocab_size  # 1 = NORMAL

    # Mark special tokens
    # Token 0 is <|endoftext|> (the actual EOS), token 1 is <|pad|>
    pad_id = tok_config.get("pad_token_id", 1)
    eos_id = 0  # <|endoftext|> is at index 0
    token_types[eos_id] = 3  # CONTROL
    token_types[pad_id] = 3  # CONTROL

    # Write tokenizer
    writer.add_tokenizer_model("gpt2")
    writer.add_tokenizer_pre("gpt-2")
    writer.add_token_list(tokens)
    writer.add_token_types(token_types)
    writer.add_token_merges(merges)
    writer.add_bos_token_id(eos_id)  # GPT-2 style: BOS = EOS
    writer.add_eos_token_id(eos_id)
    writer.add_pad_token_id(pad_id)

    print(f"  Tokenizer: {len(vocab)} tokens, {len(merges)} merges, EOS={eos_id}")


# --- Main ---

def main():
    parser = argparse.ArgumentParser(description="Export Felix-LM v3 to GGUF")
    parser.add_argument("--checkpoint", required=True, help="Path to .pt checkpoint")
    parser.add_argument("--output", default=None, help="Output .gguf path (default: auto)")
    parser.add_argument("--tokenizer", default="training/tokenizer", help="Path to tokenizer dir")
    parser.add_argument("--context-length", type=int, default=4096, help="Context length metadata")
    parser.add_argument("--validate", action="store_true", help="Run forward pass validation")
    args = parser.parse_args()

    checkpoint_path = Path(args.checkpoint)
    tokenizer_path = Path(args.tokenizer)
    output_path = Path(args.output) if args.output else checkpoint_path.with_suffix(".gguf")

    config = get_config()

    print(f"\n=== Felix-LM v3 GGUF Export ===")
    print(f"  Checkpoint: {checkpoint_path}")
    print(f"  Output: {output_path}")
    print(f"  Architecture: felix (custom, with spokes)")
    print(f"  Model: {config['d_embed']}d, {config['num_layers']}L, {config['num_heads']}H, "
          f"{config['num_spokes']}S r{config['spoke_rank']}")

    # Load checkpoint
    print(f"\nLoading checkpoint...")
    ckpt = torch.load(str(checkpoint_path), map_location="cpu", weights_only=False)

    if isinstance(ckpt, dict) and "model_state_dict" in ckpt:
        state_dict = ckpt["model_state_dict"]
        step = ckpt.get("global_step", "unknown")
        print(f"  New-format checkpoint (step {step})")
    else:
        state_dict = ckpt
        print(f"  Legacy checkpoint (model weights only)")

    total_params = sum(p.numel() for p in state_dict.values())
    print(f"  Parameters: {total_params:,}")

    # Transform pipeline
    print(f"\nTransforming...")
    state_dict = strip_compile_prefix(state_dict)
    report_spoke_gates(state_dict)
    state_dict = absorb_depth_rope(state_dict, config)
    state_dict = fuse_embed_proj(state_dict, config)

    # Convert tensors to float16 for export, except norms and scalars which stay F32
    # (llama.cpp requires norm weights to be F32 for element-wise multiply compatibility)
    f32_patterns = ("norm", "gate_bias", "output_norm")
    for key in state_dict:
        if any(p in key for p in f32_patterns):
            state_dict[key] = state_dict[key].float()
        else:
            state_dict[key] = state_dict[key].half()

    state_dict = rename_tensors(state_dict, config)

    # Write GGUF
    print(f"\nWriting GGUF...")
    output_path.parent.mkdir(parents=True, exist_ok=True)
    write_gguf(state_dict, config, tokenizer_path, output_path, args.context_length)

    print(f"\n=== Export Complete ===")
    print(f"  To use: load with felix-aware llama.cpp build")
    print(f"  To quantize: llama-quantize {output_path} output-q8_0.gguf q8_0")


if __name__ == "__main__":
    main()
