package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type speechRouteConfig struct {
	TargetSessionID string
	Mode            string
	Metadata        map[string]any
	SpeakResponses  bool
	TTSParams       map[string]any
}

type speechResponseTurn struct {
	JoinedText strings.Builder
	LatestText string
	StartedAt  time.Time
}

func (s *Service) CallInteraction(ctx context.Context, nativeMethod string, params map[string]any) (json.RawMessage, error) {
	return s.interaction.Call(ctx, nativeMethod, paramsOrNil(params))
}

func (s *Service) EnqueueAttention(item contract.AttentionItem) contract.AttentionItem {
	item = s.attention.Enqueue(item)
	s.publishControlEvent(
		item.Runtime,
		item.SessionID,
		item.TurnID,
		"",
		contract.EventAttentionItemCreated,
		"",
		"Created attention item",
		map[string]any{"attention_item": item},
	)
	return item
}

func (s *Service) ListAttention(filter AttentionListFilter) []contract.AttentionItem {
	return s.attention.List(filter)
}

func (s *Service) UpdateAttention(id string, update AttentionUpdate) (contract.AttentionItem, error) {
	item, ok := s.attention.Update(id, update)
	if !ok {
		return contract.AttentionItem{}, fmt.Errorf("unknown attention item: %s", id)
	}

	eventType := contract.EventAttentionItemUpdated
	switch item.Status {
	case contract.AttentionStatusCompleted, contract.AttentionStatusSpoken, contract.AttentionStatusInserted:
		eventType = contract.EventAttentionItemCompleted
	case contract.AttentionStatusFailed:
		eventType = contract.EventAttentionItemFailed
	}
	s.publishControlEvent(
		item.Runtime,
		item.SessionID,
		item.TurnID,
		"",
		eventType,
		"",
		"Updated attention item",
		map[string]any{"attention_item": item},
	)
	return item, nil
}

func (s *Service) EnqueueTTS(ctx context.Context, params map[string]any) (map[string]any, error) {
	text := firstNonEmptyString(
		stringValue(params, "speakable_text"),
		stringValue(params, "text"),
	)
	if text == "" {
		return nil, errors.New("text is required")
	}

	item := s.EnqueueAttention(contract.AttentionItem{
		Action:          contract.AttentionActionSpeak,
		Status:          contract.AttentionStatusQueued,
		Source:          stringValue(params, "source"),
		Runtime:         firstNonEmptyString(stringValue(params, "runtime"), stringValue(params, "source")),
		SessionID:       stringValue(params, "session_id"),
		TurnID:          stringValue(params, "turn_id"),
		Priority:        intValue(params, "priority"),
		Text:            stringValue(params, "text"),
		SpeakableText:   text,
		Voice:           stringValue(params, "voice"),
		InterruptPolicy: stringValue(params, "interrupt_policy"),
		Metadata:        mapValue(params["metadata"]),
	})

	payload := cloneMap(params)
	payload["attention_item"] = item
	s.publishControlEvent(
		item.Runtime,
		item.SessionID,
		item.TurnID,
		item.ID,
		contract.EventSpeechTTSRequested,
		"tts.speak",
		"Requested speech synthesis",
		payload,
	)

	activeItem, _ := s.attention.Update(item.ID, AttentionUpdate{Status: contract.AttentionStatusActive})
	startPayload := cloneMap(payload)
	startPayload["attention_item"] = activeItem
	s.publishControlEvent(
		item.Runtime,
		item.SessionID,
		item.TurnID,
		item.ID,
		contract.EventSpeechTTSStarted,
		"tts.speak",
		"Started speech synthesis",
		startPayload,
	)

	nativeParams := cloneMap(params)
	nativeParams["text"] = text
	if _, ok := nativeParams["play"]; !ok {
		nativeParams["play"] = true
	}
	raw, err := s.interaction.Call(ctx, "tts.speak", nativeParams)
	if err != nil {
		failedItem, _ := s.attention.Update(item.ID, AttentionUpdate{
			Status: contract.AttentionStatusFailed,
			Error:  err.Error(),
		})
		failurePayload := cloneMap(payload)
		failurePayload["attention_item"] = failedItem
		failurePayload["error"] = err.Error()
		s.publishControlEvent(
			item.Runtime,
			item.SessionID,
			item.TurnID,
			item.ID,
			contract.EventSpeechTTSFailed,
			"tts.speak",
			"Speech synthesis failed",
			failurePayload,
		)
		return nil, err
	}

	nativeResult := rawObject(raw)
	completedItem, _ := s.attention.Update(item.ID, AttentionUpdate{
		Status: contract.AttentionStatusSpoken,
		Result: nativeResult,
	})
	completedPayload := cloneMap(payload)
	completedPayload["attention_item"] = completedItem
	completedPayload["native_result"] = nativeResult
	event := s.publishControlEvent(
		item.Runtime,
		item.SessionID,
		item.TurnID,
		item.ID,
		contract.EventSpeechTTSCompleted,
		"tts.speak",
		"Completed speech synthesis",
		completedPayload,
	)

	return map[string]any{
		"attention_item": completedItem,
		"native_result":  nativeResult,
		"event_id":       event.EventID,
	}, nil
}

