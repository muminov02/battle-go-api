package service

import "errors"

var (
	ErrStudentNotFound   = errors.New("student not found")
	ErrStudentNoLevel    = errors.New("student has no level")
	ErrDemoLimitExceeded = errors.New("demo limit exceeded")
	ErrBattleNotFound    = errors.New("battle not found")
	ErrNotMember         = errors.New("not a battle member")
	ErrBattleFinished    = errors.New("battle already finished")
	ErrBattleNotStarted  = errors.New("battle not started")
	ErrNoTestingUsers    = errors.New("no testing users available for AI opponent")
)
