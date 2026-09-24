package nodes

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Tsinling0525/rivulet/dsl"
	"github.com/Tsinling0525/rivulet/llmclient"
)

// LLM calls an OpenAI-compatible chat model. It is Dify's llm node, limited to
// one provider round-trip: no knowledge context, no memory, no tool calls.
type LLM struct{}

func (LLM) Type() string { return dsl.NodeLLM }

type llmModel struct {
	Provider         string         `yaml:"provider"`
	Name             string         `yaml:"name"`
	Mode             string         `yaml:"mode"`
	Endpoint         string         `yaml:"endpoint"`
	CompletionParams map[string]any `yaml:"completion_params"`
}

type promptMessage struct {
	Role string `yaml:"role"`
	Text string `yaml:"text"`
}

type llmVariable struct {
	Variable      string       `yaml:"variable"`
	ValueSelector dsl.Selector `yaml:"value_selector"`
}

type llmConfig struct {
	Model          llmModel        `yaml:"model"`
	PromptTemplate []promptMessage `yaml:"prompt_template"`
	Variables      []llmVariable   `yaml:"variables"`
	Context        struct {
		Enabled          bool         `yaml:"enabled"`
		VariableSelector dsl.Selector `yaml:"variable_selector"`
	} `yaml:"context"`
	Vision struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"vision"`
	StructuredOutput struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"structured_output"`
}

func (LLM) Validate(node dsl.Node) []dsl.Problem {
	var cfg llmConfig
	if problems := decodeExtra(node, &cfg); problems != nil {
		return problems
	}
	var problems []dsl.Problem
	if len(cfg.PromptTemplate) == 0 {
		problems = append(problems, problem(node, "prompt_template must contain at least one message"))
	}
	for index, message := range cfg.PromptTemplate {
		switch message.Role {
		case "system", "user", "assistant":
		case "":
			problems = append(problems, problem(node, "prompt_template[%d] has no role", index))
		default:
			problems = append(problems, problem(node, "prompt_template[%d] has unsupported role %q", index, message.Role))
		}
	}
	if strings.TrimSpace(cfg.Model.Name) == "" {
		problems = append(problems, dsl.Problem{
			Severity: dsl.SeverityWarning,
			NodeID:   node.ID,
			Message:  "model.name is empty; pass --model (or set it in the DSL) or the run will fail",
		})
	}
	if cfg.Model.Provider != "" && providerOf(cfg.Model.Provider) == "" {
		problems = append(problems, dsl.Problem{
			Severity: dsl.SeverityWarning,
			NodeID:   node.ID,
			Message:  fmt.Sprintf("provider %q is not recognised; supported: %s", cfg.Model.Provider, strings.Join(providers(), ", ")),
		})
	}
	if cfg.Vision.Enabled {
		problems = append(problems, dsl.Problem{
			Severity: dsl.SeverityWarning,
			NodeID:   node.ID,
			Message:  "vision is not implemented; image inputs are ignored",
		})
	}
	if cfg.StructuredOutput.Enabled {
		problems = append(problems, dsl.Problem{
			Severity: dsl.SeverityWarning,
			NodeID:   node.ID,
			Message:  "structured_output is not implemented; the node returns raw text",
		})
	}
	if cfg.Context.Enabled {
		problems = append(problems, dsl.Problem{
			Severity: dsl.SeverityWarning,
			NodeID:   node.ID,
			Message:  "context (knowledge retrieval) is not implemented; no context is injected",
		})
	}
	return problems
}

func (LLM) Run(ctx context.Context, req Request) (Output, error) {
	var cfg llmConfig
	if err := req.Node.Data.Decode(&cfg); err != nil {
		return Output{}, nodeError(req.Node, "invalid configuration: %v", err)
	}
	client, err := resolveModelConfig(req.Node, cfg, req.Model)
	if err != nil {
		return Output{}, err
	}

	messages := make([]llmclient.Message, 0, len(cfg.PromptTemplate))
	for index, message := range cfg.PromptTemplate {
		text, err := req.Render(message.Text)
		if err != nil {
			return Output{}, nodeError(req.Node, "prompt_template[%d]: %v", index, err)
		}
		role := message.Role
		if role == "" {
			role = "user"
		}
		messages = append(messages, llmclient.Message{Role: role, Content: text})
	}
	if len(messages) == 0 {
		return Output{}, nodeError(req.Node, "prompt_template must contain at least one message")
	}

	req.Log("calling %s model %s", providerLabel(cfg.Model.Provider), client.Model)
	result, err := llmclient.Chat(ctx, client, messages)
	if err != nil {
		return Output{}, nodeError(req.Node, "%v", err)
	}
	fields := map[string]any{"text": result.Text}
	if len(result.Usage) > 0 {
		fields["usage"] = result.Usage
	}
	return Output{Fields: fields}, nil
}

