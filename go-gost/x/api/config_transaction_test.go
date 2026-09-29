package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-gost/x/config"
)

func assertMutationAvailable(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() { unlock := config.LockMutation(); unlock(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("network I/O holds runtime mutation lock")
	}
}

func TestConfigTransactionDoesNotLockWhileReadingBody(t *testing.T) {
	router := gin.New()
	router.Use(configTransaction())
	router.POST("/config", func(c *gin.Context) { c.JSON(http.StatusOK, Response{Msg: "OK"}) })
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/config", reader)
	done := make(chan struct{})
	go func() { defer close(done); router.ServeHTTP(httptest.NewRecorder(), request) }()
	if _, err := writer.Write([]byte("{")); err != nil {
		t.Fatal(err)
	}
	assertMutationAvailable(t)
	writer.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish")
	}
}

type blockedResponse struct {
	header  http.Header
	started chan struct{}
	release chan struct{}
}

func (w *blockedResponse) Header() http.Header { return w.header }
func (w *blockedResponse) WriteHeader(int)     {}
func (w *blockedResponse) Write(p []byte) (int, error) {
	close(w.started)
	<-w.release
	return len(p), nil
}

func TestConfigTransactionReleasesLockBeforeSendingResponse(t *testing.T) {
	router := gin.New()
	router.Use(configTransaction())
	router.POST("/config", func(c *gin.Context) { c.JSON(http.StatusOK, Response{Msg: "OK"}) })
	writer := &blockedResponse{header: make(http.Header), started: make(chan struct{}), release: make(chan struct{})}
	defer close(writer.release)
	request := httptest.NewRequest(http.MethodPost, "/config", strings.NewReader("{}"))
	go router.ServeHTTP(writer, request)
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("response did not start")
	}
	assertMutationAvailable(t)
}
