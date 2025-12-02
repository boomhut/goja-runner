package jsrunner

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type spyTransport struct {
	called bool
}

func (s *spyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.called = true
	return http.DefaultTransport.RoundTrip(req)
}

func TestFetchHelpers(t *testing.T) {
	textServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/text":
			fmt.Fprint(w, "hello world")
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"name":"goja","ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer textServer.Close()

	runner := New(WithWebAccess(&WebAccessConfig{Timeout: time.Second}))

	result, err := runner.Call("fetchText", textServer.URL+"/text")
	if err != nil {
		t.Fatalf("fetchText failed: %v", err)
	}
	if ExportString(result) != "hello world" {
		t.Fatalf("unexpected fetchText result: %s", ExportString(result))
	}

	jsonResult, err := runner.Call("fetchJSON", textServer.URL+"/json")
	if err != nil {
		t.Fatalf("fetchJSON failed: %v", err)
	}
	obj, ok := Export(jsonResult).(map[string]interface{})
	if !ok {
		t.Fatalf("fetchJSON returned %T, want map", jsonResult)
	}
	if obj["name"] != "goja" {
		t.Fatalf("expected name=goja, got %v", obj["name"])
	}
}

func TestCustomHTTPClientIsUsed(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	spy := &spyTransport{}
	client := &http.Client{Transport: spy, Timeout: 2 * time.Second}
	runner := New(WithWebAccess(&WebAccessConfig{Client: client, Timeout: time.Second}))

	if runner.httpClient != client {
		t.Fatalf("expected http client to be the custom client")
	}

	if _, err := runner.Call("fetchText", server.URL); err != nil {
		t.Fatalf("fetchText failed: %v", err)
	}
	if !spy.called {
		t.Fatalf("custom transport was never called")
	}
}
