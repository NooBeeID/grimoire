package endpoint

import (
	"bytes"
	"encoding/json"

	"grimoire/internal/domain/discovery/internal/mcpgateway/base"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
)

const maxNameBytes = 128

type initializeParams struct {
	ProtocolVersion string
	ClientName      string
	ClientVersion   string
}

type toolsCallParams struct {
	Name      string
	Arguments json.RawMessage // "{}" bila tidak ada
}

// paramsError menjadi -32602 Invalid params dengan data {reason}.
type paramsError struct{ Reason string }

func (e *paramsError) Error() string { return e.Reason }

func isJSONObject(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '{'
}

// decodeString hanya berhasil untuk literal string JSON (null ditolak).
func decodeString(raw json.RawMessage, dst *string) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '"' && json.Unmarshal(t, dst) == nil
}

func parseInitializeParams(raw json.RawMessage) (initializeParams, *paramsError) {
	var out initializeParams
	if !isJSONObject(raw) {
		return out, &paramsError{"params must be an object"}
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return out, &paramsError{"params must be an object"}
	}
	if !decodeString(m["protocolVersion"], &out.ProtocolVersion) {
		return initializeParams{}, &paramsError{"protocolVersion must be a string"}
	}
	if ci, ok := m["clientInfo"]; ok && isJSONObject(ci) {
		var info map[string]json.RawMessage
		if json.Unmarshal(ci, &info) == nil {
			var s string
			if decodeString(info["name"], &s) {
				out.ClientName = base.TruncateName(s, maxNameBytes)
			}
			s = ""
			if decodeString(info["version"], &s) {
				out.ClientVersion = base.TruncateName(s, maxNameBytes)
			}
		}
	}
	return out, nil
}

func parseToolsCallParams(raw json.RawMessage) (toolsCallParams, *paramsError) {
	var out toolsCallParams
	if !isJSONObject(raw) {
		return out, &paramsError{"params must be an object"}
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return out, &paramsError{"params must be an object"}
	}
	if !decodeString(m["name"], &out.Name) {
		return out, &paramsError{"name must be a string"}
	}
	if out.Name == "" {
		return out, &paramsError{"name must not be empty"}
	}
	if len(out.Name) > maxNameBytes {
		return out, &paramsError{"name too long (max 128 bytes)"}
	}
	args, ok := m["arguments"]
	switch {
	case !ok:
		out.Arguments = json.RawMessage("{}")
	case isJSONObject(args):
		out.Arguments = args
	default:
		return out, &paramsError{"arguments must be an object"}
	}
	return out, nil
}

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
	Instructions    string       `json:"instructions"`
}

type capabilities struct {
	Tools toolsCapability `json:"tools"`
}

type toolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolEntry struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations toolAnnotations `json:"annotations"`
}

type toolAnnotations struct {
	ReadOnlyHint bool `json:"readOnlyHint"`
}

type toolsListResult struct {
	Tools []toolEntry `json:"tools"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callToolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError"`
	Meta    *callToolMeta `json:"_meta,omitempty"`
}

type callToolMeta struct {
	Truncated     bool `json:"truncated"`
	OriginalBytes int  `json:"original_bytes"`
}

// toToolEntries memaksa readOnlyHint=true; slice hasil selalu non-nil.
func toToolEntries(tools []toolkit.Tool) []toolEntry {
	out := make([]toolEntry, 0, len(tools))
	for _, t := range tools {
		out = append(out, toolEntry{
			Name: t.Name, Title: t.Title, Description: t.Description,
			InputSchema: t.InputSchema, Annotations: toolAnnotations{ReadOnlyHint: true},
		})
	}
	return out
}

func toCallToolResult(o base.Outcome) callToolResult {
	r := callToolResult{
		Content: []contentItem{{Type: "text", Text: o.Result.Text}},
		IsError: o.Result.IsError || o.ErrorCode != "",
	}
	if o.Truncated {
		r.Meta = &callToolMeta{Truncated: true, OriginalBytes: o.OriginalBytes}
	}
	return r
}
