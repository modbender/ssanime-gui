package encode

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCandidatesForOS(t *testing.T) {
	cases := map[string][]string{
		"windows": {"hevc_nvenc", "hevc_qsv", "hevc_amf"},
		"linux":   {"hevc_nvenc", "hevc_qsv"},
		"darwin":  {"hevc_videotoolbox"},
		"plan9":   nil,
	}
	for goos, want := range cases {
		if got := candidatesForOS(goos); !reflect.DeepEqual(got, want) {
			t.Errorf("candidatesForOS(%q) = %v, want %v", goos, got, want)
		}
	}
}

// fakeResolver builds a GPUResolver with an injected probe and OS, recording the
// probe order so the test can assert candidates are tried in priority order.
func fakeResolver(goos string, works map[string]bool, order *[]string) *GPUResolver {
	return &GPUResolver{
		goos:   goos,
		logger: quietLogger(),
		probe: func(_ context.Context, enc string) bool {
			if order != nil {
				*order = append(*order, enc)
			}
			return works[enc]
		},
	}
}

func TestResolveGPUEncoderFirstWorkingWins(t *testing.T) {
	var order []string
	// nvenc fails, qsv works → qsv chosen, amf never probed.
	r := fakeResolver("windows", map[string]bool{"hevc_qsv": true}, &order)
	name, cpuFallback := r.ResolveGPUEncoder()
	if name != "hevc_qsv" || cpuFallback {
		t.Errorf("got %q cpuFallback=%v, want hevc_qsv/false", name, cpuFallback)
	}
	if !reflect.DeepEqual(order, []string{"hevc_nvenc", "hevc_qsv"}) {
		t.Errorf("probe order = %v, want [hevc_nvenc hevc_qsv] (stops at first working)", order)
	}
}

func TestResolveGPUEncoderPriorityOrder(t *testing.T) {
	var order []string
	// All work → the first (nvenc) wins on Windows.
	r := fakeResolver("windows", map[string]bool{"hevc_nvenc": true, "hevc_qsv": true, "hevc_amf": true}, &order)
	name, _ := r.ResolveGPUEncoder()
	if name != "hevc_nvenc" {
		t.Errorf("got %q, want hevc_nvenc (highest priority)", name)
	}
	if !reflect.DeepEqual(order, []string{"hevc_nvenc"}) {
		t.Errorf("probe order = %v, want only [hevc_nvenc]", order)
	}
}

func TestResolveGPUEncoderFallsBackToLibx265(t *testing.T) {
	// Nothing probes → CPU fallback, cpuFallback=true.
	r := fakeResolver("linux", map[string]bool{}, nil)
	name, cpuFallback := r.ResolveGPUEncoder()
	if name != cpuEncoder || !cpuFallback {
		t.Errorf("got %q cpuFallback=%v, want %s/true", name, cpuFallback, cpuEncoder)
	}
}

func TestResolveGPUEncoderCachedOncePerProcess(t *testing.T) {
	var calls int
	r := &GPUResolver{
		goos:   "windows",
		logger: quietLogger(),
		probe: func(_ context.Context, enc string) bool {
			calls++
			return enc == "hevc_nvenc"
		},
	}
	r.ResolveGPUEncoder()
	r.ResolveGPUEncoder()
	if calls != 1 {
		t.Errorf("probe ran %d times, want 1 (cached once per process)", calls)
	}
}

func TestHWProbeArgsShape(t *testing.T) {
	args := hwProbeArgs("hevc_nvenc")
	joined := args
	// Must use a synthetic source, the encoder under test, and discard output.
	want := []string{"-f", "lavfi", "-c:v", "hevc_nvenc", "-f", "null", "-"}
	for _, w := range want {
		found := false
		for _, a := range joined {
			if a == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("hwProbeArgs missing %q: %v", w, joined)
		}
	}
}