func (s *Service) CancelTTS(ctx context.Context, params map[string]any) (map[string]any, error) {
	raw, err := s.interaction.Call(ctx, "tts.stop", paramsOrNil(params))
	if err != nil {
		s.publishControlEvent("", "", "", "", contract.EventSpeechTTSFailed, "tts.stop", "Failed to cancel speech synthesis", map[string]any{"error": err.Error()})
		return nil, err
	}

	var item contract.AttentionItem
	if attentionID := stringValue(params, "attention_id"); attentionID != "" {
		item, _ = s.attention.Update(attentionID, AttentionUpdate{Status: contract.AttentionStatusCancelled})
	}
	result := rawObject(raw)
	payload := cloneMap(params)
	payload["native_result"] = result
	if item.ID != "" {
		payload["attention_item"] = item
	}
	event := s.publishControlEvent(
		item.Runtime,
		item.SessionID,
		item.TurnID,
		item.ID,
		contract.EventSpeechTTSCancelled,
		"tts.stop",
		"Cancelled speech synthesis",
		payload,
	)
	return map[string]any{"native_result": result, "attention_item": item, "event_id": event.EventID}, nil
}

func (s *Service) STTCommand(ctx context.Context, nativeMethod string, params map[string]any) (map[string]any, error) {
	raw, err := s.interaction.Call(ctx, nativeMethod, paramsOrNil(params))
	if err != nil {
		s.publishControlEvent("", "", "", "", contract.EventSpeechSTTFailed, nativeMethod, "Speech transcription command failed", map[string]any{"error": err.Error()})
		return nil, err
	}
	result := rawObject(raw)
	eventType := ""
	switch nativeMethod {
	case "stt.start":
		eventType = contract.EventSpeechSTTStarted
	case "stt.stop":
		eventType = contract.EventSpeechSTTCancelled
	}
	var eventID string
	if eventType != "" {
		event := s.publishControlEvent("", "", "", "", eventType, nativeMethod, "Speech transcription state changed", map[string]any{"native_result": result})
		eventID = event.EventID
	}
	return map[string]any{"native_result": result, "event_id": eventID}, nil
}

func (s *Service) SubmitSTT(ctx context.Context, params map[string]any) (map[string]any, error) {
	text := firstNonEmptyString(stringValue(params, "text"), stringValue(params, "transcript"))
	if text == "" {
		return nil, errors.New("text is required")
	}

	route := routeFromParams(params)
	payload := cloneMap(params)
	payload["text"] = text
	event := s.publishControlEvent(
		firstNonEmptyString(stringValue(params, "runtime"), stringValue(params, "source"), "agentic-speech"),
		stringValue(params, "session_id"),
		stringValue(params, "turn_id"),
		"",
		contract.EventSpeechSTTFinal,
		"speech.stt.submit",
		"Received final speech transcript",
		payload,
	)

	result := map[string]any{
		"event_id": event.EventID,
		"routed":   false,
	}
	if route.TargetSessionID == "" {
		return result, nil
	}

	routedEvent, err := s.routeSpeechTranscript(ctx, route, text, firstNonEmptyString(stringValue(params, "source"), "speech.stt.submit"), event.EventID)
	if err != nil {
		failurePayload := cloneMap(payload)
		failurePayload["error"] = err.Error()
		failurePayload["speech_event_id"] = event.EventID
		s.publishControlEvent(
			firstNonEmptyString(stringValue(params, "runtime"), stringValue(params, "source"), "agentic-speech"),
			stringValue(params, "session_id"),
			stringValue(params, "turn_id"),
			"",
			contract.EventSpeechSTTFailed,
			"session.send",
			"Failed to route speech transcript",
			failurePayload,
		)
		return nil, err
	}
	result["routed"] = routedEvent != nil
	result["routed_event"] = routedEvent
	return result, nil
}

