package analyze

import (
	_ "embed"
	"encoding/json"
)

// schema/cluster_analysis.json
var clusterAnalysisSchemaRaw []byte

// ClusterAnalysisSchema is the JSON schema sent to the LLM for structured output, it already includes the
// {name, strict, schema} envelope expected by OpenAI/Anthropic/Deepseek style (true for 04.2026)
// json_schema response_format.
var ClusterAnalysisSchema = json.RawMessage(mustValidJSON(clusterAnalysisSchemaRaw))

func mustValidJSON(b []byte) []byte {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		panic("analyze: embedded schema is not valid JSON: " + err.Error())
	}
	return b
}
