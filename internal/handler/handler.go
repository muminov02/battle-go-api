package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"battle-go-api/internal/models"
	"battle-go-api/internal/service"
)

// TokenProvider can create Ably tokens for battle channels.
type TokenProvider interface {
	CreateBattleToken(ctx context.Context, battleUUID string, studentID int) (string, error)
}

// ProfileProvider builds the public student profile shown in member objects.
type ProfileProvider interface {
	GetPublicProfile(ctx context.Context, studentID int) (*models.PublicProfile, error)
}

// RealtimeInfo tells the client which transport is active and how to connect.
type RealtimeInfo interface {
	Driver() string                 // "ably" | "ws"
	ConnectURL(battleUUID string) string // ws URL for ws driver; "" for ably
}

// Handler holds all service dependencies for HTTP handling.
type Handler struct {
	find       *service.FindService
	confirm    *service.ConfirmService
	answer     *service.AnswerService
	leave      *service.LeaveService
	changeType *service.ChangeTypeService
	endBattle  *service.EndBattleService
	view       *service.ViewService
	tokens     TokenProvider
	profiles   ProfileProvider
	realtime   RealtimeInfo
}

func New(
	find *service.FindService,
	confirm *service.ConfirmService,
	answer *service.AnswerService,
	leave *service.LeaveService,
	changeType *service.ChangeTypeService,
	endBattle *service.EndBattleService,
	view *service.ViewService,
	tokens TokenProvider,
	profiles ProfileProvider,
	realtime RealtimeInfo,
) *Handler {
	return &Handler{find, confirm, answer, leave, changeType, endBattle, view, tokens, profiles, realtime}
}

// --- POST /student/v1/battle/find ---

type findRequest struct {
	Type      int `json:"type" binding:"required"`
	LobbyType int `json:"lobby_type" binding:"required"`
}

func (h *Handler) Find(c *gin.Context) {
	var req findRequest
	if !bind(c, &req) {
		return
	}

	res, err := h.find.Execute(c.Request.Context(), currentUserID(c), req.Type, req.LobbyType)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	tok := h.battleToken(c, res.Battle, currentUserID(c))
	resp := gin.H{"model": h.battleJSON(c.Request.Context(), res.Battle, res.Members, tok)}
	if res.Message != "" {
		resp["message"] = res.Message
	}
	c.JSON(http.StatusOK, resp)
}

// --- POST /student/v1/battle/confirm ---

type confirmRequest struct {
	BattleID string `json:"battle_id" binding:"required"`
}

func (h *Handler) Confirm(c *gin.Context) {
	var req confirmRequest
	if !bind(c, &req) {
		return
	}

	res, err := h.confirm.Execute(c.Request.Context(), currentUserID(c), req.BattleID)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	tok := h.battleToken(c, res.Battle, currentUserID(c))
	resp := gin.H{"model": h.battleJSON(c.Request.Context(), res.Battle, res.Members, tok)}
	if res.Message != "" {
		resp["message"] = res.Message
	}
	c.JSON(http.StatusOK, resp)
}

// --- POST /student/v1/battle/answer ---

type answerRequest struct {
	BattleID   string   `json:"battle_id" binding:"required"`
	QuestionID int      `json:"question_id" binding:"required"`
	Values     []string `json:"values" binding:"required"`
	AnswerTime int      `json:"answer_time" binding:"required"`
}

func (h *Handler) Answer(c *gin.Context) {
	var req answerRequest
	if !bind(c, &req) {
		return
	}

	res, err := h.answer.Execute(c.Request.Context(), currentUserID(c), req.BattleID, req.QuestionID, req.Values, req.AnswerTime)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": res.Status, "message": res.Message})
}

// --- POST /student/v1/battle/leave ---

type leaveRequest struct {
	BattleID string `json:"battle_id" binding:"required"`
}

func (h *Handler) Leave(c *gin.Context) {
	var req leaveRequest
	if !bind(c, &req) {
		return
	}

	if err := h.leave.Execute(c.Request.Context(), currentUserID(c), req.BattleID); err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "You have left battle"})
}

// --- POST /student/v1/battle/change-type ---

type changeTypeRequest struct {
	BattleID string `json:"battle_id" binding:"required"`
}

func (h *Handler) ChangeType(c *gin.Context) {
	var req changeTypeRequest
	if !bind(c, &req) {
		return
	}

	res, err := h.changeType.Execute(c.Request.Context(), currentUserID(c), req.BattleID)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	tok := h.battleToken(c, res.Battle, currentUserID(c))
	c.JSON(http.StatusOK, gin.H{
		"message": res.Message,
		"model":   h.battleJSON(c.Request.Context(), res.Battle, res.Members, tok),
	})
}