func (s *Service) SubscribeSTT(ctx context.Context, params map[string]any) (map[string]any, error) {
	nativeParams := cloneMap(params)
	if _, ok := nativeParams["kinds"]; !ok {
		nativeParams["kinds"] = []string{"started", "partial", "final", "error", "ended"}
	}

	s.sttMu.Lock()
	existing := s.sttSubscription
	s.sttSubscription = nil
	s.sttMu.Unlock()
	s.clearSpeechResponseTurns()
	if existing != nil && existing.Cancel != nil {
		existing.Cancel()
	}

	route := routeFromParams(params)
	subscription, err := s.interaction.StartSubscription(ctx, "stt.events.subscribe", nativeParams, func(method string, params json.RawMessage) {
		go s.handleSTTNotification(ctx, method, params)
	})
	if err != nil {
		s.publishControlEvent("", "", "", "", contract.EventSpeechSTTFailed, "stt.events.subscribe", "Failed to subscribe to speech transcription events", map[string]any{"error": err.Error()})
		return nil, err
	}

	s.sttMu.Lock()
	s.sttSubscription = subscription
	s.sttRoute = route
	s.sttMu.Unlock()

	var status map[string]any
	if raw, err := s.interaction.Call(ctx, "stt.status", nil); err == nil {
		status = rawObject(raw)
	}
	event := s.publishControlEvent("", "", "", "", contract.EventSpeechSTTStarted, "stt.events.subscribe", "Subscribed to speech transcription events", map[string]any{
		"subscription_id": subscription.SubscriptionID,
		"subscription":    rawObject(subscription.Result),
		"status":          status,
	})

	return map[string]any{
		"subscription": rawObject(subscription.Result),
		"route":        route.ToMap(),
		"status":       status,
		"event_id":     event.EventID,
	}, nil
}

func (s *Service) UnsubscribeSTT(ctx context.Context, params map[string]any) (map[string]any, error) {
	s.sttMu.Lock()
	subscription := s.sttSubscription
	s.sttSubscription = nil
	s.sttRoute = speechRouteConfig{}
	s.sttMu.Unlock()
	s.clearSpeechResponseTurns()

	subscriptionID := stringValue(params, "subscription_id")
	if subscriptionID == "" && subscription != nil {
		subscriptionID = subscription.SubscriptionID
	}

	var nativeResult map[string]any
	if subscriptionID != "" {
		raw, err := s.interaction.Call(ctx, "stt.events.unsubscribe", map[string]any{"subscription_id": subscriptionID})
		if err == nil {
			nativeResult = rawObject(raw)
		}
	}
	if subscription != nil && subscription.Cancel != nil {
		subscription.Cancel()
	}

	event := s.publishControlEvent("", "", "", "", contract.EventSpeechSTTCancelled, "stt.events.unsubscribe", "Unsubscribed from speech transcription events", map[string]any{
		"subscription_id": subscriptionID,
		"native_result":   nativeResult,
	})
	return map[string]any{"subscription_id": subscriptionID, "native_result": nativeResult, "event_id": event.EventID}, nil
}

func (s *Service) OpenApp(ctx context.Context, nativeMethod string, params map[string]any) (map[string]any, error) {
	return s.callNativeWithEvents(ctx, nativeMethod, params, contract.EventAppOpenRequested, contract.EventAppOpenCompleted, contract.EventAppOpenFailed, "Requested native app command")
}

func (s *Service) ListInsertTargets(ctx context.Context, params map[string]any) (map[string]any, error) {
	s.publishControlEvent("", "", "", "", contract.EventInsertTargetsRequested, "accessibility.apps.list", "Requested insertion targets", cloneMap(params))

	appsRaw, err := s.interaction.Call(ctx, "accessibility.apps.list", paramsOrNil(params))
	if err != nil {
		s.publishControlEvent("", "", "", "", contract.EventInsertTargetsFailed, "accessibility.apps.list", "Failed to list insertion targets", map[string]any{"error": err.Error()})
		return nil, err
	}
	targetsRaw, err := s.interaction.Call(ctx, "accessibility.targets.list", paramsOrNil(params))
	if err != nil {
		s.publishControlEvent("", "", "", "", contract.EventInsertTargetsFailed, "accessibility.targets.list", "Failed to list insertion targets", map[string]any{"error": err.Error()})
		return nil, err
	}

	result := rawObject(appsRaw)
	if result == nil {
		result = make(map[string]any)
	}
	targets := rawObject(targetsRaw)
	result["targets"] = targets
	if candidates, ok := targets["candidates"]; ok {
		result["target_candidates"] = candidates
	}
	event := s.publishControlEvent("", "", "", "", contract.EventInsertTargetsCompleted, "accessibility.apps.list", "Listed insertion targets", map[string]any{"result": result})
	result["event_id"] = event.EventID
	return result, nil
}

