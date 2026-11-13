package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// defaultFingerprint derives the entire bundle from the server, including the
// runtime CLI sync value. Neither inbound headers nor Redis can upgrade it.
func defaultFingerprint() Fingerprint {
	h := claude.DefaultHeaders()
	return Fingerprint{
		UserAgent:               h["User-Agent"],
		StainlessLang:           h["X-Stainless-Lang"],
		StainlessPackageVersion: h["X-Stainless-Package-Version"],
		StainlessOS:             h["X-Stainless-OS"],
		StainlessArch:           h["X-Stainless-Arch"],
		StainlessRuntime:        h["X-Stainless-Runtime"],
		StainlessRuntimeVersion: h["X-Stainless-Runtime-Version"],
	}
}

// Fingerprint represents account fingerprint data. Only ClientID is durable;
// UpdatedAt remains for compatibility with the legacy Redis format.
type Fingerprint struct {
	ClientID                string
	UserAgent               string
	StainlessLang           string
	StainlessPackageVersion string
	StainlessOS             string
	StainlessArch           string
	StainlessRuntime        string
	StainlessRuntimeVersion string
	UpdatedAt               int64 `json:",omitempty"`
}

// IdentityCache retains legacy fingerprint access for non-Claude consumers.
// Claude identity requests use only its masked-session operations.
type IdentityCache interface {
	GetFingerprint(ctx context.Context, accountID int64) (*Fingerprint, error)
	SetFingerprint(ctx context.Context, accountID int64, fp *Fingerprint) error
	GetMaskedSessionID(ctx context.Context, accountID int64) (string, error)
	SetMaskedSessionID(ctx context.Context, accountID int64, sessionID string) error
}

// ClaudeIdentityStore is the database authority. found=true with an empty ID is
// a reserved legacy row awaiting verified manual import, never a cache miss.
type ClaudeIdentityStore interface {
	GetClientID(context.Context, int64) (clientID string, found bool, err error)
	// InitializeClientID inserts only if absent and returns the persisted winner.
	InitializeClientID(context.Context, int64, string) (string, error)
}

// Do not wrap storage errors: they may contain SQL, headers or identity values.
var ErrClaudeIdentityUnavailable = errors.New("claude identity unavailable")

type IdentityService struct {
	cache IdentityCache
	store ClaudeIdentityStore
}

func NewIdentityService(cache IdentityCache, store ClaudeIdentityStore) *IdentityService {
	return &IdentityService{cache: cache, store: store}
}

// GetOrCreateFingerprint never reads or writes fingerprint Redis. Existing
// reserved/invalid rows fail closed, even during concurrent initialization.
func (s *IdentityService) GetOrCreateFingerprint(ctx context.Context, accountID int64, _ http.Header) (*Fingerprint, error) {
	if s == nil || s.store == nil {
		return nil, ErrClaudeIdentityUnavailable
	}
	clientID, found, err := s.store.GetClientID(ctx, accountID)
	if err != nil {
		return nil, ErrClaudeIdentityUnavailable
	}
	if !found {
		candidate, err := generateClientID()
		if err != nil {
			return nil, ErrClaudeIdentityUnavailable
		}
		clientID, err = s.store.InitializeClientID(ctx, accountID, candidate)
		if err != nil {
			return nil, ErrClaudeIdentityUnavailable
		}
	}
	if !validClaudeClientID(clientID) {
		return nil, ErrClaudeIdentityUnavailable
	}
	fp := defaultFingerprint()
	fp.ClientID = clientID
	return &fp, nil
}

func validClaudeClientID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (s *IdentityService) ApplyFingerprint(req *http.Request, fp *Fingerprint) {
	if fp == nil {
		return
	}
	setHeaderRaw(req.Header, "User-Agent", fp.UserAgent)
	setHeaderRaw(req.Header, "X-Stainless-Lang", fp.StainlessLang)
	setHeaderRaw(req.Header, "X-Stainless-Package-Version", fp.StainlessPackageVersion)
	setHeaderRaw(req.Header, "X-Stainless-OS", fp.StainlessOS)
	setHeaderRaw(req.Header, "X-Stainless-Arch", fp.StainlessArch)
	setHeaderRaw(req.Header, "X-Stainless-Runtime", fp.StainlessRuntime)
	setHeaderRaw(req.Header, "X-Stainless-Runtime-Version", fp.StainlessRuntimeVersion)
}

