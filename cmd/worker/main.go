package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"

	battlelogic "battle-go-api/internal/battle"
	"battle-go-api/internal/config"
	"battle-go-api/internal/db/mysql"
	"battle-go-api/internal/db/postgres"
	"battle-go-api/internal/models"
	"battle-go-api/internal/realtime"
	"battle-go-api/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pgPool, err := pgxpool.New(context.Background(), cfg.PGDSN)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pgPool.Close()

	mysqlDB, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql open: %v", err)
	}
	defer mysqlDB.Close()
	if err := mysqlDB.Ping(); err != nil {
		log.Fatalf("mysql ping: %v", err)
	}

	battleRepo := postgres.NewBattleRepository(pgPool)
	memberRepo := postgres.NewMemberRepository(pgPool)
	resultWriter := mysql.NewResultWriter(mysqlDB)

	// Realtime transport must match the API (REALTIME_DRIVER). For ws the worker
	// publishes via pg_notify, which the API's listener fans out to clients.
	var rt service.RealtimeService
	switch cfg.RealtimeDriver {
	case "ws":
		rt = realtime.NewWSService(pgPool, cfg.WSPublicURL, cfg.JWTPublicKey)
		log.Println("Realtime: WebSocket (pg_notify publisher)")
	default:
		ably, err := realtime.NewAblyService(cfg.AblyKey)
		if err != nil {
			log.Fatalf("ably: %v", err)
		}
		rt = ably
		log.Println("Realtime: Ably")
	}

	endBattleSvc := service.NewEndBattleService(battleRepo, memberRepo, resultWriter, rt)

	log.Println("Worker started")

	// KickMember daemon: every 5s
	go func() {
		for {
			kickExpiredMembers(context.Background(), battleRepo, memberRepo, rt)
			time.Sleep(5 * time.Second)
		}
	}()

	// EndBattle daemon: every 3s
	for {
		endExpiredBattles(context.Background(), battleRepo, memberRepo, endBattleSvc)
		finishIdleMembers(context.Background(), battleRepo, memberRepo, rt)
		endFinishedBattles(context.Background(), battleRepo, memberRepo, endBattleSvc)
		time.Sleep(3 * time.Second)
	}
}

// finishIdleMembers blank-finishes any member stuck on the same question for
// longer than MemberIdleTimeout (2 × question time). Once all members are
// finished, endFinishedBattles (same tick) ends the battle.
// PostgreSQL only — MySQL is written when the whole battle ends.
func finishIdleMembers(ctx context.Context, battles *postgres.BattleRepository, members *postgres.MemberRepository, rt service.RealtimeService) {
	ongoing, err := battles.FindOnGoing(ctx)
	if err != nil {
		log.Printf("idle: find ongoing: %v", err)
		return
	}

	now := time.Now()
	for _, b := range ongoing {
		mems, err := members.FindByBattleID(ctx, b.ID)
		if err != nil {
			log.Printf("idle: members %d: %v", b.ID, err)
			continue
		}
		for _, m := range mems {
			if !battlelogic.IsMemberIdle(m, b.StartTime, now) {
				continue
			}
			battlelogic.FillBlanks(b, m)
			if err := members.Save(ctx, m); err != nil {
				log.Printf("idle: save member %d: %v", m.ID, err)
				continue
			}
			_ = rt.PublishMember(ctx, b.UUID, m)
			log.Printf("idle-finish: battle %d student %d (no activity %s)", b.ID, m.StudentID, battlelogic.MemberIdleTimeout)
		}
	}
}

// kickExpiredMembers handles ON_QUEUE battles whose expire_time has passed.
// Mirrors PHP actionKickMember.
func kickExpiredMembers(ctx context.Context, battles *postgres.BattleRepository, members *postgres.MemberRepository, rt service.RealtimeService) {
	expired, err := battles.FindExpiredOnQueue(ctx)
	if err != nil {
		log.Printf("kick: find expired: %v", err)
		return
	}

	for _, b := range expired {
		mems, err := members.FindByBattleID(ctx, b.ID)
		if err != nil {
			log.Printf("kick: find members battle %d: %v", b.ID, err)
			continue
		}

		toKick, deleteBattle := battlelogic.KickExpiredMembers(b, mems)

		if deleteBattle {
			if err := members.DeleteByBattle(ctx, b.ID); err != nil {
				log.Printf("kick: delete members: %v", err)
			}
			if err := battles.Delete(ctx, b.ID); err != nil {
				log.Printf("kick: delete battle: %v", err)
			}
			continue
		}

		// Delete kicked members
		kickedSet := make(map[int]bool, len(toKick))
		for _, id := range toKick {
			kickedSet[id] = true
			if err := members.Delete(ctx, id); err != nil {
				log.Printf("kick: delete member %d: %v", id, err)
			}
		}

		// Reset battle to WAITING
		b.Status = models.BattleStatusWaiting
		b.ExpireTime = nil
		if err := battles.Save(ctx, b); err != nil {
			log.Printf("kick: save battle: %v", err)
			continue
		}

		// Reset remaining members to NOT_CONFIRMED
		var remaining []*models.BattleMember
		for _, m := range mems {
			if kickedSet[m.ID] {
				continue
			}
			m.Status = models.MemberStatusNotConfirmed
			if err := members.Save(ctx, m); err != nil {
				log.Printf("kick: reset member: %v", err)
			}
			remaining = append(remaining, m)
		}

		_ = rt.PublishBattleWithMembers(ctx, b, remaining)
	}
}

// endExpiredBattles handles ON_GOING battles whose end_time has passed.
// Mirrors PHP actionEndBattle pass 1.
func endExpiredBattles(ctx context.Context, battles *postgres.BattleRepository, members *postgres.MemberRepository, svc *service.EndBattleService) {
	expired, err := battles.FindOnGoingExpired(ctx)
	if err != nil {
		log.Printf("end: find expired: %v", err)
		return
	}

	for _, b := range expired {
		mems, err := members.FindByBattleID(ctx, b.ID)
		if err != nil {
			log.Printf("end: find members %d: %v", b.ID, err)
			continue
		}

		// AI exception: if student never played, delete battle
		if battlelogic.ShouldDeleteAIBattle(b, mems) {
			_ = members.DeleteByBattle(ctx, b.ID)
			_ = battles.Delete(ctx, b.ID)
			continue
		}

		if err := svc.Execute(ctx, b, mems, true); err != nil {
			log.Printf("end: execute battle %d: %v", b.ID, err)
		}
	}
}

// endFinishedBattles handles ON_GOING battles where all members are done.
// Mirrors PHP actionEndBattle pass 2.
func endFinishedBattles(ctx context.Context, battles *postgres.BattleRepository, members *postgres.MemberRepository, svc *service.EndBattleService) {
	ongoing, err := battles.FindOnGoingAllMembersFinished(ctx)
	if err != nil {
		log.Printf("end-finished: find: %v", err)
		return
	}

	for _, b := range ongoing {
		mems, err := members.FindByBattleID(ctx, b.ID)
		if err != nil {
			log.Printf("end-finished: members %d: %v", b.ID, err)
			continue
		}

		if !battlelogic.AllMembersFinished(mems) {
			continue // fake repo returns all ON_GOING; real PG has the NOT EXISTS check
		}

		if err := svc.Execute(ctx, b, mems, false); err != nil {
			log.Printf("end-finished: execute %d: %v", b.ID, err)
		}
	}
}