func (s *Service) ObserveScreen(ctx context.Context, params map[string]any) (map[string]any, error) {
	s.publishControlEvent("", "", "", "", contract.EventScreenObserveRequested, "observation.screenshot", "Requested screen observation", cloneMap(params))

	includeAX := boolValue(params, "include_ax_inventory")
	recording := recordingRequested(params)
	includeScreenshot := true
	if value, ok := params["include_screenshot"]; ok {
		includeScreenshot = anyBool(value)
	}

	result := make(map[string]any)
	if includeAX {
		raw, err := s.interaction.Call(ctx, "accessibility.apps.list", paramsOrNil(params))
		if err != nil {
			s.publishControlEvent("", "", "", "", contract.EventScreenObserveFailed, "accessibility.apps.list", "Failed to observe accessibility inventory", map[string]any{"error": err.Error()})
			return nil, err
		}
		result["ax_inventory"] = rawObject(raw)
	}

	if recording || includeScreenshot {
		nativeMethod := "observation.screenshot"
		nativeParams := observationParams(params)
		if recording {
			nativeMethod = "observation.recording.start"
		}
		raw, err := s.interaction.Call(ctx, nativeMethod, nativeParams)
		if err != nil {
			s.publishControlEvent("", "", "", "", contract.EventScreenObserveFailed, nativeMethod, "Failed to observe screen", map[string]any{"error": err.Error()})
			return nil, err
		}
		result["observation"] = rawObject(raw)
		result["native_method"] = nativeMethod
	}

	event := s.publishControlEvent("", "", "", "", contract.EventScreenObserveCompleted, stringValue(result, "native_method"), "Completed screen observation", map[string]any{"result": result})
	result["event_id"] = event.EventID
	return result, nil
}

func (s *Service) ClickScreen(ctx context.Context, params map[string]any) (map[string]any, error) {
	return s.callNativeWithEvents(ctx, "screen.click", params, contract.EventScreenClickRequested, contract.EventScreenClickCompleted, contract.EventScreenClickFailed, "Requested native screen click")
}

func (s *Service) EnqueueInsert(ctx context.Context, params map[string]any) (map[string]any, error) {
	text := firstNonEmptyString(stringValue(params, "text"), stringValue(params, "content"))
	if text == "" {
		return nil, errors.New("text is required")
	}

	target := mapValue(params["target"])
	item := s.EnqueueAttention(contract.AttentionItem{
		Action:    contract.AttentionActionInsert,
		Status:    contract.AttentionStatusQueued,
		Source:    stringValue(params, "source"),
		Runtime:   firstNonEmptyString(stringValue(params, "runtime"), stringValue(params, "source")),
		SessionID: stringValue(params, "session_id"),
		TurnID:    stringValue(params, "turn_id"),
		Priority:  intValue(params, "priority"),
		Text:      text,
		Target:    target,
		Metadata:  mapValue(params["metadata"]),
	})

	requestPayload := cloneMap(params)
	requestPayload["attention_item"] = item
	s.publishControlEvent(item.Runtime, item.SessionID, item.TurnID, item.ID, contract.EventSpeechInsertRequested, "insert.enqueue", "Requested text insertion", requestPayload)

	activeItem, _ := s.attention.Update(item.ID, AttentionUpdate{Status: contract.AttentionStatusActive})
	startPayload := cloneMap(requestPayload)
	startPayload["attention_item"] = activeItem
	s.publishControlEvent(item.Runtime, item.SessionID, item.TurnID, item.ID, contract.EventSpeechInsertStarted, "insert.enqueue", "Started text insertion", startPayload)

	nativeMethod, nativeParams, err := nativeInsertRequest(params)
	if err != nil {
		failedItem, _ := s.attention.Update(item.ID, AttentionUpdate{Status: contract.AttentionStatusFailed, Error: err.Error()})
		failurePayload := cloneMap(requestPayload)
		failurePayload["attention_item"] = failedItem
		failurePayload["error"] = err.Error()
		s.publishControlEvent(item.Runtime, item.SessionID, item.TurnID, item.ID, contract.EventSpeechInsertFailed, "insert.enqueue", "Text insertion failed", failurePayload)
		return nil, err
	}

	raw, err := s.interaction.Call(ctx, nativeMethod, nativeParams)
	if err != nil {
		failedItem, _ := s.attention.Update(item.ID, AttentionUpdate{Status: contract.AttentionStatusFailed, Error: err.Error()})
		failurePayload := cloneMap(requestPayload)
		failurePayload["attention_item"] = failedItem
		failurePayload["native_method"] = nativeMethod
		failurePayload["error"] = err.Error()
		s.publishControlEvent(item.Runtime, item.SessionID, item.TurnID, item.ID, contract.EventSpeechInsertFailed, nativeMethod, "Text insertion failed", failurePayload)
		return nil, err
	}

	nativeResult := rawObject(raw)
	completedItem, _ := s.attention.Update(item.ID, AttentionUpdate{
		Status: contract.AttentionStatusInserted,
		Result: nativeResult,
	})
	completedPayload := cloneMap(requestPayload)
	completedPayload["attention_item"] = completedItem
	completedPayload["native_method"] = nativeMethod
	completedPayload["native_result"] = nativeResult
	event := s.publishControlEvent(item.Runtime, item.SessionID, item.TurnID, item.ID, contract.EventSpeechInsertCompleted, nativeMethod, "Completed text insertion", completedPayload)
	return map[string]any{
		"attention_item": completedItem,
		"native_method":  nativeMethod,
		"native_result":  nativeResult,
		"event_id":       event.EventID,
	}, nil
}

