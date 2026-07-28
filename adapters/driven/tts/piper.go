package tts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"autodj/domain/aidj"
)

// defaultPiperBin is the Piper CLI name (offline pt-BR TTS, CPU).
const defaultPiperBin = "piper"

// Piper implements outbound.TTSGateway by shelling the Piper CLI: it reads text
// on stdin and writes a wav to stdout. It is the self-host fallback when no cloud
// key is set. Bin/Model are injectable; models are per-voice .onnx files.
type Piper struct {
	Bin    string
	Models map[aidj.Voice]string // voice → model .onnx path; default model when unmapped
}

// NewPiper builds a Piper gateway. models maps voices to .onnx model files; the
// default voice's model is the fallback for unmapped voices.
func NewPiper(models map[aidj.Voice]string) *Piper {
	return &Piper{Bin: defaultPiperBin, Models: models}
}

// Available reports whether the Piper binary is on PATH and at least one model
// is configured. Wiring uses it to pick Piper vs no gateway.
func (p *Piper) Available() bool {
	if p.model(aidj.DefaultVoice) == "" {
		return false
	}
	return resolve(p.bin()) != ""
}

func (p *Piper) bin() string {
	if p.Bin != "" {
		return p.Bin
	}
	return defaultPiperBin
}

// model resolves the model path for a voice, falling back to the default voice's.
func (p *Piper) model(v aidj.Voice) string {
	if m, ok := p.Models[v]; ok && m != "" {
		return m
	}
	return p.Models[aidj.DefaultVoice]
}

// Synthesize runs `piper --model <m> --output_file -` with text on stdin and
// returns the wav bytes. Errors when the binary/model is missing or piper fails.
func (p *Piper) Synthesize(ctx context.Context, text string, voice aidj.Voice) (io.ReadCloser, error) {
	bin := resolve(p.bin())
	if bin == "" {
		return nil, fmt.Errorf("tts: piper binary %q not found", p.bin())
	}
	model := p.model(voice)
	if model == "" {
		return nil, fmt.Errorf("tts: no piper model for voice %q", voice)
	}
	cmd := exec.CommandContext(ctx, bin, "--model", model, "--output_file", "-")
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tts: piper: %w", err)
	}
	return io.NopCloser(bytes.NewReader(out)), nil
}

// resolve returns the binary path if it exists on PATH, else "".
func resolve(bin string) string {
	if bin == "" {
		return ""
	}
	if p, err := exec.LookPath(bin); err == nil {
		return p
	}
	return ""
}
