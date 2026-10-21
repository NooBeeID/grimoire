package base

import (
	"encoding/json"
	"testing"
)

func TestParseMessage(t *testing.T) {
	cases := []struct {
		name, body string
		kind       Kind
		code       int
	}{
		{"parse-error", `{`, KindParseError, CodeParseError},
		{"empty", ``, KindParseError, CodeParseError},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, KindBatch, CodeInvalidRequest},
		{"scalar", `42`, KindInvalid, CodeInvalidRequest},
		{"client-response", `{"jsonrpc":"2.0","id":1,"result":{}}`, KindResponse, 0},
		{"client-error", `{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":"x"}}`, KindResponse, 0},
		{"notification", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, KindNotification, 0},
		{"request-string-id", `{"jsonrpc":"2.0","id":"a","method":"ping"}`, KindRequest, 0},
		{"request-number-id", `{"jsonrpc":"2.0","id":7,"method":"ping","params":{}}`, KindRequest, 0},
		{"id-null", `{"jsonrpc":"2.0","id":null,"method":"ping"}`, KindInvalid, CodeInvalidRequest},
		{"id-bool", `{"jsonrpc":"2.0","id":true,"method":"ping"}`, KindInvalid, CodeInvalidRequest},
		{"id-object", `{"jsonrpc":"2.0","id":{},"method":"ping"}`, KindInvalid, CodeInvalidRequest},
		{"wrong-version", `{"jsonrpc":"1.0","id":1,"method":"ping"}`, KindInvalid, CodeInvalidRequest},
		{"no-version", `{"id":1,"method":"ping"}`, KindInvalid, CodeInvalidRequest},
		{"empty-method", `{"jsonrpc":"2.0","id":1,"method":""}`, KindInvalid, CodeInvalidRequest},
		{"method-not-string", `{"jsonrpc":"2.0","id":1,"method":5}`, KindInvalid, CodeInvalidRequest},
		{"params-array", `{"jsonrpc":"2.0","id":1,"method":"ping","params":[]}`, KindInvalid, CodeInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, req, e := ParseMessage([]byte(tc.body))
			if k != tc.kind {
				t.Fatalf("kind=%d want %d", k, tc.kind)
			}
			if tc.code != 0 && (e == nil || e.Code != tc.code) {
				t.Fatalf("err=%+v", e)
			}
			if tc.code == 0 && e != nil {
				t.Fatalf("err=%+v", e)
			}
			if k == KindRequest && len(req.ID) == 0 {
				t.Fatal("id kosong")
			}
		})
	}
}

func TestJSONRPCBuilders(t *testing.T) {
	r, err := Success(json.RawMessage(`"x"`), struct{}{})
	if err != nil || string(r.Result) != `{}` || string(r.ID) != `"x"` {
		t.Fatalf("%+v %v", r, err)
	}
	b, _ := json.Marshal(HTTPReject(CodeUnauthorized, "Unauthorized", nil))
	if string(b) != `{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}` {
		t.Fatal(string(b))
	}
	f := Failure(json.RawMessage(`5`), &Error{Code: CodeMethodNotFound, Message: "Method not found"})
	if string(f.ID) != "5" || f.Error.Code != CodeMethodNotFound {
		t.Fatalf("%+v", f)
	}
}
