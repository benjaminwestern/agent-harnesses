package interaction

import (
	"context"
	"encoding/json"
)

const (
	MethodDiagnosticsStatus            = "diagnostics.status"
	MethodDiagnosticsList              = "diagnostics.list"
	MethodDiagnosticsLog               = "diagnostics.log"
	MethodDiagnosticsAcknowledge       = "diagnostics.acknowledge"
	MethodDiagnosticsResolve           = "diagnostics.resolve"
	MethodDiagnosticsRepair            = "diagnostics.repair"
	MethodDiagnosticsClear             = "diagnostics.clear"
	MethodDiagnosticsSupportReport     = "diagnostics.support_report"
	MethodDiagnosticsEventsSubscribe   = "diagnostics.events.subscribe"
	MethodDiagnosticsEventsUnsubscribe = "diagnostics.events.unsubscribe"

	MethodSTTRealtimeModelsList           = "stt.realtime.models.list"
	MethodSTTRealtimeModelGet             = "stt.realtime.model.get"
	MethodSTTRealtimeModelSet             = "stt.realtime.model.set"
	MethodSTTRealtimeModelDownload        = "stt.realtime.model.download"
	MethodSTTRealtimePrewarm              = "stt.realtime.prewarm"
	MethodSTTRealtimeFileTranscribeStart  = "stt.realtime.file_transcribe.start"
	MethodSTTRealtimeFileTranscribeStatus = "stt.realtime.file_transcribe.status"
	MethodSTTRealtimeFileTranscribeResult = "stt.realtime.file_transcribe.result"
	MethodSTTRealtimeFileTranscribeCancel = "stt.realtime.file_transcribe.cancel"
	MethodSTTRealtimeFileTranscribeList   = "stt.realtime.file_transcribe.list"

	MethodSTTBatchModelsList       = "stt.batch.models.list"
	MethodSTTBatchModelGet         = "stt.batch.model.get"
	MethodSTTBatchModelSet         = "stt.batch.model.set"
	MethodSTTBatchModelDownload    = "stt.batch.model.download"
	MethodSTTBatchModelEnsure      = "stt.batch.model.ensure"
	MethodSTTBatchTranscribeStart  = "stt.batch.transcribe.start"
	MethodSTTBatchTranscribeStatus = "stt.batch.transcribe.status"
	MethodSTTBatchTranscribeResult = "stt.batch.transcribe.result"
	MethodSTTBatchTranscribeCancel = "stt.batch.transcribe.cancel"
	MethodSTTBatchTranscribeList   = "stt.batch.transcribe.list"

	MethodTTSPlay    = "tts.play"
	MethodTTSPause   = "tts.pause"
	MethodTTSResume  = "tts.resume"
	MethodTTSRestart = "tts.restart"

	MethodAccessibilityContextCapture       = "accessibility.context.capture"
	MethodAccessibilityContextScreen        = "accessibility.context.screen"
	MethodAccessibilityFind                 = "accessibility.find"
	MethodAccessibilityActionPerform        = "accessibility.action.perform"
	MethodAccessibilitySelectionSet         = "accessibility.selection.set"
	MethodAccessibilityTextReplace          = "accessibility.text.replace"
	MethodAccessibilityTextInspect          = "accessibility.text.inspect"
	MethodAccessibilityDiagnostics          = "accessibility.diagnostics"
	MethodAccessibilityTargetOverlayOpen    = "accessibility.target_overlay.open"
	MethodAccessibilityTargetHighlight      = "accessibility.target.highlight"
	MethodAccessibilityTargetProfilesList   = "accessibility.target_profiles.list"
	MethodAccessibilityTargetProfilesSave   = "accessibility.target_profiles.save"
	MethodAccessibilityTargetProfilesApply  = "accessibility.target_profiles.apply"
	MethodAccessibilityTargetProfilesDelete = "accessibility.target_profiles.delete"

	MethodObservationTargetHighlight = "observation.target.highlight"

	MethodAppsInstalledList = "apps.installed.list"
	MethodAppsFind          = "apps.find"
)

