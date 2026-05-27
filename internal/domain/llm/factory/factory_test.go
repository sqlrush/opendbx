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

func TestNew_OpenAICompat_NotImplemented(t *testing.T) {
	t.Parallel()
	for _, prov := range []string{"openai-compat", "ollama"} {
		cfg := config.Config{
			LLM:    config.LLMConfig{ActiveModel: "m"},
			Models: []config.ModelConfig{{Name: "m", Provider: prov}},
		}
		_, err := New(cfg)
		if !errors.Is(err, llm.ErrNotImplemented) {
			t.Errorf("provider %q → %v; want ErrNotImplemented", prov, err)
		}
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
