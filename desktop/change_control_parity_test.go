package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

type changeControlRecordedRequest struct {
	Method string
	Path   string
	Body   []byte
}

type changeControlParityState struct {
	mu            sync.Mutex
	requests      []changeControlRecordedRequest
	lifecycle     string
	approvals     int
	forbid        bool
	blockPause    chan struct{}
	pauseEntered  chan struct{}
	pauseReleased sync.Once
}

func TestChangeControlParityPreservesApprovalAndExactMutationScope(t *testing.T) {
	app, state, _ := newChangeControlParityTestApp(t)

	for name, request := range map[string]ChangeAuthorizationRequest{
		"wrong confirmation": {
			ChangeRequestID: "change-prod", Confirmation: "CHANGE-PROD", Justification: "CHG-404", AttemptLimit: 1, MaxConcurrency: 1,
		},
		"missing justification": {
			ChangeRequestID: "change-prod", Confirmation: "change-prod", AttemptLimit: 1, MaxConcurrency: 1,
		},
		"unbounded concurrency": {
			ChangeRequestID: "change-prod", Confirmation: "change-prod", Justification: "CHG-404", AttemptLimit: 1, MaxConcurrency: 3,
		},
		"attempt cap exceeded": {
			ChangeRequestID: "change-prod", Confirmation: "change-prod", Justification: "CHG-404", AttemptLimit: 101, MaxConcurrency: 1,
		},
		"invalid execution window": {
			ChangeRequestID: "change-prod", Confirmation: "change-prod", Justification: "CHG-404", AttemptLimit: 1, MaxConcurrency: 1,
			ExecutionWindows: []ChangeExecutionWindowInput{{Weekdays: []int{1}, StartMinuteUTC: 1440, DurationMinutes: 1}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := app.AuthorizeChangeRequest(request); err == nil {
				t.Fatalf("AuthorizeChangeRequest(%#v) succeeded", request)
			}
		})
	}
	if got := state.mutationRequests(); len(got) != 0 {
		t.Fatalf("invalid authorization reached mutation API: %#v", got)
	}

	authorized, err := app.AuthorizeChangeRequest(ChangeAuthorizationRequest{
		ChangeRequestID: "change-prod",
		Confirmation:    "change-prod",
		Justification:   "CHG-404 connectivity canary",
		AttemptLimit:    2,
		MaxConcurrency:  1,
		ValidFrom:       "2032-03-04T05:00:00Z",
		ValidUntil:      "2032-03-05T05:00:00Z",
		ExecutionWindows: []ChangeExecutionWindowInput{{
			Weekdays: []int{1, 3, 5}, StartMinuteUTC: 120, DurationMinutes: 45,
		}},
	})
	if err != nil {
		t.Fatalf("authorize Change request: %v", err)
	}
	if authorized.Action != "approval_recorded" || authorized.ChangeRequest.Summary.ApprovalCount != 1 || authorized.ChangeRequest.Summary.RequiredApprovals != 2 || authorized.ChangeRequest.Summary.Lifecycle != "pending" {
		t.Fatalf("authorization result = %#v, want one server-recorded pending approval", authorized)
	}
	requests := state.mutationRequests()
	if len(requests) != 1 || requests[0].Path != "/v1/admin/change-requests/change-prod/authorize" {
		t.Fatalf("authorization requests = %#v", requests)
	}
	var authorizationBody struct {
		AttemptLimit     int `json:"attempt_limit"`
		MaxConcurrency   int `json:"max_concurrency"`
		Justification    string
		ExecutionWindows []struct {
			Weekdays       []int         `json:"weekdays"`
			StartMinuteUTC int           `json:"start_minute_utc"`
			Duration       time.Duration `json:"duration"`
		} `json:"execution_windows"`
	}
	if err := json.Unmarshal(requests[0].Body, &authorizationBody); err != nil {
		t.Fatalf("decode authorization body: %v", err)
	}
	if authorizationBody.AttemptLimit != 2 || authorizationBody.MaxConcurrency != 1 || authorizationBody.Justification != "CHG-404 connectivity canary" || len(authorizationBody.ExecutionWindows) != 1 || authorizationBody.ExecutionWindows[0].Duration != 45*time.Minute {
		t.Fatalf("authorization body = %#v", authorizationBody)
	}

	state.mu.Lock()
	state.lifecycle = "authorized"
	state.approvals = 2
	state.mu.Unlock()

	for _, action := range []struct {
		name string
		want string
	}{
		{name: "pause", want: "paused"},
		{name: "resume", want: "authorized"},
		{name: "revoke", want: "revoked"},
	} {
		if _, err := app.ChangeRequestLifecycle(ChangeLifecycleRequest{
			ChangeRequestID: "change-prod", Confirmation: "not-change-prod", Action: action.name,
		}); err == nil {
			t.Fatalf("%s accepted mismatched confirmation", action.name)
		}
		result, err := app.ChangeRequestLifecycle(ChangeLifecycleRequest{
			ChangeRequestID: "change-prod", Confirmation: "change-prod", Action: action.name,
		})
		if err != nil {
			t.Fatalf("%s Change request: %v", action.name, err)
		}
		if result.ChangeRequest.Summary.ChangeRequestID != "change-prod" || result.ChangeRequest.Summary.Lifecycle != action.want {
			t.Fatalf("%s result = %#v", action.name, result)
		}
	}

	if _, err := app.PromoteChangeBaseline(ChangeBaselinePromotionRequest{
		ChangeRequestID: "change-prod", Confirmation: "base/firewall", ResourceAddress: "base/firewall",
	}); err == nil {
		t.Fatal("baseline promotion with unresolved outcome and no acknowledgement succeeded")
	}
	if _, err := app.PromoteChangeBaseline(ChangeBaselinePromotionRequest{
		ChangeRequestID: "change-prod", Confirmation: "base/unknown", ResourceAddress: "base/unknown", AcknowledgeExceptions: true,
	}); err == nil {
		t.Fatal("baseline promotion for resource outside Change request succeeded")
	}
	promoted, err := app.PromoteChangeBaseline(ChangeBaselinePromotionRequest{
		ChangeRequestID:       "change-prod",
		Confirmation:          "base/firewall",
		ResourceAddress:       "base/firewall",
		AcknowledgeExceptions: true,
	})
	if err != nil {
		t.Fatalf("promote Change baseline: %v", err)
	}
	if promoted.Baseline == nil || promoted.Baseline.ChangeRequestID != "change-prod" || promoted.Baseline.ResourceAddress != "base/firewall" || promoted.Baseline.DesiredHash != "sha256:firewall" {
		t.Fatalf("baseline promotion result = %#v", promoted)
	}

	preview, err := app.ChooseBaselineAdoptionPlan("production")
	if err != nil {
		t.Fatalf("choose baseline adoption plan: %v", err)
	}
	if preview.PlanID == "" || preview.Fleet != "production" || preview.ReleaseRef != "" || preview.ResourceCount != 0 || preview.TargetCount != 0 || len(preview.ResourceAddresses) != 0 {
		t.Fatalf("baseline adoption preview = %#v", preview)
	}
	if _, err := app.CreateBaselineAdoption(BaselineAdoptionRequest{PlanID: preview.PlanID, Fleet: "production", Confirmation: "Production"}); err == nil {
		t.Fatal("baseline adoption with case-insensitive confirmation succeeded")
	}
	adopted, err := app.CreateBaselineAdoption(BaselineAdoptionRequest{PlanID: preview.PlanID, Fleet: "production", Confirmation: "production"})
	if err != nil {
		t.Fatalf("create baseline adoption: %v", err)
	}
	if adopted.Action != "baseline_adoption_created" || adopted.ChangeRequest.Summary.ChangeRequestID != "adoption-prod" || adopted.ChangeRequest.Summary.Fleet != "production" {
		t.Fatalf("baseline adoption result = %#v", adopted)
	}
	requests = state.mutationRequests()
	adoptionRequest := requests[len(requests)-1]
	if adoptionRequest.Path != "/v1/admin/fleets/production/baseline-adoptions" || string(adoptionRequest.Body) != `{}` {
		t.Fatalf("baseline adoption request = %#v, want server-derived empty request", adoptionRequest)
	}
	if _, err := app.CreateBaselineAdoption(BaselineAdoptionRequest{PlanID: preview.PlanID, Fleet: "production", Confirmation: "production"}); err == nil {
		t.Fatal("consumed baseline adoption plan was replayed")
	}

	state.mu.Lock()
	state.forbid = true
	state.mu.Unlock()
	_, err = app.ChangeRequestLifecycle(ChangeLifecycleRequest{ChangeRequestID: "change-prod", Confirmation: "change-prod", Action: "pause"})
	var failure *ActionFailure
	if !errors.As(err, &failure) || failure.Kind != ActionForbidden {
		t.Fatalf("forbidden lifecycle error = %T %v, want authorization ActionFailure", err, err)
	}
}

