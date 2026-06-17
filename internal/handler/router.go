package handler

import (
	"crypto/rsa"
	"net/http"

	"github.com/gin-gonic/gin"
)

// NewRouter creates the Gin router with all battle routes registered.
// wsHandler is non-nil only when REALTIME_DRIVER=ws; it serves the WebSocket
// endpoint (authenticated via ?token=, so it sits outside the Bearer group).
func NewRouter(h *Handler, jwtPublicKey *rsa.PublicKey, wsHandler http.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), CORSMiddleware(), EnvelopeMiddleware())

	if wsHandler != nil {
		r.GET("/student/v1/battle/ws", gin.WrapH(wsHandler))
	}

	battle := r.Group("/student/v1/battle")
	battle.Use(AuthMiddleware(jwtPublicKey))

	battle.POST("/find", h.Find)
	battle.POST("/confirm", h.Confirm)
	battle.POST("/answer", h.Answer)
	battle.POST("/leave", h.Leave)
	battle.POST("/change-type", h.ChangeType)
	battle.GET("/:uuid", h.View)
	battle.GET("/:uuid/questions", h.Questions)

	return r
}