func (s *Service) callNativeWithEvents(
	ctx context.Context,
	nativeMethod string,
	params map[string]any,
	requestedEvent string,
	completedEvent string,
	failedEvent string,
	summary string,
) (map[string]any, error) {
	runtime := firstNonEmptyString(stringValue(params, "runtime"), stringValue(params, "source"))
	sessionID := stringValue(params, "session_id")
	turnID := stringValue(params, "turn_id")
	s.publishControlEvent(runtime, sessionID, turnID, "", requestedEvent, nativeMethod, summary, cloneMap(params))

	raw, err := s.interaction.Call(ctx, nativeMethod, paramsOrNil(params))
	if err != nil {
		payload := cloneMap(params)
		payload["error"] = err.Error()
		s.publishControlEvent(runtime, sessionID, turnID, "", failedEvent, nativeMethod, summary+" failed", payload)
		return nil, err
	}
	result := rawObject(raw)
	event := s.publishControlEvent(runtime, sessionID, turnID, "", completedEvent, nativeMethod, summary+" completed", map[string]any{
		"request":       cloneMap(params),
		"native_result": result,
	})
	if result == nil {
		result = make(map[string]any)
	}
	result["event_id"] = event.EventID
	return result, nil
}

func (s *Service) handleSTTNotification(ctx context.Context, nativeMethod string, rawParams json.RawMessage) {
	var params map[string]any
	if err := json.Unmarshal(rawParams, &params); err != nil {
		s.publishControlEvent("agentic-interaction", "", "", "", contract.EventSpeechSTTFailed, nativeMethod, "Failed to decode speech transcription event", map[string]any{"error": err.Error()})
		return
	}

	nativeEvent := mapValue(params["event"])
	kind := strings.ToLower(firstNonEmptyString(stringValue(nativeEvent, "kind"), stringValue(params, "kind")))
	text := firstNonEmptyString(stringValue(nativeEvent, "transcript"), stringValue(nativeEvent, "text"))
	eventType := sttEventType(kind)
	payload := map[string]any{
		"source":          "agentic-interaction",
		"subscription_id": firstNonEmptyString(stringValue(params, "subscription_id"), stringValue(params, "subscriptionId")),
		"kind":            kind,
		"text":            text,
		"native_event":    nativeEvent,
		"native_payload":  params,
	}

	s.sttMu.Lock()
	route := s.sttRoute
	s.sttMu.Unlock()
	if route.TargetSessionID != "" {
		payload["route"] = map[string]any{
			"target_session_id": route.TargetSessionID,
			"mode":              route.Mode,
		}
	}

	event := s.publishControlEvent("agentic-interaction", "", "", "", eventType, nativeMethod, sttSummary(kind, text), payload)
	if eventType != contract.EventSpeechSTTFinal || route.TargetSessionID == "" {
		return
	}
	if _, err := s.routeSpeechTranscript(ctx, route, text, "agentic-interaction", event.EventID); err != nil {
		s.publishControlEvent("agentic-interaction", "", "", "", contract.EventSpeechSTTFailed, "session.send", "Failed to route speech transcript", map[string]any{
			"error":           err.Error(),
			"speech_event_id": event.EventID,
			"route": map[string]any{
				"target_session_id": route.TargetSessionID,
				"mode":              route.Mode,
			},
		})
	}
}

