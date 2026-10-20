package middleware

import (
	"grimoire/internal/infra/http/respond"
	"net/http"
	"strings"
	"testing"
)

func TestAccessLog_FieldsAndNoQuery(t *testing.T) {
	log, buf := useLogger(t)
	h := TraceID()(AccessLog(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setLogUser(r.Context(), "u-1")
		w.WriteHeader(201)
		_, _ = w.Write([]byte("hello"))
	})))
	do(h, "POST", "/api/x?token=secret", map[string]string{"X-Forwarded-For": "1.2.3.4", respond.TraceHeader: "t-9"}, "")
	if strings.Contains(buf.String(), "secret") {
		t.Fatal("query bocor di log")
	}
	m := buf.find(t, "http request")
	if m == nil {
		t.Fatal("tidak ada log")
	}
	if m["method"] != "POST" || m["path"] != "/api/x" || m["status"] != float64(201) || m["bytes"] != float64(5) ||
		m["remote_ip"] != "10.0.0.5" || m["trace_id"] != "t-9" || m["user_id"] != "u-1" || m["level"] != "INFO" {
		t.Fatalf("%v", m)
	}
	if _, ok := m["duration_ms"].(float64); !ok {
		t.Fatal("duration_ms")
	}
}

func TestAccessLog_LevelByStatusAndHealth(t *testing.T) {
	log, buf := useLogger(t)
	status := 200
	h := AccessLog(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
	do(h, "GET", "/a", nil, "")
	status = 500
	do(h, "GET", "/b", nil, "")
	status = 200
	do(h, "GET", "/healthz", nil, "")
	lv := map[string]any{}
	for _, m := range buf.records(t) {
		lv[m["path"].(string)] = m["level"]
	}
	if lv["/a"] != "INFO" || lv["/b"] != "ERROR" || lv["/healthz"] != "DEBUG" {
		t.Fatalf("%v", lv)
	}
}

func TestAccessLog_PanicLoggedAs500(t *testing.T) {
	log, buf := useLogger(t)
	h := Recover(log)(AccessLog(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("x") })))
	w := do(h, "GET", "/p", nil, "")
	m := buf.find(t, "http request")
	if w.Code != 500 || m == nil || m["status"] != float64(500) {
		t.Fatalf("%d %v", w.Code, m)
	}
}