func TestChangeControlParityAllowsOneInflightActionPerTarget(t *testing.T) {
	app, state, _ := newChangeControlParityTestApp(t)
	state.mu.Lock()
	state.lifecycle = "authorized"
	state.blockPause = make(chan struct{})
	state.pauseEntered = make(chan struct{})
	state.mu.Unlock()

	firstDone := make(chan error, 1)
	go func() {
		_, err := app.ChangeRequestLifecycle(ChangeLifecycleRequest{ChangeRequestID: "change-prod", Confirmation: "change-prod", Action: "pause"})
		firstDone <- err
	}()
	select {
	case <-state.pauseEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("first pause request did not reach server")
	}
	_, secondErr := app.ChangeRequestLifecycle(ChangeLifecycleRequest{ChangeRequestID: "change-prod", Confirmation: "change-prod", Action: "pause"})
	var failure *ActionFailure
	if !errors.As(secondErr, &failure) || failure.Kind != ActionConflict {
		t.Fatalf("concurrent pause error = %T %v, want conflict", secondErr, secondErr)
	}
	close(state.blockPause)
	if err := <-firstDone; err != nil {
		t.Fatalf("first pause: %v", err)
	}
	requests := state.mutationRequests()
	pauseCount := 0
	for _, request := range requests {
		if request.Path == "/v1/admin/change-requests/change-prod/pause" {
			pauseCount++
		}
	}
	if pauseCount != 1 {
		t.Fatalf("pause request count = %d, want 1", pauseCount)
	}
}