func (s *Service) routeSpeechTranscript(ctx context.Context, route speechRouteConfig, text string, source string, speechEventID string) (*contract.RuntimeEvent, error) {
	if route.TargetSessionID == "" {
		return nil, nil
	}
	mode := strings.ToLower(strings.TrimSpace(route.Mode))
	if mode == "" {
		mode = "send"
	}
	if mode != "send" {
		return nil, nil
	}
	metadata := cloneMap(route.Metadata)
	metadata["source"] = source
	metadata["speech_event_id"] = speechEventID
	return s.SendInput(ctx, api.SendInputRequest{
		SessionID: route.TargetSessionID,
		Text:      text,
		Metadata:  metadata,
	})
}

func (s *Service) handleSpeechResponseEvent(event contract.RuntimeEvent) {
	s.sttMu.Lock()
	route := s.sttRoute
	s.sttMu.Unlock()
	if !route.SpeakResponses || route.TargetSessionID == "" || event.SessionID != route.TargetSessionID {
		return
	}

	switch event.EventType {
	case contract.EventAssistantMessageDelta:
		s.recordSpeechResponseDelta(event)
	case contract.EventTurnCompleted:
		text := s.completeSpeechResponseTurn(event)
		if text == "" {
			return
		}
		go s.speakSpeechResponse(route, event, text)
	case contract.EventTurnErrored, contract.EventTurnInterrupted, contract.EventSessionStopped, contract.EventSessionErrored:
		s.deleteSpeechResponseTurn(event.SessionID, event.TurnID)
	}
}

func (s *Service) recordSpeechResponseDelta(event contract.RuntimeEvent) {
	if event.SessionID == "" || event.TurnID == "" {
		return
	}
	text := contract.EventDeltaText(event)
	if text == "" {
		return
	}
	key := speechResponseKey(event.SessionID, event.TurnID)

	s.speechResponseMu.Lock()
	defer s.speechResponseMu.Unlock()
	turn := s.speechResponses[key]
	if turn == nil {
		turn = &speechResponseTurn{StartedAt: time.Now()}
		s.speechResponses[key] = turn
	}
	if event.NativeEventName == "message.part.updated" {
		turn.LatestText = strings.TrimSpace(text)
		return
	}
	turn.JoinedText.WriteString(text)
	turn.LatestText = strings.TrimSpace(turn.JoinedText.String())
}

func (s *Service) completeSpeechResponseTurn(event contract.RuntimeEvent) string {
	if event.SessionID == "" || event.TurnID == "" {
		return speechResponseTextFromEvent(event)
	}
	key := speechResponseKey(event.SessionID, event.TurnID)

	s.speechResponseMu.Lock()
	turn := s.speechResponses[key]
	delete(s.speechResponses, key)
	s.speechResponseMu.Unlock()

	if turn != nil {
		if text := strings.TrimSpace(turn.LatestText); text != "" {
			return text
		}
		if text := strings.TrimSpace(turn.JoinedText.String()); text != "" {
			return text
		}
	}
	return speechResponseTextFromEvent(event)
}

func (s *Service) speakSpeechResponse(route speechRouteConfig, event contract.RuntimeEvent, text string) {
	params := cloneMap(route.TTSParams)
	params["text"] = text
	params["speakable_text"] = text
	params["source"] = event.Runtime
	params["runtime"] = event.Runtime
	params["session_id"] = event.SessionID
	params["turn_id"] = event.TurnID
	if _, ok := params["metadata"]; !ok {
		params["metadata"] = map[string]any{}
	}
	metadata := mapValue(params["metadata"])
	if metadata == nil {
		metadata = map[string]any{}
		params["metadata"] = metadata
	}
	metadata["reason"] = firstNonEmptyString(stringValue(metadata, "reason"), "stt_route_response")
	metadata["response_event_id"] = event.EventID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := s.EnqueueTTS(ctx, params); err != nil {
		s.publishControlEvent(event.Runtime, event.SessionID, event.TurnID, "", contract.EventSpeechTTSFailed, "speech.stt.route.response", "Failed to speak routed assistant response", map[string]any{
			"error":             err.Error(),
			"response_event_id": event.EventID,
		})
	}
}

func (s *Service) deleteSpeechResponseTurn(sessionID string, turnID string) {
	if sessionID == "" || turnID == "" {
		return
	}
	s.speechResponseMu.Lock()
	delete(s.speechResponses, speechResponseKey(sessionID, turnID))
	s.speechResponseMu.Unlock()
}