var extendedMethods = []string{
	MethodDiagnosticsStatus,
	MethodDiagnosticsList,
	MethodDiagnosticsLog,
	MethodDiagnosticsAcknowledge,
	MethodDiagnosticsResolve,
	MethodDiagnosticsRepair,
	MethodDiagnosticsClear,
	MethodDiagnosticsSupportReport,
	MethodDiagnosticsEventsSubscribe,
	MethodDiagnosticsEventsUnsubscribe,
	MethodSTTRealtimeModelsList,
	MethodSTTRealtimeModelGet,
	MethodSTTRealtimeModelSet,
	MethodSTTRealtimeModelDownload,
	MethodSTTRealtimePrewarm,
	MethodSTTRealtimeFileTranscribeStart,
	MethodSTTRealtimeFileTranscribeStatus,
	MethodSTTRealtimeFileTranscribeResult,
	MethodSTTRealtimeFileTranscribeCancel,
	MethodSTTRealtimeFileTranscribeList,
	MethodSTTBatchModelsList,
	MethodSTTBatchModelGet,
	MethodSTTBatchModelSet,
	MethodSTTBatchModelDownload,
	MethodSTTBatchModelEnsure,
	MethodSTTBatchTranscribeStart,
	MethodSTTBatchTranscribeStatus,
	MethodSTTBatchTranscribeResult,
	MethodSTTBatchTranscribeCancel,
	MethodSTTBatchTranscribeList,
	MethodTTSPlay,
	MethodTTSPause,
	MethodTTSResume,
	MethodTTSRestart,
	MethodAccessibilityContextCapture,
	MethodAccessibilityContextScreen,
	MethodAccessibilityFind,
	MethodAccessibilityActionPerform,
	MethodAccessibilitySelectionSet,
	MethodAccessibilityTextReplace,
	MethodAccessibilityTextInspect,
	MethodAccessibilityDiagnostics,
	MethodAccessibilityTargetOverlayOpen,
	MethodAccessibilityTargetHighlight,
	MethodAccessibilityTargetProfilesList,
	MethodAccessibilityTargetProfilesSave,
	MethodAccessibilityTargetProfilesApply,
	MethodAccessibilityTargetProfilesDelete,
	MethodObservationTargetHighlight,
	MethodAppsInstalledList,
	MethodAppsFind,
}

func init() {
	AllMethods = append(AllMethods, extendedMethods...)
}

func (c *Client) DiagnosticsStatus(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodDiagnosticsStatus, nil)
}

func (c *Client) DiagnosticsList(ctx context.Context, params DiagnosticsListParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodDiagnosticsList, params)
}

func (c *Client) DiagnosticsLog(ctx context.Context, params DiagnosticsLogParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodDiagnosticsLog, params)
}

func (c *Client) DiagnosticsAcknowledge(ctx context.Context, params DiagnosticsIssueParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodDiagnosticsAcknowledge, params)
}

func (c *Client) DiagnosticsResolve(ctx context.Context, params DiagnosticsIssueParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodDiagnosticsResolve, params)
}

func (c *Client) DiagnosticsRepair(ctx context.Context, params DiagnosticsRepairParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodDiagnosticsRepair, params)
}

func (c *Client) DiagnosticsClear(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodDiagnosticsClear, nil)
}

func (c *Client) DiagnosticsSupportReport(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodDiagnosticsSupportReport, nil)
}

func (c *Client) DiagnosticsEventsSubscribe(ctx context.Context, params DiagnosticsEventSubscribeParams, onNotification func(method string, params json.RawMessage)) (*Subscription, error) {
	return c.StartSubscription(ctx, MethodDiagnosticsEventsSubscribe, params, onNotification)
}

func (c *Client) DiagnosticsEventsUnsubscribe(ctx context.Context, params EventUnsubscribeParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodDiagnosticsEventsUnsubscribe, params)
}

func (c *Client) STTRealtimeModelsList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimeModelsList, nil)
}

func (c *Client) STTRealtimeModelGet(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimeModelGet, nil)
}

func (c *Client) STTRealtimeModelSet(ctx context.Context, params STTModelSetParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimeModelSet, params)
}

func (c *Client) STTRealtimeModelDownload(ctx context.Context, params STTModelDownloadParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimeModelDownload, params)
}

func (c *Client) STTRealtimePrewarm(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimePrewarm, nil)
}

func (c *Client) STTRealtimeFileTranscribeStart(ctx context.Context, params any) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimeFileTranscribeStart, params)
}

