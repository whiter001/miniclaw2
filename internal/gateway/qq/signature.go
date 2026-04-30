package qq

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"miniclaw2/internal/config"
)

const webhookSignatureTolerance = 5 * time.Minute

func VerifyWebhookRequest(cfg config.Config, headers http.Header, body []byte, now time.Time) error {
	timestamp := strings.TrimSpace(headers.Get("X-Signature-Timestamp"))
	if timestamp == "" {
		return fmt.Errorf("missing qq webhook signature timestamp")
	}
	signedAt, err := parseWebhookSignatureTimestamp(timestamp)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now()
	}
	if signedAt.Before(now.Add(-webhookSignatureTolerance)) || signedAt.After(now.Add(webhookSignatureTolerance)) {
		return fmt.Errorf("qq webhook signature timestamp is outside the allowed window")
	}
	signatureHex := strings.TrimSpace(headers.Get("X-Signature-Ed25519"))
	if signatureHex == "" {
		return fmt.Errorf("missing qq webhook signature")
	}
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("invalid qq webhook signature encoding: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid qq webhook signature length")
	}
	publicKey, err := qqWebhookPublicKey(cfg.QQAppSecret)
	if err != nil {
		return err
	}
	message := append([]byte(timestamp), body...)
	if !ed25519.Verify(publicKey, message, signature) {
		return fmt.Errorf("invalid qq webhook signature")
	}
	return nil
}

func signQQWebhookPayload(secret, timestamp string, body []byte) (string, error) {
	message := append([]byte(timestamp), body...)
	return signQQWebhookMessage(secret, message)
}

func signQQWebhookMessage(secret string, message []byte) (string, error) {
	privateKey, err := qqWebhookPrivateKey(secret)
	if err != nil {
		return "", err
	}
	signature := ed25519.Sign(privateKey, message)
	return hex.EncodeToString(signature), nil
}

func qqWebhookPublicKey(secret string) (ed25519.PublicKey, error) {
	privateKey, err := qqWebhookPrivateKey(secret)
	if err != nil {
		return nil, err
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("failed to derive qq webhook public key")
	}
	return publicKey, nil
}

func qqWebhookPrivateKey(secret string) (ed25519.PrivateKey, error) {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return nil, fmt.Errorf("qq_app_secret is empty")
	}
	seed := trimmed
	for len(seed) < ed25519.SeedSize {
		seed += trimmed
	}
	seed = seed[:ed25519.SeedSize]
	return ed25519.NewKeyFromSeed([]byte(seed)), nil
}

func parseWebhookSignatureTimestamp(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("missing qq webhook signature timestamp")
	}
	if parsed, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		if parsed > 1_000_000_000_000 {
			return time.UnixMilli(parsed), nil
		}
		return time.Unix(parsed, 0), nil
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("invalid qq webhook signature timestamp")
}
