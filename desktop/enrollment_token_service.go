package main

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

var maxEnrollmentTTLSeconds = int64(time.Duration(1<<63-1) / time.Second)

type EnrollmentTokenRequest struct {
	Fleet      string `json:"fleet"`
	TTLSeconds int64  `json:"ttlSeconds"`
}

type ClipboardWriter func(context.Context, string) error

type EnrollmentTokenService struct {
	mu    sync.Mutex
	token []byte
}

func NewEnrollmentTokenService() *EnrollmentTokenService {
	return &EnrollmentTokenService{}
}

func (s *EnrollmentTokenService) CreateConnected(ctx context.Context, client *admin.Client, request EnrollmentTokenRequest) (EnrollmentTokenResult, error) {
	if client == nil {
		return EnrollmentTokenResult{}, ErrSessionNotConnected
	}
	if request.Fleet == "" {
		return EnrollmentTokenResult{}, enrollmentValidationFailure("Select an existing Fleet before creating a token.")
	}
	if request.TTLSeconds <= 0 || request.TTLSeconds > maxEnrollmentTTLSeconds {
		return EnrollmentTokenResult{}, enrollmentValidationFailure("Enter a positive token lifetime supported by the server.")
	}

	fleets, err := client.ListFleetsContext(ctx)
	if err != nil {
		return EnrollmentTokenResult{}, err
	}
	if !slices.Contains(fleets, request.Fleet) {
		return EnrollmentTokenResult{}, enrollmentValidationFailure("Select an existing Fleet from the current workspace.")
	}

	response, err := client.CreateEnrollTokenContext(ctx, request.Fleet, time.Duration(request.TTLSeconds)*time.Second)
	if err != nil {
		return EnrollmentTokenResult{}, err
	}
	if response.Fleet != request.Fleet || response.Token == "" || response.ExpiresAt.IsZero() {
		return EnrollmentTokenResult{}, errors.New("server returned incomplete enrollment token metadata")
	}
	if cause := context.Cause(ctx); cause != nil {
		return EnrollmentTokenResult{}, cause
	}

	s.replace(response.Token)
	return EnrollmentTokenResult{
		Token:     response.Token,
		Fleet:     response.Fleet,
		ExpiresAt: response.ExpiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *EnrollmentTokenService) Copy(ctx context.Context, writer ClipboardWriter) error {
	if writer == nil {
		return errors.New("native clipboard is unavailable")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.token) == 0 {
		return errors.New("the enrollment token has been cleared")
	}
	if err := writer(ctx, string(s.token)); err != nil {
		return errors.New("native clipboard copy failed")
	}
	return nil
}

func (s *EnrollmentTokenService) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	zeroBytes(s.token)
	s.token = nil
}

func (s *EnrollmentTokenService) replace(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	zeroBytes(s.token)
	s.token = []byte(token)
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func enrollmentValidationFailure(guidance string) error {
	return &ActionFailure{
		Kind:      ActionValidation,
		Message:   "The enrollment token request is invalid.",
		Guidance:  guidance,
		Retryable: false,
	}
}
