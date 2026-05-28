// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package factory

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/platform/config"
)

func TestNew_Anthropic_GlobalKey(t *testing.T) {
	t.Parallel()
	cfg := config.Config{LLM: config.LLMConfig{ActiveModel: "claude-sonnet-4-6", APIKey: "sk-global"}}
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("Name = %q; want anthropic", p.Name())
	}
}

func TestNew_Anthropic_NoKey_AuthFailed(t *testing.T) {
	t.Parallel()
	cfg := config.Config{LLM: config.LLMConfig{ActiveModel: "claude-sonnet-4-6"}}
	_, err := New(cfg)
	if !errors.Is(err, llm.ErrAuthFailed) {
		t.Errorf("no key → %v; want ErrAuthFailed", err)
	}
}

func TestNew_PerModelKeyOverridesGlobal(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		LLM:    config.LLMConfig{ActiveModel: "m1", APIKey: "sk-global"},
		Models: []config.ModelConfig{{Name: "m1", Provider: "anthropic", APIKey: "sk-permodel"}},
	}
	// Resolve directly to assert key precedence (R2 H-4).
	r := resolve(cfg)
	if r.apiKey != "sk-permodel" {
		t.Errorf("apiKey = %q; want per-model sk-permodel", r.apiKey)
	}
	if r.provider != "anthropic" || r.model != "m1" {
		t.Errorf("resolve = %+v; want anthropic/m1", r)
	}
}

func TestNew_Fake(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		LLM:    config.LLMConfig{ActiveModel: "fakemodel"},
		Models: []config.ModelConfig{{Name: "fakemodel", Provider: "fake"}},
	}
	p, err := New(cfg)
	if err != nil || p.Name() != "fake" {
		t.Errorf("fake provider: p=%v err=%v", p, err)
	}
}

func TestNew_OpenAICompat(t *testing.T) {
	t.Parallel()
	// spec-1.20.1: openai-compat with key + base_url constructs (was NOT_IMPLEMENTED).
	cfg := config.Config{
		LLM:    config.LLMConfig{ActiveModel: "m"},
		Models: []config.ModelConfig{{Name: "m", Provider: "openai-compat", APIKey: "sk", BaseURL: "https://api.deepseek.com/v1"}},
	}
	p, err := New(cfg)
	if err != nil || p == nil {
		t.Fatalf("openai-compat → %v; want provider", err)
	}
	if p.Name() != "openai-compat" {
		t.Errorf("Name = %q; want openai-compat", p.Name())
	}
}

func TestNew_OpenAICompat_NoKey(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		LLM:    config.LLMConfig{ActiveModel: "m"},
		Models: []config.ModelConfig{{Name: "m", Provider: "openai-compat", BaseURL: "https://x/v1"}},
	}
	if _, err := New(cfg); !errors.Is(err, llm.ErrAuthFailed) {
		t.Errorf("openai-compat no key → %v; want ErrAuthFailed", err)
	}
}

func TestNew_Ollama_Keyless(t *testing.T) {
	t.Parallel()
	// ollama is keyless (local OpenAI-compat endpoint); Name() distinguishes it.
	cfg := config.Config{
		LLM:    config.LLMConfig{ActiveModel: "qwen"},
		Models: []config.ModelConfig{{Name: "qwen", Provider: "ollama", BaseURL: "http://localhost:11434/v1"}},
	}
	p, err := New(cfg)
	if err != nil || p == nil {
		t.Fatalf("ollama keyless → %v; want provider", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("Name = %q; want ollama", p.Name())
	}
}

func TestNew_AdaptiveThinking_NotImplemented(t *testing.T) {
	t.Parallel()
	cfg := config.Config{LLM: config.LLMConfig{ActiveModel: "m", APIKey: "sk", ThinkingMode: "adaptive"}}
	_, err := New(cfg)
	if !errors.Is(err, llm.ErrNotImplemented) {
		t.Errorf("adaptive thinking → %v; want ErrNotImplemented", err)
	}
}

func TestNew_UnknownProvider_Unavailable(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		LLM:    config.LLMConfig{ActiveModel: "m"},
		Models: []config.ModelConfig{{Name: "m", Provider: "bogus"}},
	}
	_, err := New(cfg)
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Errorf("unknown provider → %v; want ErrUnavailable", err)
	}
}
