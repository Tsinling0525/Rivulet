package workflow

import (
	"time"

	"github.com/Tsinling0525/rivulet/dsl"
	"gopkg.in/yaml.v3"
)

// Dify error_strategy values. remove-abnormal-output behaves like
// continue-on-error here: the run continues and the node contributes no values.
const (
	errorStrategyTerminated = "terminated"
	errorStrategyContinue   = "continue-on-error"
	errorStrategyRemove     = "remove-abnormal-output"
)

const maxRetriesLimit = 10

type retryConfig struct {
	Enabled       bool `yaml:"enabled"`
	MaxRetries    int  `yaml:"max_retries"`
	RetryInterval int  `yaml:"retry_interval"`
	Exponential   struct {
		Enabled     bool    `yaml:"enabled"`
		Multiplier  float64 `yaml:"multiplier"`
		MaxInterval int     `yaml:"max_interval"`
	} `yaml:"exponential_backoff"`
}

type retryPolicy struct {
	maxRetries int
	interval   time.Duration
	multiplier float64
	maxDelay   time.Duration
}

func (p retryPolicy) delay(attempt int) time.Duration {
	if p.interval <= 0 {
		return 0
	}
	delay := p.interval
	if p.multiplier > 1 {
		for i := 1; i < attempt; i++ {
			delay = time.Duration(float64(delay) * p.multiplier)
		}
	}
	if p.maxDelay > 0 && delay > p.maxDelay {
		delay = p.maxDelay
	}
	return delay
}

func retryPolicyOf(node dsl.Node) retryPolicy {
	raw, ok := node.Data.Extra["retry_config"]
	if !ok || raw == nil {
		return retryPolicy{}
	}
	encoded, err := yaml.Marshal(raw)
	if err != nil {
		return retryPolicy{}
	}
	var cfg retryConfig
	if err := yaml.Unmarshal(encoded, &cfg); err != nil || !cfg.Enabled {
		return retryPolicy{}
	}
	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries > maxRetriesLimit {
		maxRetries = maxRetriesLimit
	}
	policy := retryPolicy{
		maxRetries: maxRetries,
		interval:   time.Duration(cfg.RetryInterval) * time.Millisecond,
		multiplier: cfg.Exponential.Multiplier,
	}
	if cfg.Exponential.Enabled {
		if policy.multiplier <= 1 {
			policy.multiplier = 2
		}
		policy.maxDelay = time.Duration(cfg.Exponential.MaxInterval) * time.Millisecond
	}
	return policy
}

func errorStrategyOf(node dsl.Node) string {
	switch node.Data.StringField("error_strategy") {
	case errorStrategyContinue, errorStrategyRemove:
		return errorStrategyContinue
	default:
		return errorStrategyTerminated
	}
}

// failureDefaults returns the values downstream nodes see when a node failed
// under continue-on-error. The wildcard entry makes any reference to the failed
// node resolve to an empty string instead of aborting the run.
func failureDefaults(node dsl.Node) map[string]any {
	out := map[string]any{"*": ""}
	switch raw := node.Data.Extra["default_value"].(type) {
	case map[string]any:
		for key, value := range raw {
			out[key] = value
		}
	case []any:
		for _, item := range raw {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			key, _ := entry["key"].(string)
			if key == "" {
				continue
			}
			out[key] = entry["value"]
		}
	}
	return out
}
