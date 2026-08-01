package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type sessionKey struct {
	label       string
	name        string
	host        string
	port        int
	user        string
	keyPath     string
	passHash    string
	keyPassHash string
	knownHosts  string
	strict      bool
	agent       bool
	persist     bool
	timeout     time.Duration
	keepAlive   time.Duration
	retries     int
}

// sessionKeyFor expects cfg to have already passed through Config.normalize.
func sessionKeyFor(cfg Config) sessionKey {
	return sessionKey{
		label:       cfg.Label,
		name:        cfg.Name,
		host:        cfg.Host,
		port:        cfg.Port,
		user:        cfg.User,
		keyPath:     cfg.KeyPath,
		passHash:    hashIfSet(cfg.Pass),
		keyPassHash: hashIfSet(cfg.KeyPass),
		knownHosts:  cfg.KnownHosts,
		strict:      cfg.Strict,
		agent:       cfg.Agent,
		persist:     cfg.Persist,
		timeout:     cfg.Timeout,
		keepAlive:   cfg.KeepAlive,
		retries:     cfg.Retries,
	}
}

func hashIfSet(secret string) string {
	if secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