func (s *Service) clearSpeechResponseTurns() {
	s.speechResponseMu.Lock()
	s.speechResponses = make(map[string]*speechResponseTurn)
	s.speechResponseMu.Unlock()
}

func speechResponseKey(sessionID string, turnID string) string {
	return sessionID + "\x00" + turnID
}

func speechResponseTextFromEvent(event contract.RuntimeEvent) string {
	if text := contract.EventFinalText(event); text != "" {
		return text
	}
	summary := strings.TrimSpace(event.Summary)
	const openCodePrefix = "OpenCode turn completed: "
	if strings.HasPrefix(summary, openCodePrefix) {
		return strings.TrimSpace(strings.TrimPrefix(summary, openCodePrefix))
	}
	return ""
}

func (s *Service) publishControlEvent(
	runtime string,
	sessionID string,
	turnID string,
	requestID string,
	eventType string,
	nativeEventName string,
	summary string,
	payload map[string]any,
) contract.RuntimeEvent {
	if runtime == "" {
		runtime = firstNonEmptyString(stringValue(payload, "runtime"), stringValue(payload, "source"), "agentic-control")
	}
	if summary == "" {
		summary = eventType
	}
	event := contract.RuntimeEvent{
		SchemaVersion:   contract.ControlPlaneSchemaVersion,
		EventID:         newIdentifier("evt"),
		RecordedAtMS:    time.Now().UnixMilli(),
		Runtime:         runtime,
		SessionID:       sessionID,
		Transport:       contract.TransportRPC,
		Ownership:       contract.OwnershipControlled,
		EventType:       eventType,
		NativeEventName: nativeEventName,
		Summary:         summary,
		TurnID:          turnID,
		RequestID:       requestID,
		Payload:         payload,
	}
	s.PublishEvent(event)
	return event
}

func sttEventType(kind string) string {
	switch kind {
	case "started", "status":
		return contract.EventSpeechSTTStarted
	case "partial":
		return contract.EventSpeechSTTPartial
	case "final":
		return contract.EventSpeechSTTFinal
	case "error":
		return contract.EventSpeechSTTFailed
	case "ended", "cancelled", "canceled":
		return contract.EventSpeechSTTCancelled
	default:
		return contract.EventSpeechSTTPartial
	}
}

func sttSummary(kind string, text string) string {
	switch kind {
	case "partial":
		return "Received partial speech transcript"
	case "final":
		return "Received final speech transcript"
	case "error":
		return "Speech transcription failed"
	case "ended":
		return "Speech transcription ended"
	case "started", "status":
		return "Speech transcription started"
	default:
		if text != "" {
			return "Received speech transcription event"
		}
		return "Speech transcription event"
	}
}

func routeFromParams(params map[string]any) speechRouteConfig {
	routeMap := mapValue(params["route"])
	metadata := cloneMap(mapValue(params["metadata"]))
	if routeMetadata := mapValue(routeMap["metadata"]); routeMetadata != nil {
		for key, value := range routeMetadata {
			metadata[key] = value
		}
	}
	ttsParams := cloneMap(mapValue(params["response_tts"]))
	if routeTTS := mapValue(routeMap["tts"]); routeTTS != nil {
		ttsParams = cloneMap(routeTTS)
	}
	if response := mapValue(routeMap["response"]); response != nil {
		if responseTTS := mapValue(response["tts"]); responseTTS != nil {
			ttsParams = cloneMap(responseTTS)
		}
	}
	return speechRouteConfig{
		TargetSessionID: firstNonEmptyString(
			stringValue(routeMap, "target_session_id"),
			stringValue(routeMap, "targetSessionID"),
			stringValue(params, "target_session_id"),
		),
		Mode:           strings.ToLower(firstNonEmptyString(stringValue(routeMap, "mode"), stringValue(params, "route_mode"))),
		Metadata:       metadata,
		SpeakResponses: routeSpeakResponses(params, routeMap),
		TTSParams:      ttsParams,
	}
}

func routeSpeakResponses(params map[string]any, routeMap map[string]any) bool {
	if boolValue(params, "speak_responses") ||
		boolValue(params, "speakResponses") ||
		boolValue(params, "tts_responses") ||
		boolValue(routeMap, "speak_responses") ||
		boolValue(routeMap, "speakResponses") ||
		boolValue(routeMap, "tts_responses") {
		return true
	}
	if mapValue(params["response_tts"]) != nil || mapValue(routeMap["tts"]) != nil {
		return true
	}
	response := mapValue(routeMap["response"])
	if response == nil {
		return false
	}
	return boolValue(response, "speak") || boolValue(response, "tts") || mapValue(response["tts"]) != nil
}

