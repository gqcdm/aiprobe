package providers

import (
	"github.com/gqcdm/aiprobe/internal/detect"
	"github.com/gqcdm/aiprobe/internal/providers/anthropic"
	"github.com/gqcdm/aiprobe/internal/providers/gemini"
	"github.com/gqcdm/aiprobe/internal/providers/openai"
	"github.com/gqcdm/aiprobe/internal/schema"
)

func All() []detect.Adapter {
	return []detect.Adapter{
		openai.New(),
		anthropic.New(),
		gemini.New(),
	}
}

func ByProvider(provider schema.Provider) detect.Adapter {
	for _, adapter := range All() {
		if adapter.Name() == provider {
			return adapter
		}
	}
	return nil
}