func TestChangeControlParityClearsPreparedAdoptionAcrossProfileSwitch(t *testing.T) {
	app, _, profile := newChangeControlParityTestApp(t)
	preview, err := app.ChooseBaselineAdoptionPlan("production")
	if err != nil || preview.PlanID == "" {
		t.Fatalf("prepare baseline adoption: %#v %v", preview, err)
	}
	staging := profile
	staging.Name = "Staging"
	if _, err := app.ConnectProfile(staging); err != nil {
		t.Fatalf("switch profile after adoption preparation: %v", err)
	}
	app.changeControl.mu.Lock()
	pending := app.changeControl.pendingAdoption
	app.changeControl.mu.Unlock()
	if pending != nil {
		t.Fatalf("obsolete profile retained baseline adoption request: %#v", pending)
	}
}

func TestChangeControlAuthorizationViewPreservesServerAcceptedRolloutControls(t *testing.T) {
	view := mapRolloutAuthorizationView(admin.RolloutAuthorization{
		ID: "rollout-prod", ChangeRequestID: "change-prod", Fleet: "production",
		ValidFrom: time.Date(2032, 3, 4, 5, 0, 0, 0, time.UTC), ValidUntil: time.Date(2032, 3, 5, 5, 0, 0, 0, time.UTC),
		AttemptLimit: 2, MaxConcurrency: 1,
		ExecutionWindows: []admin.RecurringWindow{{Weekdays: []time.Weekday{time.Monday, time.Wednesday, time.Friday}, StartMinuteUTC: 120, Duration: 45 * time.Minute}},
		AuthorizedBy:     "operator-a", AuthorizedAt: time.Date(2032, 3, 4, 5, 6, 0, 0, time.UTC), Justification: "CHG-404",
	})
	if view.ID != "rollout-prod" || view.ValidFrom != "2032-03-04T05:00:00Z" || view.ValidUntil != "2032-03-05T05:00:00Z" || view.AttemptLimit != 2 || view.MaxConcurrency != 1 || view.AuthorizedBy != "operator-a" {
		t.Fatalf("rollout authorization view = %#v", view)
	}
	if len(view.ExecutionWindows) != 1 || !slices.Equal(view.ExecutionWindows[0].Weekdays, []int{1, 3, 5}) || view.ExecutionWindows[0].StartMinuteUTC != 120 || view.ExecutionWindows[0].DurationMinutes != 45 {
		t.Fatalf("rollout execution windows = %#v", view.ExecutionWindows)
	}
}

