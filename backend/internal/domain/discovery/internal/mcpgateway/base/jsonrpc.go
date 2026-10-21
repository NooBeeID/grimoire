package base

import (
	"bytes"
	"encoding/json"
)

// Kode error JSON-RPC dan kode khusus gateway.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
	CodeUnauthorized   = -32001
	CodeForbidden      = -32002 // cadangan; 403 Origin memakai OriginGuard
	CodeRateLimited    = -32003
	CodeUnavailable    = -32004
)

// Request adalah pesan JSON-RPC masuk.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Error adalah objek error JSON-RPC.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Response adalah pesan JSON-RPC keluar.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Kind mengklasifikasikan pesan masuk.
type Kind int

const (
	KindRequest Kind = iota + 1
	KindNotification
	KindResponse
	KindBatch
	KindInvalid
	KindParseError
)

var nullID = json.RawMessage("null")

// ParseMessage mengklasifikasikan body. Error non-nil menyertai Kind selain Request/Notification/Response.
func ParseMessage(body []byte) (Kind, Request, *Error) {
	trimmed := bytes.TrimSpace(body)
	if !json.Valid(trimmed) {
		return KindParseError, Request{}, &Error{Code: CodeParseError, Message: "Parse error"}
	}
	invalid := func(reason string) (Kind, Request, *Error) {
		return KindInvalid, Request{}, &Error{Code: CodeInvalidRequest, Message: "Invalid Request", Data: map[string]string{"reason": reason}}
	}
	if trimmed[0] == '[' {
		return KindBatch, Request{}, &Error{Code: CodeInvalidRequest, Message: "Invalid Request", Data: map[string]string{"reason": "batch not supported"}}
	}
	if trimmed[0] != '{' {
		return invalid("message must be an object")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return invalid("message must be an object")
	}
	_, hasMethod := m["method"]
	_, hasResult := m["result"]
	_, hasError := m["error"]
	if !hasMethod && (hasResult || hasError) {
		return KindResponse, Request{}, nil
	}
	var req Request
	var ver string
	if json.Unmarshal(m["jsonrpc"], &ver) != nil || ver != "2.0" {
		return invalid("jsonrpc must be \"2.0\"")
	}
	req.JSONRPC = ver
	if json.Unmarshal(m["method"], &req.Method) != nil || req.Method == "" {
		return invalid("method must be a non-empty string")
	}
	if p, ok := m["params"]; ok {
		if !isObject(p) {
			return invalid("params must be an object")
		}
		req.Params = p
	}
	id, hasID := m["id"]
	if !hasID {
		return KindNotification, req, nil
	}
	id = bytes.TrimSpace(id)
	if len(id) == 0 || !(id[0] == '"' || id[0] == '-' || (id[0] >= '0' && id[0] <= '9')) {
		return invalid("id must be a string or number")
	}
	req.ID = id
	return KindRequest, req, nil
}

func isObject(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '{'
}

// Success membangun respons sukses; result di-marshal ke RawMessage.
func Success(id json.RawMessage, result any) (Response, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return Response{}, err
	}
	return Response{JSONRPC: "2.0", ID: orNull(id), Result: b}, nil
}

// Failure membangun respons error dengan id yang dipertahankan.
func Failure(id json.RawMessage, e *Error) Response {
	return Response{JSONRPC: "2.0", ID: orNull(id), Error: e}
}

// HTTPReject membangun respons error untuk penolakan level HTTP (id:null).
func HTTPReject(code int, msg string, data any) Response {
	return Failure(nil, &Error{Code: code, Message: msg, Data: data})
}

func orNull(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return nullID
	}
	return id
}
