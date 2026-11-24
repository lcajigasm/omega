package cli

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLocalModelsSlowServerIsUp(t *testing.T) {
	old := localProbeBudget
	localProbeBudget = 100 * time.Millisecond
	defer func() { localProbeBudget = old }()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	if ids, up := localModels(srv.URL + "/v1"); !up || ids != nil {
		t.Fatalf("slow server: ids=%v up=%v, want nil, true", ids, up)
	}
}

func TestLocalModelsAnswerWithinBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(800 * time.Millisecond) // over the old 700 ms budget
		w.Write([]byte(`{"data":[{"id":"a"},{"id":"b"}]}`))
	}))
	defer srv.Close()
	ids, up := localModels(srv.URL + "/v1")
	if !up || len(ids) != 2 {
		t.Fatalf("ids=%v up=%v, want [a b], true", ids, up)
	}
}

func TestLocalModelsDown(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	if _, up := localModels("http://" + addr + "/v1"); up {
		t.Fatal("nothing listening reported up")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	if _, up := localModels(srv.URL); up {
		t.Fatal("5xx reported up")
	}
}
