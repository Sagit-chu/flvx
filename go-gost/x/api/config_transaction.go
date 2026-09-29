package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-gost/x/config"
)

const maxConfigRequestBody = 16 << 20

// Read the complete bounded request before acquiring the runtime transaction
// lock. Buffer the response until after it is released: neither a slow upload
// nor a client that stops reading may block panel commands, reload or shutdown.
func configTransaction() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if ctx.Request.Body != nil {
			body := http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxConfigRequestBody)
			data, err := io.ReadAll(body)
			body.Close()
			if err != nil {
				status := http.StatusBadRequest
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					status = http.StatusRequestEntityTooLarge
				}
				ctx.AbortWithStatusJSON(status, Response{Code: status, Msg: "Unable to read configuration request"})
				return
			}
			ctx.Request.Body = io.NopCloser(bytes.NewReader(data))
		}

		writer := ctx.Writer
		buffered := &configResponseWriter{ResponseWriter: writer, header: writer.Header().Clone(), status: http.StatusOK, size: -1}
		ctx.Writer = buffered
		defer func() { ctx.Writer = writer }()
		func() {
			unlock := config.LockMutation()
			defer unlock()
			// A request waiting behind reload may have been closed during shutdown.
			if ctx.Request.Context().Err() != nil {
				ctx.Abort()
				return
			}
			ctx.Next()
		}()
		ctx.Writer = writer
		for key, values := range buffered.header {
			writer.Header()[key] = values
		}
		writer.WriteHeader(buffered.status)
		writer.Write(buffered.body.Bytes())
	}
}

// Config endpoints return JSON rather than streaming. Preserve Gin's response
// bookkeeping while delaying all network writes until the transaction ends.
type configResponseWriter struct {
	gin.ResponseWriter
	header http.Header
	body   bytes.Buffer
	status int
	size   int
}

func (w *configResponseWriter) Header() http.Header { return w.header }
func (w *configResponseWriter) WriteHeader(status int) {
	if !w.Written() && status > 0 {
		w.status = status
	}
}
func (w *configResponseWriter) WriteHeaderNow() {
	if !w.Written() {
		w.size = 0
	}
}
func (w *configResponseWriter) Write(p []byte) (int, error) {
	w.WriteHeaderNow()
	n, err := w.body.Write(p)
	w.size += n
	return n, err
}
func (w *configResponseWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *configResponseWriter) Status() int                       { return w.status }
func (w *configResponseWriter) Size() int                         { return w.size }
func (w *configResponseWriter) Written() bool                     { return w.size >= 0 }
func (w *configResponseWriter) Flush()                            { w.WriteHeaderNow() }