func (r speechRouteConfig) ToMap() map[string]any {
	result := map[string]any{}
	if r.TargetSessionID != "" {
		result["target_session_id"] = r.TargetSessionID
	}
	if r.Mode != "" {
		result["mode"] = r.Mode
	}
	if len(r.Metadata) > 0 {
		result["metadata"] = cloneMap(r.Metadata)
	}
	if r.SpeakResponses {
		result["speak_responses"] = true
	}
	if len(r.TTSParams) > 0 {
		result["tts"] = cloneMap(r.TTSParams)
	}
	return result
}

func nativeInsertRequest(params map[string]any) (string, map[string]any, error) {
	text := firstNonEmptyString(stringValue(params, "text"), stringValue(params, "content"))
	target := mapValue(params["target"])
	x, hasX := firstNumber(target, params, "x")
	y, hasY := firstNumber(target, params, "y")
	if strings.EqualFold(stringValue(target, "mode"), "coordinate") || (hasX && hasY) {
		if !hasX || !hasY {
			return "", nil, errors.New("x and y are required for coordinate insertion")
		}
		return "accessibility.click_insert", map[string]any{
			"text": text,
			"x":    x,
			"y":    y,
		}, nil
	}

	nativeParams := map[string]any{"text": text}
	copyFirstString(nativeParams, "target_path", target, params, "target_path", "path", "ax_path")
	copyFirstString(nativeParams, "target_bundle_id", target, params, "target_bundle_id", "bundle_id")
	copyFirstString(nativeParams, "target_app_name", target, params, "target_app_name", "app_name")
	copyFirstString(nativeParams, "target_window_title", target, params, "target_window_title", "window_title", "window_title_contains")
	if pid, ok := firstInt(target, params, "target_pid", "pid"); ok {
		nativeParams["target_pid"] = pid
	}
	if mode := firstNonEmptyString(stringValue(params, "native_mode"), stringValue(target, "native_mode")); mode != "" {
		nativeParams["mode"] = mode
	}
	return "accessibility.insert", nativeParams, nil
}

func recordingRequested(params map[string]any) bool {
	if mapValue(params["recording"]) != nil {
		return true
	}
	for _, key := range []string{"record_for_seconds", "recordForSeconds", "fps", "countdown_seconds", "countdownSeconds"} {
		if _, ok := params[key]; ok {
			return true
		}
	}
	return false
}

func observationParams(params map[string]any) map[string]any {
	if recording := mapValue(params["recording"]); recording != nil {
		nativeParams := cloneMap(recording)
		if _, ok := nativeParams["target"]; !ok {
			if target := mapValue(params["target"]); target != nil {
				nativeParams["target"] = target
			}
		}
		return nativeParams
	}
	nativeParams := cloneMap(params)
	for _, key := range []string{"include_ax_inventory", "include_screenshot", "recording"} {
		delete(nativeParams, key)
	}
	return nativeParams
}

func rawObject(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{"value": string(raw)}
	}
	return value
}

func paramsOrNil(params map[string]any) any {
	if len(params) == 0 {
		return nil
	}
	return params
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any)
	for key, value := range input {
		output[key] = value
	}
	return output
}

func mapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case nil:
		return nil
	default:
		return nil
	}
}

func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func intValue(values map[string]any, key string) int {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func boolValue(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	return anyBool(values[key])
}

func anyBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || typed == "1"
	default:
		return false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNumber(primary map[string]any, fallback map[string]any, key string) (float64, bool) {
	if value, ok := numberValue(primary, key); ok {
		return value, true
	}
	return numberValue(fallback, key)
}

func numberValue(values map[string]any, key string) (float64, bool) {
	if values == nil {
		return 0, false
	}
	switch value := values[key].(type) {
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case float64:
		return value, true
	case float32:
		return float64(value), true
	default:
		return 0, false
	}
}

func firstInt(primary map[string]any, fallback map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := numberValue(primary, key); ok {
			return int(value), true
		}
		if value, ok := numberValue(fallback, key); ok {
			return int(value), true
		}
	}
	return 0, false
}

func copyFirstString(target map[string]any, targetKey string, primary map[string]any, fallback map[string]any, keys ...string) {
	for _, key := range keys {
		if value := stringValue(primary, key); value != "" {
			target[targetKey] = value
			return
		}
		if value := stringValue(fallback, key); value != "" {
			target[targetKey] = value
			return
		}
	}
}

func ptrTo[T any](value T) *T {
	return &value
}
