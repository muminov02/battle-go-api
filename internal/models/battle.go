package models

import (
	"encoding/json"
	"time"
)

// Battle statuses
const (
	BattleStatusWaiting  = 100
	BattleStatusOnGoing  = 200
	BattleStatusFinished = 300
	BattleStatusOnQueue  = 400
)

// Battle types
const (
	BattleTypeP2P   = 100
	BattleTypeGroup = 200
	BattleTypeAI    = 300
)

// Lobby types
const (
	LobbyTypeGrammar    = 100
	LobbyTypeVocabulary = 200
)

// QuestionTimeSeconds is the per-question time budget (used for battle end_time
// calculation, the Ably question_time hint, and the per-member idle timeout = 2×).
const QuestionTimeSeconds = 15

// JsonQuestion types (json_question.type column). Mirrors PHP JsonQuestionTypeEnum.
// NOTE: differs from lobby type — VOCABULARY here is 110, not 200.
const (
	JsonQuestionTypeGrammar    = 100
	JsonQuestionTypeVocabulary = 110
)

// JsonQuestionTypeForLobby maps a battle lobby type to the json_question.type value.
func JsonQuestionTypeForLobby(lobbyType int) int {
	if lobbyType == LobbyTypeVocabulary {
		return JsonQuestionTypeVocabulary
	}
	return JsonQuestionTypeGrammar
}

// Member statuses
const (
	MemberStatusNotConfirmed = 100
	MemberStatusConfirmed    = 200
)

// Member types
const (
	MemberTypeCreator     = 100
	MemberTypeParticipant = 200
	MemberTypeAI          = 300
)

// BattleMemberCount maps battle type to required member count
var BattleMemberCount = map[int]int{
	BattleTypeP2P:   2,
	BattleTypeGroup: 4,
	BattleTypeAI:    2,
}

// Battle is the domain model for an active or finished battle
type Battle struct {
	ID           int
	UUID         string
	Type         int
	LobbyType    int
	LevelID      int
	LevelGroupID int
	CourseID     int // stored on battle for fast lobby matching without MySQL JOIN
	Status       int
	StartTime    *time.Time
	ExpireTime   *time.Time
	EndTime      *time.Time
	Questions    []Question
	CreatedAt    time.Time
}

// BattleMember is a participant in a battle
type BattleMember struct {
	ID              int
	StudentID       int
	BattleID        int
	Place           *int
	Points          *int
	Status          int
	CurrentQuestion int
	Answers         []Answer
	IsFinished      bool
	Type            int
	// LastQuestionAt is when CurrentQuestion last advanced (set on each accepted answer).
	// Nil until the first answer — idle check falls back to the battle start time.
	LastQuestionAt *time.Time
}

// Answer is a single answer submitted by a member
type Answer struct {
	QuestionID     int      `json:"question_id"`
	Question       string   `json:"question"`
	Values         []string `json:"values"`
	Time           int      `json:"time"` // milliseconds
	IsCorrect      bool     `json:"is_correct"`
	Points         int      `json:"points"`
	CorrectAnswer  string   `json:"correct_answer"`
	CorrectAnswers []string `json:"correct_answers"`
}

// Question is a single quiz question in a battle
type Question struct {
	ID       int              `json:"id"`
	Value    string           `json:"value"`
	Label    string           `json:"label"`
	Options  []QuestionOption `json:"options"`
	Config   json.RawMessage  `json:"config,omitempty"` // PHP stores [] or {} — accept either
	Order    int              `json:"order"`
	NoValue  bool             `json:"no_value"`
}

// QuestionOption holds alternatives for a question
type QuestionOption struct {
	Order        int           `json:"order"`
	Alternatives []Alternative `json:"alternatives"`
}

// Alternative is one possible answer in a question option
type Alternative struct {
	ID    int    `json:"id"`
	Type  int    `json:"type"` // 200=ANSWER (correct), 100=OPTION (distractor)
	Value string `json:"value"`
}

// Alternative types — mirror PHP ExerciseQuestionPartTypeEnum (OPTION=100, ANSWER=200).
const (
	AlternativeTypeOption = 100 // distractor
	AlternativeTypeAnswer = 200 // the correct answer
)

// Student is the minimal student data needed by the battle service
type Student struct {
	ID            int
	LevelID       int
	LevelGroupID  int
	CourseID      int
	Status        int
	IsTestingUser bool
}

// PublicProfile mirrors PHP PublicProfileResource (member.student in responses).
type PublicProfile struct {
	FullName string      `json:"full_name"`
	Avatar   *string     `json:"avatar"`
	Level    *LevelInfo  `json:"level"`
	Point    int         `json:"point"`
	Themes   []ThemeInfo `json:"themes"`
}

// LevelInfo mirrors PHP LevelResource (member.student.level).
type LevelInfo struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Order      int     `json:"order"`
	LevelGroup int     `json:"level_group"`
	ParentID   *int    `json:"parent_id"`
	Status     int     `json:"status"`
	CourseID   int     `json:"course_id"`
	ImageURL   *string `json:"image_url"`
}

// ThemeInfo mirrors PHP StudentThemeResource (member.student.themes[]).
type ThemeInfo struct {
	Type int    `json:"type"`
	URL  string `json:"url"`
}

// StudentStatusDemo values — demo students have battle limits
var StudentStatusDemoValues = []int{200, 300}
