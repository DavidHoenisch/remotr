package server

import "net/http"

type fastPathMutationClass string

const (
	mutationRelease            fastPathMutationClass = "release"
	mutationSchedule           fastPathMutationClass = "schedule"
	mutationFleetPolicy        fastPathMutationClass = "fleet_policy"
	mutationFleetUpgrade       fastPathMutationClass = "fleet_upgrade"
	mutationEndpointEnrollment fastPathMutationClass = "endpoint_enrollment"
	mutationEndpointDelete     fastPathMutationClass = "endpoint_delete"
	mutationEndpointReassign   fastPathMutationClass = "endpoint_reassign"
	mutationEndpointLabels     fastPathMutationClass = "endpoint_labels"
	mutationEndpointUpgrade    fastPathMutationClass = "endpoint_upgrade"
	mutationDiagnostics        fastPathMutationClass = "diagnostics"
	mutationChangeControl      fastPathMutationClass = "change_control"
	mutationSecretLifecycle    fastPathMutationClass = "secret_lifecycle"
	mutationEnrollmentToken    fastPathMutationClass = "enrollment_token"
	mutationDeploymentToken    fastPathMutationClass = "deployment_token"
)

var fastPathMutationScopes = map[fastPathMutationClass]cacheScope{
	mutationRelease: cacheScopeGlobal, mutationSchedule: cacheScopeGlobal,
	mutationFleetPolicy: cacheScopeFleet, mutationFleetUpgrade: cacheScopeFleet,
	mutationEndpointEnrollment: cacheScopeEndpoint, mutationEndpointDelete: cacheScopeEndpoint,
	mutationEndpointReassign: cacheScopeEndpoint, mutationEndpointLabels: cacheScopeEndpoint,
	mutationEndpointUpgrade: cacheScopeEndpoint, mutationDiagnostics: cacheScopeEndpoint,
	mutationChangeControl: cacheScopeGlobal, mutationSecretLifecycle: cacheScopeGlobal,
	mutationEnrollmentToken: cacheScopeGlobal, mutationDeploymentToken: cacheScopeGlobal,
}

func (s *Server) beginFastPathMutation(class fastPathMutationClass, key string) func() {
	complete, _ := s.beginFastPathMutationChecked(class, key)
	if complete == nil {
		return func() {}
	}
	return complete
}

func (s *Server) beginFastPathMutationChecked(class fastPathMutationClass, key string) (func(), error) {
	scope, known := fastPathMutationScopes[class]
	if !known {
		scope = cacheScopeGlobal
	}
	return s.fastPath.beginMutationChecked(scope, key)
}

func (s *Server) beginFastPathMutationHTTP(w http.ResponseWriter, class fastPathMutationClass, key string) (func(), bool) {
	complete, err := s.beginFastPathMutationChecked(class, key)
	if err != nil {
		http.Error(w, "shared cache coordinator unavailable", http.StatusServiceUnavailable)
		return nil, false
	}
	return complete, true
}