// --- GET /student/v1/battle/:uuid ---

func (h *Handler) View(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "uuid required"})
		return
	}

	res, err := h.view.Execute(c.Request.Context(), uuid)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	tok := h.battleToken(c, res.Battle, currentUserID(c))
	c.JSON(http.StatusOK, h.battleJSON(c.Request.Context(), res.Battle, res.Members, tok))
}

// --- GET /student/v1/battle/:uuid/questions ---
// Client fetches questions here once the battle is ON_GOING (Ably no longer carries them).
// Members only.
func (h *Handler) Questions(c *gin.Context) {
	uuid := c.Param("uuid")
	res, err := h.view.Execute(c.Request.Context(), uuid)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	sid := currentUserID(c)
	isMember := false
	for _, m := range res.Members {
		if m.StudentID == sid {
			isMember = true
			break
		}
	}
	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"message": service.ErrNotMember.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"uuid":      res.Battle.UUID,
		"status":    res.Battle.Status,
		"questions": res.Battle.Questions,
	})
}

// --- shared helpers ---

func bind(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			field := ve[0]
			msg := fmt.Sprintf("%s is required", toSnakeCase(field.Field()))
			c.JSON(http.StatusUnprocessableEntity, gin.H{"message": msg})
			return false
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return false
	}
	return true
}

func toSnakeCase(s string) string {
	result := make([]byte, 0, len(s)+4)
	for i := 0; i < len(s); i++ {
		if i > 0 && s[i] >= 'A' && s[i] <= 'Z' {
			prevLower := s[i-1] >= 'a' && s[i-1] <= 'z'
			nextLower := i+1 < len(s) && s[i+1] >= 'a' && s[i+1] <= 'z'
			if prevLower || nextLower {
				result = append(result, '_')
			}
		}
		result = append(result, s[i]|0x20)
	}
	return string(result)
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrBattleNotFound), errors.Is(err, service.ErrStudentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
	case errors.Is(err, service.ErrNotMember):
		c.JSON(http.StatusForbidden, gin.H{"message": err.Error()})
	case errors.Is(err, service.ErrBattleFinished),
		errors.Is(err, service.ErrBattleNotStarted),
		errors.Is(err, service.ErrStudentNoLevel),
		errors.Is(err, service.ErrDemoLimitExceeded):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
	default:
		log.Printf("internal error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
	}
}

func (h *Handler) battleToken(c *gin.Context, b *models.Battle, studentID int) string {
	tok, err := h.tokens.CreateBattleToken(c.Request.Context(), b.UUID, studentID)
	if err != nil {
		return "" // non-fatal — client just won't have token
	}
	return tok
}

// battleJSON builds the response shape matching PHP BattleResource:
// { uuid, status, members:[{place, points, student:{…profile…}, answers}], token }.
// Members are ordered by place ASC (PHP getBattleMembers orderBy place).
// answers is the raw JSON string of the answers array (PHP returns the column string).
func (h *Handler) battleJSON(ctx context.Context, b *models.Battle, members []*models.BattleMember, token string) gin.H {
	ordered := append([]*models.BattleMember(nil), members...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return placeOrder(ordered[i].Place) < placeOrder(ordered[j].Place)
	})

	mems := make([]gin.H, len(ordered))
	for i, m := range ordered {
		// PHP returns the raw answers column: null until answered, then a JSON string.
		var answers interface{}
		if len(m.Answers) > 0 {
			b, _ := json.Marshal(m.Answers)
			answers = string(b)
		}
		var student *models.PublicProfile
		if p, err := h.profiles.GetPublicProfile(ctx, m.StudentID); err == nil {
			student = p
		}
		mems[i] = gin.H{
			"place":      m.Place,
			"points":     m.Points,
			"student":    student,
			"answers":    answers,
			"student_id": m.StudentID,
			"status":     m.Status,
		}
	}
	return gin.H{
		"uuid":    b.UUID,
		"status":  b.Status,
		"members": mems,
		"token":   token,
		// Additive, backward-compatible: tells the client which realtime transport
		// is active and (for ws) the connect URL. Old Ably clients can ignore it.
		"realtime": gin.H{
			"driver": h.realtime.Driver(),
			"url":    realtimeURL(h.realtime.ConnectURL(b.UUID)),
		},
	}
}

// realtimeURL returns nil for an empty URL so JSON shows null (ably), else the string.
func realtimeURL(u string) interface{} {
	if u == "" {
		return nil
	}
	return u
}

// placeOrder sorts members with a place before those without (nil place last).
func placeOrder(p *int) int {
	if p == nil {
		return 1 << 30
	}
	return *p
}
