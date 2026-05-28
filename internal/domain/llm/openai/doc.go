// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package openai is the OpenAI-compatible LLM provider (covers deepseek /
// qwen / glm / moonshot / ollama via /v1/chat/completions). It implements
// the model-agnostic llm.Provider (规则 16) over the official openai-go SDK,
// with the SDK import isolated to this package (IMP-7 / 规则 16).
//
// Design: spec-1.20.1-openai-compat.md (T-4 骨架: client + params 基架 +
// stream wrapper; 完整 delta/tool_calls/reasoning/finish 映射 → T-5;
// classifyOpenAIErr + factory 接通 → T-6).
//
// Author: sqlrush
package openai