const (
	providerOpenAI   = "openai"
	providerDeepSeek = "deepseek"
	providerOllama   = "ollama"
)

func providers() []string {
	return []string{providerOpenAI, providerDeepSeek, providerOllama}
}

// ProviderNames returns the supported llm providers.
func ProviderNames() []string { return providers() }

// ProviderDefaults returns the default endpoint and API-key environment
// variable for a provider name.
func ProviderDefaults(provider string) (endpoint, keyEnv string, ok bool) {
	name := providerOf(provider)
	if name == "" {
		return "", "", false
	}
	return providerEndpoint(name), providerKeyEnv(name), true
}

// providerOf maps a Dify provider identifier such as
// "langgenius/openai/openai" onto a supported provider name.
func providerOf(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return ""
	}
	for _, candidate := range providers() {
		if strings.Contains(normalized, candidate) {
			return candidate
		}
	}
	return ""
}

func providerLabel(provider string) string {
	if name := providerOf(provider); name != "" {
		return name
	}
	if strings.TrimSpace(provider) == "" {
		return providerOpenAI
	}
	return provider
}

// resolveModelConfig merges the DSL model block with CLI overrides. CLI flags
// win, because they exist precisely to retarget a workflow at another endpoint.
func resolveModelConfig(node dsl.Node, cfg llmConfig, override *llmclient.Config) (llmclient.Config, error) {
	provider := providerOf(cfg.Model.Provider)
	if provider == "" && strings.TrimSpace(cfg.Model.Provider) != "" {
		return llmclient.Config{}, nodeError(node, "provider %q is not supported; use one of %s",
			cfg.Model.Provider, strings.Join(providers(), ", "))
	}
	if provider == "" {
		provider = providerOpenAI
	}

	client := llmclient.Config{
		Endpoint: providerEndpoint(provider),
		APIKey:   os.Getenv(providerKeyEnv(provider)),
		Model:    strings.TrimSpace(cfg.Model.Name),
	}
	if provider == providerOllama && client.APIKey == "" {
		client.APIKey = providerOllama
	}
	if endpoint := strings.TrimSpace(cfg.Model.Endpoint); endpoint != "" {
		client.Endpoint = endpoint
	}

	for key, value := range cfg.Model.CompletionParams {
		switch key {
		case "temperature":
			if temperature, ok := toFloat(value); ok {
				client.Temperature = &temperature
			}
		case "max_tokens":
			if tokens, ok := toInt(value); ok {
				client.MaxOutputTokens = tokens
			}
		default:
			if client.ExtraFields == nil {
				client.ExtraFields = map[string]any{}
			}
			client.ExtraFields[key] = value
		}
	}

	if override != nil {
		if override.Endpoint != "" {
			client.Endpoint = override.Endpoint
		}
		if override.APIKey != "" {
			client.APIKey = override.APIKey
		}
		if override.Model != "" {
			client.Model = override.Model
		}
		if override.Temperature != nil {
			client.Temperature = override.Temperature
		}
		if override.MaxOutputTokens > 0 {
			client.MaxOutputTokens = override.MaxOutputTokens
		}
		if len(override.ExtraFields) > 0 {
			if client.ExtraFields == nil {
				client.ExtraFields = map[string]any{}
			}
			for key, value := range override.ExtraFields {
				client.ExtraFields[key] = value
			}
		}
	}
	if override != nil && override.HTTPClient != nil {
		client.HTTPClient = override.HTTPClient
	}
	if strings.TrimSpace(client.Model) == "" {
		return llmclient.Config{}, nodeError(node, "no model configured; set model.name in the DSL or pass --model")
	}
	if strings.TrimSpace(client.APIKey) == "" {
		return llmclient.Config{}, nodeError(node, "no API key for provider %s; set %s or pass --api-key",
			provider, providerKeyEnv(provider))
	}
	return client, nil
}

func providerEndpoint(provider string) string {
	switch provider {
	case providerDeepSeek:
		return "https://api.deepseek.com/chat/completions"
	case providerOllama:
		return "http://localhost:11434/v1/chat/completions"
	default:
		return "https://api.openai.com/v1/chat/completions"
	}
}

func providerKeyEnv(provider string) string {
	switch provider {
	case providerDeepSeek:
		return "DEEPSEEK_API_KEY"
	case providerOllama:
		return "OLLAMA_API_KEY"
	default:
		return "OPENAI_API_KEY"
	}
}

func toFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}