func newChangeControlParityTestApp(t *testing.T) (*App, *changeControlParityState, ConnectionProfile) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	state := &changeControlParityState{lifecycle: "pending"}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		state.mu.Lock()
		forbid := state.forbid
		state.mu.Unlock()
		if forbid && request.URL.Path != "/v1/admin/me" {
			http.Error(response, "forbidden", http.StatusForbidden)
			return
		}

		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me":
			_, _ = response.Write([]byte(`{"operator_id":"operator-change","roles":["global_admin"]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/change-requests/change-prod":
			_, _ = response.Write([]byte(state.changeRequestJSON("change-prod", "production")))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/change-requests/change-prod/authorize":
			state.recordMutation(t, request)
			state.mu.Lock()
			state.approvals = 1
			state.mu.Unlock()
			_, _ = response.Write([]byte(`{}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/change-requests/change-prod/pause":
			state.recordMutation(t, request)
			state.mu.Lock()
			block, entered := state.blockPause, state.pauseEntered
			state.mu.Unlock()
			if block != nil {
				state.pauseReleased.Do(func() { close(entered) })
				<-block
			}
			state.mu.Lock()
			state.lifecycle = "paused"
			state.mu.Unlock()
			_, _ = response.Write([]byte(state.changeRequestJSON("change-prod", "production")))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/change-requests/change-prod/resume":
			state.recordMutation(t, request)
			state.mu.Lock()
			state.lifecycle = "authorized"
			state.mu.Unlock()
			_, _ = response.Write([]byte(state.changeRequestJSON("change-prod", "production")))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/change-requests/change-prod/revoke":
			state.recordMutation(t, request)
			state.mu.Lock()
			state.lifecycle = "revoked"
			state.mu.Unlock()
			_, _ = response.Write([]byte(state.changeRequestJSON("change-prod", "production")))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/change-requests/change-prod/baseline":
			state.recordMutation(t, request)
			_, _ = response.Write([]byte(`{"id":"baseline-prod","change_request_id":"change-prod","fleet":"production","resource_address":"base/firewall","desired_hash":"sha256:firewall","risk":"connectivity","provider":"nftables","authorized_by":"operator-change","authorized_at":"2032-03-04T05:06:07Z"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/fleets/production/baseline-adoptions":
			state.recordMutation(t, request)
			_, _ = response.Write([]byte(state.changeRequestJSON("adoption-prod", "production")))
		default:
			http.NotFound(response, request)
		}
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{fixture.serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    connectionCertPool(t, fixture.caPEM),
		MinVersion:   tls.VersionTLS12,
		Time:         func() time.Time { return connectionTestTime },
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	stateDir := fixture.saveClientState(t, "operator-change", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect Change-control Operator: %v", err)
	}
	app := NewApp("test")
	app.sessions = manager
	app.changeControl.now = func() time.Time { return time.Date(2032, 3, 4, 5, 0, 0, 0, time.UTC) }
	return app, state, profile
}

func (s *changeControlParityState) recordMutation(t *testing.T, request *http.Request) {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Errorf("read %s %s: %v", request.Method, request.URL.Path, err)
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, changeControlRecordedRequest{Method: request.Method, Path: request.URL.Path, Body: slices.Clone(body)})
	s.mu.Unlock()
}

func (s *changeControlParityState) mutationRequests() []changeControlRecordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

func (s *changeControlParityState) changeRequestJSON(id, fleet string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	approvals := ""
	if s.approvals >= 1 {
		approvals = `{"operator_id":"operator-change","approved_at":"2032-03-04T05:05:07Z","justification":"CHG-404 connectivity canary"}`
	}
	if s.approvals >= 2 {
		approvals += `,{"operator_id":"operator-second","approved_at":"2032-03-04T05:05:08Z","justification":"second review"}`
	}
	return `{"id":"` + id + `","fleet":"` + fleet + `","release_ref":"release-42","artifact_digest":"sha256:artifact","authorization_group":"network","risk":"connectivity","authorization_state":"` + s.lifecycle + `","required_approvals":2,"approvals":[` + approvals + `],"created_at":"2032-03-04T05:01:07Z","frozen_targets":[{"endpoint_id":"endpoint-a","compatible":true,"preflight_ready":true},{"endpoint_id":"endpoint-b","compatible":true,"preflight_ready":true}],"resources":[{"address":"base/firewall","desired_hash":"sha256:firewall","risk":"connectivity","provider":"nftables","rollback_class":"automatic","baseline_eligible":true}],"outcomes":{"endpoint-a":{"endpoint_id":"endpoint-a","state":"verified_successful"},"endpoint-b":{"endpoint_id":"endpoint-b","state":"not_seen"}},"audit_history":[{"at":"2032-03-04T05:01:07Z","actor_id":"operator-seed","action":"created"}]}`
}
