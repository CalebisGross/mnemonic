package llm

// Pricing holds per-token costs in USD for a model.
type Pricing struct {
	InputPerToken  float64
	OutputPerToken float64
}

// modelPricing contains known model pricing (as of March 2026).
var modelPricing = map[string]Pricing{
	"gemini-2.5-flash":              {InputPerToken: 0.15e-6, OutputPerToken: 0.60e-6},
	"gemini-2.5-pro":                {InputPerToken: 1.25e-6, OutputPerToken: 10.0e-6},
	"gemini-2.0-flash":              {InputPerToken: 0.10e-6, OutputPerToken: 0.40e-6},
	"gemini-3-flash-preview":        {InputPerToken: 0.50e-6, OutputPerToken: 3.0e-6},
	"gemini-3.1-flash-lite-preview": {InputPerToken: 0.25e-6, OutputPerToken: 1.50e-6},
	"gemini-embedding-2-preview":    {InputPerToken: 0.00625e-3, OutputPerToken: 0},
	"text-embedding-004":            {InputPerToken: 0.00625e-3, OutputPerToken: 0},
}

// EstimateCost returns the estimated cost in USD for given token counts.
// If customInputPrice or customOutputPrice are > 0, they override the
// built-in pricing (expressed as USD per 1M tokens).
func EstimateCost(model string, promptTokens, completionTokens int, customInputPrice, customOutputPrice float64) float64 {
	var inputRate, outputRate float64

	if customInputPrice > 0 {
		inputRate = customInputPrice / 1e6
	} else if p, ok := modelPricing[model]; ok {
		inputRate = p.InputPerToken
	}

	if customOutputPrice > 0 {
		outputRate = customOutputPrice / 1e6
	} else if p, ok := modelPricing[model]; ok {
		outputRate = p.OutputPerToken
	}

	return float64(promptTokens)*inputRate + float64(completionTokens)*outputRate
}
