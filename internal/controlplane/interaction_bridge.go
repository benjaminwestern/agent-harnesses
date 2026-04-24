package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	interactionrpc "github.com/benjaminwestern/agentic-control/pkg/interaction"
)

type nativeSubscription struct {
	Method            string
	UnsubscribeMethod string
	Subscription      *interactionrpc.Subscription
}

func (s *Service) InteractionCall(ctx context.Context, nativeMethod string, params any) (json.RawMessage, error) {
	if nativeMethod == "" {
		return nil, fmt.Errorf("method is required")
	}
	if isNativeSubscriptionMethod(nativeMethod) {
		return nil, fmt.Errorf("%s is a streaming method; use interaction.subscribe", nativeMethod)
	}
	return s.interaction.Call(ctx, nativeMethod, params)
}

func (s *Service) SubscribeInteraction(ctx context.Context, nativeMethod string, params any) (map[string]any, error) {
	if nativeMethod == "" {
		return nil, fmt.Errorf("method is required")
	}
	unsubscribeMethod := nativeUnsubscribeMethod(nativeMethod)
	if unsubscribeMethod == "" {
		return nil, fmt.Errorf("unsupported interaction subscription method: %s", nativeMethod)
	}

	subscription, err := s.interaction.StartSubscription(ctx, nativeMethod, params, func(method string, params json.RawMessage) {
		s.publishControlEvent(
			"agentic-interaction",
			"",
			"",
			"",
			contract.EventRuntimeEvent,
			method,
			"Received Agentic Interaction event",
			map[string]any{
				"subscription_method": nativeMethod,
				"native_method":       method,
				"native_payload":      rawPayload(params),
			},
		)
	})
	if err != nil {
		return nil, err
	}
	if subscription.SubscriptionID == "" {
		if subscription.Cancel != nil {
			subscription.Cancel()
		}
		return nil, fmt.Errorf("%s returned an empty subscription_id", nativeMethod)
	}

	s.interactionSubMu.Lock()
	s.interactionSubs[subscription.SubscriptionID] = nativeSubscription{
		Method:            nativeMethod,
		UnsubscribeMethod: unsubscribeMethod,
		Subscription:      subscription,
	}
	s.interactionSubMu.Unlock()

	return map[string]any{
		"subscription": rawObject(subscription.Result),
	}, nil
}

func (s *Service) UnsubscribeInteraction(ctx context.Context, subscriptionID string, nativeMethod string) (map[string]any, error) {
	if subscriptionID == "" {
		return nil, fmt.Errorf("subscription_id is required")
	}

	s.interactionSubMu.Lock()
	subscription, ok := s.interactionSubs[subscriptionID]
	delete(s.interactionSubs, subscriptionID)
	s.interactionSubMu.Unlock()

	unsubscribeMethod := subscription.UnsubscribeMethod
	if unsubscribeMethod == "" {
		unsubscribeMethod = nativeUnsubscribeMethod(nativeMethod)
	}
	if !ok && unsubscribeMethod == "" {
		return nil, fmt.Errorf("unknown interaction subscription: %s", subscriptionID)
	}

	var nativeResult map[string]any
	if unsubscribeMethod != "" {
		raw, err := s.interaction.Call(ctx, unsubscribeMethod, interactionrpc.EventUnsubscribeParams{
			SubscriptionID: subscriptionID,
		})
		if err != nil {
			if subscription.Subscription != nil && subscription.Subscription.Cancel != nil {
				subscription.Subscription.Cancel()
			}
			return nil, err
		}
		nativeResult = rawObject(raw)
	}

	if subscription.Subscription != nil && subscription.Subscription.Cancel != nil {
		subscription.Subscription.Cancel()
	}

	return map[string]any{
		"subscription_id": subscriptionID,
		"native_result":   nativeResult,
		"unsubscribed":    true,
	}, nil
}

func isNativeSubscriptionMethod(method string) bool {
	return nativeUnsubscribeMethod(method) != ""
}

func nativeUnsubscribeMethod(method string) string {
	switch method {
	case interactionrpc.MethodSTTEventsSubscribe:
		return interactionrpc.MethodSTTEventsUnsubscribe
	case interactionrpc.MethodObservationEventsSubscribe:
		return interactionrpc.MethodObservationEventsUnsubscribe
	}

	if strings.HasSuffix(method, ".subscribe") {
		candidate := strings.TrimSuffix(method, ".subscribe") + ".unsubscribe"
		if interactionMethodExists(candidate) {
			return candidate
		}
	}

	return ""
}

func interactionMethodExists(method string) bool {
	for _, candidate := range interactionrpc.AllMethods {
		if candidate == method {
			return true
		}
	}
	return false
}

func rawPayload(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}
