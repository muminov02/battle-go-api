package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"

	"battle-go-api/internal/config"
	"battle-go-api/internal/db/mysql"
	"battle-go-api/internal/db/postgres"
	"battle-go-api/internal/handler"
	"battle-go-api/internal/realtime"
	"battle-go-api/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// PostgreSQL
	pgPool, err := pgxpool.New(context.Background(), cfg.PGDSN)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pgPool.Close()

	if err := postgres.Migrate(context.Background(), pgPool); err != nil {
		log.Fatalf("postgres migrate: %v", err)
	}
	log.Println("PostgreSQL ready")

	// MySQL
	mysqlDB, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql open: %v", err)
	}
	defer mysqlDB.Close()
	if err := mysqlDB.Ping(); err != nil {
		log.Fatalf("mysql ping: %v", err)
	}
	log.Println("MySQL ready")

	// Load battle config from MySQL key_storage
	battleCfg := config.LoadBattleConfig(mysqlDB)

	// Repositories
	battleRepo := postgres.NewBattleRepository(pgPool)
	memberRepo := postgres.NewMemberRepository(pgPool)
	studentReader := mysql.NewStudentReader(mysqlDB)
	questionReader := mysql.NewQuestionReader(mysqlDB)
	resultWriter := mysql.NewResultWriter(mysqlDB)
	profileReader := mysql.NewProfileReader(mysqlDB)

	// Realtime: pick transport by REALTIME_DRIVER (ws | ably).
	// Both implement service.RealtimeService + handler.TokenProvider + handler.RealtimeInfo.
	var (
		rt        service.RealtimeService
		tokens    handler.TokenProvider
		rtInfo    handler.RealtimeInfo
		wsHandler http.HandlerFunc
	)
	switch cfg.RealtimeDriver {
	case "ws":
		wsSvc := realtime.NewWSService(pgPool, cfg.WSPublicURL, cfg.JWTPublicKey)
		go wsSvc.StartListener(context.Background()) // fan-out pg_notify → connected clients
		rt, tokens, rtInfo = wsSvc, wsSvc, wsSvc
		wsHandler = wsSvc.ServeWS
		log.Printf("Realtime: WebSocket (%s)", cfg.WSPublicURL)
	default:
		ablySvc, err := realtime.NewAblyService(cfg.AblyKey)
		if err != nil {
			log.Fatalf("ably: %v", err)
		}
		rt, tokens, rtInfo = ablySvc, ablySvc, ablySvc
		log.Println("Realtime: Ably")
	}

	// Services
	findSvc := service.NewFindService(battleRepo, memberRepo, studentReader, questionReader, resultWriter, rt, battleCfg)
	confirmSvc := service.NewConfirmService(battleRepo, memberRepo, questionReader, resultWriter, rt, battleCfg)
	answerSvc := service.NewAnswerService(battleRepo, memberRepo, resultWriter, rt)
	leaveSvc := service.NewLeaveService(battleRepo, memberRepo, resultWriter, rt)
	changeTypeSvc := service.NewChangeTypeService(battleRepo, memberRepo, studentReader, rt)
	endBattleSvc := service.NewEndBattleService(battleRepo, memberRepo, resultWriter, rt)
	viewSvc := service.NewViewService(battleRepo, memberRepo)

	// HTTP handler
	h := handler.New(findSvc, confirmSvc, answerSvc, leaveSvc, changeTypeSvc, endBattleSvc, viewSvc, tokens, profileReader, rtInfo)
	router := handler.NewRouter(h, cfg.JWTPublicKey, wsHandler)

	addr := ":" + cfg.Port
	log.Printf("battle API listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}
