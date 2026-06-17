package config

import (
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"

	"battle-go-api/internal/service"
)

// App holds all resolved runtime configuration.
type App struct {
	Port         string
	PGDSN        string
	MySQLDSN     string
	AblyKey      string
	JWTPublicKey *rsa.PublicKey
	Battle       service.Config

	// RealtimeDriver selects the realtime transport: "ably" (default) or "ws".
	RealtimeDriver string
	// WSPublicURL is the public ws(s):// base the frontend uses to connect (ws driver only).
	WSPublicURL string
}

// Load reads .env (if present) then resolves config from environment variables.
func Load() (*App, error) {
	_ = godotenv.Load() // ignore error — .env is optional in production

	// clientFoundRows=true → UPDATE RowsAffected reflects matched rows.
	// loc=Local + parseTime=true → write/read MySQL DATETIMEs in the process's local
	// timezone so start_time/end_time match MySQL NOW()/created_at (driver default is UTC).
	mysqlDSN := mustEnv("MYSQL_DSN")
	mysqlDSN = appendDSNParam(mysqlDSN, "clientFoundRows", "true")
	mysqlDSN = appendDSNParam(mysqlDSN, "parseTime", "true")
	mysqlDSN = appendDSNParam(mysqlDSN, "loc", "Local")

	port := env("PORT", "8080")
	driver := env("REALTIME_DRIVER", "ably")

	cfg := &App{
		Port:           port,
		PGDSN:          mustEnv("PG_DSN"),
		MySQLDSN:       mysqlDSN,
		AblyKey:        env("ABLY_KEY", ""), // required only when driver=ably
		RealtimeDriver: driver,
		WSPublicURL:    env("WS_PUBLIC_URL", "ws://localhost:"+port+"/student/v1/battle/ws"),
	}

	if driver == "ably" && cfg.AblyKey == "" {
		return nil, fmt.Errorf("config: ABLY_KEY required when REALTIME_DRIVER=ably")
	}

	pubKey, err := parsePublicKey(mustEnv("JWT_PUBLIC_KEY"))
	if err != nil {
		return nil, fmt.Errorf("config: parse JWT_PUBLIC_KEY: %w", err)
	}
	cfg.JWTPublicKey = pubKey

	return cfg, nil
}

// LoadBattleConfig reads battle.config from MySQL key_storage.
// Falls back to defaults if row not found.
func LoadBattleConfig(db *sql.DB) service.Config {
	var raw []byte
	err := db.QueryRow(`SELECT value FROM key_storage WHERE ` + "`key`" + ` = 'battle.config' LIMIT 1`).Scan(&raw)
	if err != nil {
		return service.DefaultConfig()
	}

	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return service.DefaultConfig()
	}

	def := service.DefaultConfig()
	cfg := def

	if v, ok := m["demo_limit"].(float64); ok {
		cfg.DemoLimit = int(v)
	}
	if v, ok := m["word_question_count"].(float64); ok {
		cfg.WordQuestionCount = int(v)
	}
	if v, ok := m["word_option_count"].(float64); ok {
		cfg.WordOptionCount = int(v)
	}
	if v, ok := m["exercise_question_count"].(float64); ok {
		cfg.ExerciseQuestionCount = int(v)
	}
	if v, ok := m["translate_foreign_word"].(string); ok {
		cfg.TranslateForeignText = v
	}
	if v, ok := m["translate_origin_word"].(string); ok {
		cfg.TranslateOriginText = v
	}
	return cfg
}

func parsePublicKey(pem string) (*rsa.PublicKey, error) {
	// Support both \\n (env file literal) and real newlines
	pem = strings.ReplaceAll(pem, `\n`, "\n")
	key, err := jwt.ParseRSAPublicKeyFromPEM([]byte(pem))
	if err != nil {
		return nil, err
	}
	return key, nil
}

// appendDSNParam adds key=value to a go-sql-driver DSN query string if not already present.
func appendDSNParam(dsn, key, value string) string {
	if strings.Contains(dsn, key+"=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + key + "=" + value
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %s is not set", key))
	}
	return v
}