func (c *Client) STTRealtimeFileTranscribeStatus(ctx context.Context, params STTJobParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimeFileTranscribeStatus, params)
}

func (c *Client) STTRealtimeFileTranscribeResult(ctx context.Context, params STTJobParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimeFileTranscribeResult, params)
}

func (c *Client) STTRealtimeFileTranscribeCancel(ctx context.Context, params STTJobParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimeFileTranscribeCancel, params)
}

func (c *Client) STTRealtimeFileTranscribeList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTRealtimeFileTranscribeList, nil)
}

func (c *Client) STTBatchModelsList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchModelsList, nil)
}

func (c *Client) STTBatchModelGet(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchModelGet, nil)
}

func (c *Client) STTBatchModelSet(ctx context.Context, params STTModelSetParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchModelSet, params)
}

func (c *Client) STTBatchModelDownload(ctx context.Context, params STTModelDownloadParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchModelDownload, params)
}

func (c *Client) STTBatchModelEnsure(ctx context.Context, params STTModelDownloadParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchModelEnsure, params)
}

func (c *Client) STTBatchTranscribeStart(ctx context.Context, params any) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchTranscribeStart, params)
}

func (c *Client) STTBatchTranscribeStatus(ctx context.Context, params STTJobParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchTranscribeStatus, params)
}

func (c *Client) STTBatchTranscribeResult(ctx context.Context, params STTJobParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchTranscribeResult, params)
}

func (c *Client) STTBatchTranscribeCancel(ctx context.Context, params STTJobParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchTranscribeCancel, params)
}

func (c *Client) STTBatchTranscribeList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTBatchTranscribeList, nil)
}

func (c *Client) TTSPlay(ctx context.Context, params TTSControlParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSPlay, params)
}

func (c *Client) TTSPause(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSPause, nil)
}

func (c *Client) TTSResume(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSResume, nil)
}

func (c *Client) TTSRestart(ctx context.Context, params TTSControlParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSRestart, params)
}

func (c *Client) AccessibilityContextCapture(ctx context.Context, params AccessibilityContextParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityContextCapture, params)
}

func (c *Client) AccessibilityContextScreen(ctx context.Context, params AccessibilityContextParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityContextScreen, params)
}

func (c *Client) AccessibilityFind(ctx context.Context, params AccessibilityFindParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityFind, params)
}

func (c *Client) AccessibilityActionPerform(ctx context.Context, params AccessibilityActionParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityActionPerform, params)
}

func (c *Client) AccessibilitySelectionSet(ctx context.Context, params AccessibilitySelectionSetParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilitySelectionSet, params)
}

func (c *Client) AccessibilityTextReplace(ctx context.Context, params AccessibilityTextReplaceParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTextReplace, params)
}

func (c *Client) AccessibilityTextInspect(ctx context.Context, params AccessibilityTextInspectParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTextInspect, params)
}

func (c *Client) AccessibilityDiagnostics(ctx context.Context, params AccessibilityDiagnosticsParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityDiagnostics, params)
}

func (c *Client) AccessibilityTargetOverlayOpen(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTargetOverlayOpen, nil)
}

func (c *Client) AccessibilityTargetHighlight(ctx context.Context, params AccessibilityTargetHighlightParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTargetHighlight, params)
}

func (c *Client) AccessibilityTargetProfilesList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTargetProfilesList, nil)
}

func (c *Client) AccessibilityTargetProfilesSave(ctx context.Context, params AccessibilityTargetProfileSaveParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTargetProfilesSave, params)
}

func (c *Client) AccessibilityTargetProfilesApply(ctx context.Context, params AccessibilityTargetProfileSelectParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTargetProfilesApply, params)
}

func (c *Client) AccessibilityTargetProfilesDelete(ctx context.Context, params AccessibilityTargetProfileSelectParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTargetProfilesDelete, params)
}

func (c *Client) ObservationTargetHighlight(ctx context.Context, params ObservationTargetHighlightParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodObservationTargetHighlight, params)
}

func (c *Client) AppsInstalledList(ctx context.Context, params InstalledApplicationsParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAppsInstalledList, params)
}

func (c *Client) AppsFind(ctx context.Context, params ApplicationFindParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAppsFind, params)
}
