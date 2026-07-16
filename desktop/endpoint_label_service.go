package main

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/endpointlabel"
)

type EndpointLabelSetRequest struct {
	EndpointID string `json:"endpointId"`
	Key        string `json:"key"`
	Value      string `json:"value"`
}

type EndpointLabelRemoveRequest struct {
	EndpointID string `json:"endpointId"`
	Key        string `json:"key"`
}

type EndpointLabelResultView struct {
	Effect     string      `json:"effect"`
	EndpointID string      `json:"endpointId"`
	Key        string      `json:"key"`
	Value      string      `json:"value"`
	Labels     []LabelView `json:"labels"`
}

type EndpointLabelService struct{}

func NewEndpointLabelService() *EndpointLabelService {
	return &EndpointLabelService{}
}

func (s *EndpointLabelService) SetConnected(ctx context.Context, client *admin.Client, request EndpointLabelSetRequest) (EndpointLabelResultView, error) {
	if err := validateEndpointLabelTarget(request.EndpointID, request.Key); err != nil {
		return EndpointLabelResultView{}, err
	}
	if err := endpointlabel.ValidateValue(request.Value); err != nil {
		return EndpointLabelResultView{}, endpointLabelValidationFailure(err.Error())
	}
	if client == nil {
		return EndpointLabelResultView{}, ErrSessionNotConnected
	}

	endpoint, err := client.GetEndpointContext(ctx, request.EndpointID)
	if err != nil {
		return EndpointLabelResultView{}, err
	}
	if endpoint.ID != request.EndpointID {
		return EndpointLabelResultView{}, errors.New("server returned a different Endpoint for the Label target")
	}
	_, existed := endpoint.Labels[request.Key]

	result, err := client.SetEndpointLabelContext(ctx, request.EndpointID, request.Key, request.Value)
	if err != nil {
		return EndpointLabelResultView{}, err
	}
	if result.Key != request.Key || result.Value != request.Value {
		return EndpointLabelResultView{}, errors.New("server returned mismatched Endpoint Label metadata")
	}

	effect := "added"
	if existed {
		effect = "replaced"
	}
	return EndpointLabelResultView{
		Effect:     effect,
		EndpointID: request.EndpointID,
		Key:        request.Key,
		Value:      request.Value,
		Labels:     endpointLabelsView(result.Labels),
	}, nil
}

func (s *EndpointLabelService) RemoveConnected(ctx context.Context, client *admin.Client, request EndpointLabelRemoveRequest) (EndpointLabelResultView, error) {
	if err := validateEndpointLabelTarget(request.EndpointID, request.Key); err != nil {
		return EndpointLabelResultView{}, err
	}
	if client == nil {
		return EndpointLabelResultView{}, ErrSessionNotConnected
	}
	if err := client.DeleteEndpointLabelContext(ctx, request.EndpointID, request.Key); err != nil {
		return EndpointLabelResultView{}, err
	}

	endpoint, err := client.GetEndpointContext(ctx, request.EndpointID)
	if err != nil {
		return EndpointLabelResultView{}, err
	}
	if endpoint.ID != request.EndpointID {
		return EndpointLabelResultView{}, errors.New("server returned a different Endpoint after Label removal")
	}
	return EndpointLabelResultView{
		Effect:     "removed",
		EndpointID: request.EndpointID,
		Key:        request.Key,
		Labels:     endpointLabelsView(endpoint.Labels),
	}, nil
}

func validateEndpointLabelTarget(endpointID, key string) error {
	if endpointID == "" || strings.TrimSpace(endpointID) != endpointID {
		return endpointLabelValidationFailure("Select one exact Endpoint before changing a Label.")
	}
	if err := endpointlabel.ValidateKey(key); err != nil {
		return endpointLabelValidationFailure(err.Error())
	}
	return nil
}

func endpointLabelsView(labels map[string]string) []LabelView {
	view := make([]LabelView, 0, len(labels))
	for key, value := range labels {
		view = append(view, LabelView{Key: key, Value: value})
	}
	slices.SortFunc(view, func(left, right LabelView) int {
		return strings.Compare(left.Key, right.Key)
	})
	return view
}

func endpointLabelValidationFailure(guidance string) error {
	return &ActionFailure{
		Kind:      ActionValidation,
		Message:   "The Endpoint Label request is invalid.",
		Guidance:  guidance,
		Retryable: false,
	}
}
