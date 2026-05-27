// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package factory

import (
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/anthropic"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
	"github.com/sqlrush/opendbx/internal/domain/llm/openai"
	"github.com/sqlrush/opendbx/internal/platform/config"
)

// Resolved is the outcome of resolving config into a concrete provider
// choice (provider id + model name + api key), before construction.
type resolved struct {
	provider string
	model    string
	apiKey   string
	baseURL  string
}

// New builds an llm.Provider from config (spec-1.20 D-7).
//
// Resolution (R2 H-4):
//   - ActiveModel → matching cfg.Models[i].Name; if none, ActiveModel is
//     used as a bare model name with LLMConfig credentials (provider
//     defaults to "anthropic").
//   - API key precedence: per-model ModelConfig.APIKey > global LLMConfig.APIKey.
//
// Provider dispatch:
//   - "anthropic" → anthropic.New
//   - "fake"      → fake (empty scripted; tests inject their own via fake.New)
//   - "openai-compat" → openai.New (cloud OpenAI-compat; key + base_url required) [spec-1.20.1]
//   - "ollama"        → openai.New (keyless local OpenAI-compat endpoint) [spec-1.20.1]
//   - other → LLM.UNAVAILABLE
//
// 原则 3: an unconstructable provider returns an LLM.* errcode — never a
// silent fallback. config ThinkingMode="adaptive" → LLM.NOT_IMPLEMENTED
// (R2.1 MED; adaptive thinking is spec-3.11).
func New(cfg config.Config) (llm.Provider, error) {
	if cfg.LLM.ThinkingMode == "adaptive" {
		return nil, llm.ErrNotImplemented
	}
	r := resolve(cfg)
	switch r.provider {
	case "anthropic":
		return anthropic.New(anthropic.Config{APIKey: r.apiKey, Model: r.model, BaseURL: r.baseURL})
	case "fake":
		return fake.New(), nil
	case "openai-compat":
		return openai.New(openai.Config{APIKey: r.apiKey, Model: r.model, BaseURL: r.baseURL, Name: "openai-compat"})
	case "ollama":
		return openai.New(openai.Config{APIKey: r.apiKey, Model: r.model, BaseURL: r.baseURL, Name: "ollama", AllowKeyless: true})
	default:
		return nil, llm.ErrUnavailable
	}
}

// resolve applies the ActiveModel → Models lookup + API-key precedence.
func resolve(cfg config.Config) resolved {
	r := resolved{
		provider: "anthropic", // default when no Models entry matches
		model:    cfg.LLM.ActiveModel,
		apiKey:   cfg.LLM.APIKey,
		baseURL:  cfg.LLM.BaseURL,
	}
	for i := range cfg.Models {
		mc := cfg.Models[i]
		if mc.Name == cfg.LLM.ActiveModel {
			r.provider = mc.Provider
			r.model = mc.Name
			if mc.APIKey != "" { // per-model overrides global (R2 H-4)
				r.apiKey = mc.APIKey
			}
			if mc.BaseURL != "" {
				r.baseURL = mc.BaseURL
			}
			break
		}
	}
	return r
}