// RewriteUserID preserves the existing metadata and session derivation algorithm.
func (s *IdentityService) RewriteUserID(body []byte, accountID int64, accountUUID, cachedClientID, fingerprintUA string) ([]byte, error) {
	if len(body) == 0 || accountUUID == "" || cachedClientID == "" {
		return body, nil
	}
	metadata := gjson.GetBytes(body, "metadata")
	if !metadata.Exists() || metadata.Type == gjson.Null {
		return body, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(metadata.Raw), "{") {
		return body, nil
	}
	userIDResult := metadata.Get("user_id")
	if !userIDResult.Exists() || userIDResult.Type != gjson.String {
		return body, nil
	}
	userID := userIDResult.String()
	if userID == "" {
		return body, nil
	}
	parsed := ParseMetadataUserID(userID)
	if parsed == nil {
		return body, nil
	}
	sessionTail := parsed.SessionID
	seed := fmt.Sprintf("%d::%s", accountID, sessionTail)
	newSessionHash := generateUUIDFromSeed(seed)
	version := ExtractCLIVersion(fingerprintUA)
	newUserID := FormatMetadataUserID(cachedClientID, accountUUID, newSessionHash, version)
	if newUserID == userID {
		return body, nil
	}
	newBody, err := sjson.SetBytes(body, "metadata.user_id", newUserID)
	if err != nil {
		return body, nil
	}
	return newBody, nil
}

// RewriteUserIDWithMasking retains the existing optional masked-session behavior.
func (s *IdentityService) RewriteUserIDWithMasking(ctx context.Context, body []byte, account *Account, accountUUID, cachedClientID, fingerprintUA string) ([]byte, error) {
	newBody, err := s.RewriteUserID(body, account.ID, accountUUID, cachedClientID, fingerprintUA)
	if err != nil {
		return newBody, err
	}
	if !account.IsSessionIDMaskingEnabled() {
		return newBody, nil
	}
	metadata := gjson.GetBytes(newBody, "metadata")
	if !metadata.Exists() || metadata.Type == gjson.Null {
		return newBody, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(metadata.Raw), "{") {
		return newBody, nil
	}
	userIDResult := metadata.Get("user_id")
	if !userIDResult.Exists() || userIDResult.Type != gjson.String {
		return newBody, nil
	}
	userID := userIDResult.String()
	if userID == "" {
		return newBody, nil
	}
	uidParsed := ParseMetadataUserID(userID)
	if uidParsed == nil {
		return newBody, nil
	}
	maskedSessionID, err := s.cache.GetMaskedSessionID(ctx, account.ID)
	if err != nil {
		logger.LegacyPrintf("service.identity", "Warning: failed to get masked session ID for account %d: %v", account.ID, err)
		return newBody, nil
	}
	if maskedSessionID == "" {
		maskedSessionID = generateRandomUUID()
		logger.LegacyPrintf("service.identity", "Generated masked session ID for account %d", account.ID)
	}
	if err := s.cache.SetMaskedSessionID(ctx, account.ID, maskedSessionID); err != nil {
		logger.LegacyPrintf("service.identity", "Warning: failed to set masked session ID for account %d: %v", account.ID, err)
	}
	version := ExtractCLIVersion(fingerprintUA)
	newUserID := FormatMetadataUserID(uidParsed.DeviceID, uidParsed.AccountUUID, maskedSessionID, version)
	slog.Debug("session_id_masking_applied", "account_id", account.ID, "metadata_present", true, "new_format", uidParsed.IsNewFormat)
	if newUserID == userID {
		return newBody, nil
	}
	maskedBody, setErr := sjson.SetBytes(newBody, "metadata.user_id", newUserID)
	if setErr != nil {
		return newBody, nil
	}
	return maskedBody, nil
}

func generateRandomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		b = h[:16]
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func generateClientID() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", ErrClaudeIdentityUnavailable
	}
	return hex.EncodeToString(b), nil
}

func generateUUIDFromSeed(seed string) string {
	hash := sha256.Sum256([]byte(seed))
	bytes := hash[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}
