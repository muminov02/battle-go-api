package handler

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"battle-go-api/internal/auth"
)

// responseCapture buffers a handler's response so EnvelopeMiddleware can rewrap it.
type responseCapture struct {
	gin.ResponseWriter
	buf    *bytes.Buffer
	status int
	wrote  bool
}

func (w *responseCapture) WriteHeader(code int)        { w.status = code }
func (w *responseCapture) WriteHeaderNow()             {}
func (w *responseCapture) Write(b []byte) (int, error) { w.wrote = true; return w.buf.Write(b) }
func (w *responseCapture) WriteString(s string) (int, error) {
	w.wrote = true
	return w.buf.WriteString(s)
}
func (w *responseCapture) Status() int   { return w.status }
func (w *responseCapture) Written() bool { return w.wrote }
func (w *responseCapture) Size() int     { return w.buf.Len() }

// EnvelopeMiddleware wraps every JSON response in the PHP StructuredApiController shape:
//
//	200 → {ok:true,  status_code:200, description:"Success",          result:<body>}
//	422 → {ok:false, status_code:422, description:"Validation error", errors:<body>}
//	else→ {ok:false, status_code:n,   description:"<body.message>"}
func EnvelopeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// WebSocket upgrades hijack the connection — never buffer/rewrap them.
		if c.IsWebsocket() {
			c.Next()
			return
		}
		cap := &responseCapture{ResponseWriter: c.Writer, buf: &bytes.Buffer{}, status: http.StatusOK}
		c.Writer = cap
		c.Next()

		var data interface{}
		if cap.buf.Len() > 0 {
			if err := json.Unmarshal(cap.buf.Bytes(), &data); err != nil {
				data = cap.buf.String()
			}
		}

		var env gin.H
		switch {
		case cap.status == http.StatusOK:
			env = gin.H{"ok": true, "status_code": 200, "description": "Success", "result": data}
		case cap.status == http.StatusUnprocessableEntity:
			env = gin.H{"ok": false, "status_code": 422, "description": "Validation error", "errors": data}
		default:
			desc := http.StatusText(cap.status)
			if m, ok := data.(map[string]interface{}); ok {
				if msg, ok := m["message"].(string); ok && msg != "" {
					desc = msg
				}
			}
			env = gin.H{"ok": false, "status_code": cap.status, "description": desc}
		}

		out, _ := json.Marshal(env)
		cap.ResponseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
		cap.ResponseWriter.WriteHeader(cap.status)
		cap.ResponseWriter.Write(out)
	}
}

const userIDKey = "user_id"

// CORSMiddleware allows browser clients (test frontend) to call the API.
// Permissive (*) — intended for development/testing. Aborts OPTIONS preflight
// before auth runs so the browser never gets a 401 on preflight.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// AuthMiddleware validates Bearer JWT and sets user_id in Gin context.
func AuthMiddleware(publicKey *rsa.PublicKey) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "missing authorization header"})
			return
		}

		token := strings.TrimPrefix(header, "Bearer ")
		userID, err := auth.ValidateToken(token, publicKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": err.Error()})
			return
		}

		c.Set(userIDKey, userID)
		c.Next()
	}
}

func currentUserID(c *gin.Context) int {
	return c.MustGet(userIDKey).(int)
}
