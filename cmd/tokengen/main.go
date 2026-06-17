// tokengen — dev/test helper that mints go-api JWTs (RS256).
//
// It needs the RSA PRIVATE key (the same MAIN_JWT_PRIVATE_KEY your PHP app signs with).
// The battle API only holds the public key, so token issuance lives here.
//
// ── Private key (one of) ──────────────────────────────────────────────────────
//   JWT_PRIVATE_KEY_FILE=/path/to/private.pem      (PEM file)
//   JWT_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n...\n-----END..."   (inline, \n ok)
//
// ── Run as an HTTP API ────────────────────────────────────────────────────────
//   PORT=8090 JWT_PRIVATE_KEY_FILE=/root/jwt_private.pem ./tokengen
//   GET  /token?user_id=29           -> {"token":"..."}
//   GET  /token?user_id=29&days=30   -> custom expiry (default 1 day)
//   Optional gate: set TOKEN_GEN_SECRET=xyz  then pass ?secret=xyz  (else 401).
//
// ── Or as a CLI (prints a token and exits) ────────────────────────────────────
//   JWT_PRIVATE_KEY_FILE=/root/jwt_private.pem ./tokengen 29
//   JWT_PRIVATE_KEY_FILE=/root/jwt_private.pem ./tokengen 29 30   (days)
//
// SECURITY: this issues a token for ANY user_id with no real auth. Run it only on
// a trusted/internal host, or set TOKEN_GEN_SECRET, or stop it after the demo.
package main

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	key, err := loadPrivateKey()
	if err != nil {
		log.Fatalf("tokengen: load private key: %v", err)
	}

	// CLI mode: ./tokengen <user_id> [days]
	if len(os.Args) > 1 {
		uid, err := strconv.Atoi(os.Args[1])
		if err != nil {
			log.Fatalf("tokengen: user_id must be a number, got %q", os.Args[1])
		}
		days := 1
		if len(os.Args) > 2 {
			if d, err := strconv.Atoi(os.Args[2]); err == nil {
				days = d
			}
		}
		tok, err := mint(key, uid, days)
		if err != nil {
			log.Fatalf("tokengen: %v", err)
		}
		fmt.Println(tok)
		return
	}

	// HTTP mode
	secret := os.Getenv("TOKEN_GEN_SECRET")
	port := env("PORT", "8090")

	http.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if secret != "" && r.URL.Query().Get("secret") != secret {
			http.Error(w, `{"error":"forbidden"}`, http.StatusUnauthorized)
			return
		}
		uid, err := strconv.Atoi(r.URL.Query().Get("user_id"))
		if err != nil {
			http.Error(w, `{"error":"user_id query param required (number)"}`, http.StatusBadRequest)
			return
		}
		days := 1
		if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d > 0 {
			days = d
		}
		tok, err := mint(key, uid, days)
		if err != nil {
			http.Error(w, `{"error":"sign failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"token": tok, "user_id": uid, "expires_days": days})
	})

	log.Printf("tokengen listening on :%s  (GET /token?user_id=29[&days=30])", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// mint builds a go-api RS256 token matching what the PHP app issues.
func mint(key *rsa.PrivateKey, userID, days int) (string, error) {
	claims := jwt.MapClaims{
		"exp":     time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix(),
		"iss":     "main",
		"user_id": userID,
		"aud":     "go-api",
		"role":    "student",
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
}

func loadPrivateKey() (*rsa.PrivateKey, error) {
	var pem string
	if f := os.Getenv("JWT_PRIVATE_KEY_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		pem = string(b)
	} else if v := os.Getenv("JWT_PRIVATE_KEY"); v != "" {
		pem = strings.ReplaceAll(v, `\n`, "\n") // support \n literals from env files
	} else {
		return nil, fmt.Errorf("set JWT_PRIVATE_KEY_FILE or JWT_PRIVATE_KEY")
	}
	return jwt.ParseRSAPrivateKeyFromPEM([]byte(pem))
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
