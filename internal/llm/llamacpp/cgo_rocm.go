//go:build llamacpp && rocm

package llamacpp

/*
#cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build/ggml/src/ggml-hip -L/opt/rocm/lib -lggml-hip -lhipblas -lamdhip64 -lrocblas
*/
import "C"
