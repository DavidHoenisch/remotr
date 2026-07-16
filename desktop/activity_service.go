package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

const (
	activityPageLimit       = 50
	activitySeenIDLimit     = 1000
	activityDetailLimit     = 20
	activityDetailValueSize = 1024
)

type ActivityOption func(*ActivityService)

type ActivityService struct {
	connection *ConnectionService
	now        func() time.Time
}

func NewActivityService(options ...ActivityOption) *ActivityService {
	service := &ActivityService{
		connection: NewConnectionService(),
		now:        time.Now,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func WithActivityClock(now func() time.Time) ActivityOption {
	return func(service *ActivityService) {
		if now != nil {
			service.now = now
		}
	}
}

func (s *ActivityService) LoadPage(ctx context.Context, profile ConnectionProfile, request ActivityPageRequest) (ActivityPageView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	options, err := activityListOptions(request)
	if err != nil {
		return ActivityPageView{}, err
	}
	connected, err := s.connection.connect(ctx, profile)
	if err != nil {
		return ActivityPageView{}, err
	}
	return s.loadPageConnected(ctx, connected.client, request, options)
}

func (s *ActivityService) LoadPageConnected(ctx context.Context, client *admin.Client, request ActivityPageRequest) (ActivityPageView, error) {
	if client == nil {
		return ActivityPageView{}, ErrSessionNotConnected
	}
	options, err := activityListOptions(request)
	if err != nil {
		return ActivityPageView{}, err
	}
	return s.loadPageConnected(ctx, client, request, options)
}

func (s *ActivityService) loadPageConnected(
	ctx context.Context,
	client *admin.Client,
	request ActivityPageRequest,
	options admin.AuditListOptions,
) (ActivityPageView, error) {
	page, err := client.ListAuditEventsContext(ctx, options)
	if cause := context.Cause(ctx); cause != nil {
		return ActivityPageView{}, cause
	}
	loadedAt := s.now().UTC()
	if err != nil {
		state, classified := classifyWorkspaceSectionError("Activity", err)
		return ActivityPageView{
			Events: []ActivityEvent{},
			Section: SectionResult{
				State: state,
				Snapshot: SnapshotTimestamps{
					FailedAt: timestampPointer(loadedAt),
				},
				Error: classified,
			},
		}, nil
	}

	seen := make(map[string]struct{}, len(request.SeenEventIDs)+len(page.Events))
	for _, eventID := range request.SeenEventIDs {
		seen[eventID] = struct{}{}
	}
	events := make([]ActivityEvent, 0, min(len(page.Events), activityPageLimit))
	for _, event := range page.Events {
		if event.ID == "" {
			continue
		}
		if _, duplicate := seen[event.ID]; duplicate {
			continue
		}
		seen[event.ID] = struct{}{}
		events = append(events, mapAuditEvent(event))
		if len(events) == activityPageLimit {
			break
		}
	}
	return ActivityPageView{
		Events:     events,
		NextCursor: page.NextCursor,
		Section:    workspaceSectionResult("Activity", nil, len(events), loadedAt, latestActivityObservation(page.Events)),
	}, nil
}

func activityListOptions(request ActivityPageRequest) (admin.AuditListOptions, error) {
	if len(request.SeenEventIDs) > activitySeenIDLimit {
		return admin.AuditListOptions{}, activityValidationFailure("Activity history is too large to continue safely.")
	}
	since, err := parseOptionalActivityTime(request.Since)
	if err != nil {
		return admin.AuditListOptions{}, activityValidationFailure("Activity start time must be an RFC 3339 timestamp.")
	}
	until, err := parseOptionalActivityTime(request.Until)
	if err != nil {
		return admin.AuditListOptions{}, activityValidationFailure("Activity end time must be an RFC 3339 timestamp.")
	}
	if !since.IsZero() && !until.IsZero() && until.Before(since) {
		return admin.AuditListOptions{}, activityValidationFailure("Activity end time must not be before its start time.")
	}
	return admin.AuditListOptions{
		Since:     since,
		Until:     until,
		Action:    strings.TrimSpace(request.Action),
		ActorType: strings.TrimSpace(request.ActorType),
		Limit:     activityPageLimit,
		Cursor:    request.Cursor,
	}, nil
}

func parseOptionalActivityTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func activityValidationFailure(message string) error {
	return &ClassifiedError{
		Kind:     ErrorValidation,
		Message:  message,
		Guidance: "Adjust the Activity filters and try again.",
	}
}

func mapAuditEvent(event admin.AuditEvent) ActivityEvent {
	actor := event.ActorID
	if actor == "" {
		actor = event.ActorType
	}
	status := "failed"
	if event.StatusCode >= 200 && event.StatusCode < 300 {
		status = "accepted"
	}
	return ActivityEvent{
		EventID:      event.ID,
		OccurredAt:   formatTimestamp(event.OccurredAt),
		Actor:        actor,
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Status:       status,
		RequestID:    event.RequestID,
		Details:      safeActivityDetails(event.Details),
	}
}

func safeActivityDetails(details map[string]any) []ActivityDetail {
	keys := make([]string, 0, len(details))
	for key := range details {
		if !sensitiveActivityDetailKey(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]ActivityDetail, 0, min(len(keys), activityDetailLimit))
	for _, key := range keys {
		value, ok := formatActivityDetailValue(details[key])
		if !ok {
			continue
		}
		result = append(result, ActivityDetail{Key: key, Value: value})
		if len(result) == activityDetailLimit {
			break
		}
	}
	return result
}

func sensitiveActivityDetailKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
	for _, sensitive := range []string{
		"token", "secret", "password", "privatekey", "certificate", "certpem",
		"keypem", "fingerprint", "clientip", "authorization", "cookie",
	} {
		if strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}

func formatActivityDetailValue(value any) (string, bool) {
	var formatted string
	switch typed := value.(type) {
	case string:
		formatted = typed
	case bool:
		formatted = fmt.Sprintf("%t", typed)
	case float64:
		formatted = fmt.Sprintf("%v", typed)
	default:
		return "", false
	}
	if len(formatted) > activityDetailValueSize {
		formatted = boundedViewString(formatted, activityDetailValueSize)
	}
	return formatted, true
}
