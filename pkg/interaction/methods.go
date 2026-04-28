package interaction

import (
	"context"
	"encoding/json"
)

const (
	MethodSystemDescribe = "system.describe"
	MethodSystemStatus   = "system.status"
	MethodSystemHealth   = "system.health"

	MethodDevicesList = "devices.list"

	MethodPermissionsStatus                    = "permissions.status"
	MethodPermissionsRequestMicrophone         = "permissions.request_microphone"
	MethodPermissionsRequestAccessibility      = "permissions.request_accessibility"
	MethodPermissionsOpenAccessibilitySettings = "permissions.open_accessibility_settings"
	MethodPermissionsOpenMicrophoneSettings    = "permissions.open_microphone_settings"
	MethodPermissionsOpenInputMonitoring       = "permissions.open_input_monitoring_settings"
	MethodPermissionsOpenScreenRecording       = "permissions.open_screen_recording_settings"

	MethodModelsCatalog             = "models.catalog"
	MethodModelsParakeetEOUStatus   = "models.parakeet_eou.status"
	MethodModelsParakeetEOUEnsure   = "models.parakeet_eou.ensure"
	MethodModelsParakeetEOUDownload = "models.parakeet_eou.download"
	MethodModelsParakeetEOUDelete   = "models.parakeet_eou.delete"
	MethodModelsDownloadJob         = "models.download_job"
	MethodModelsDownloadCancel      = "models.download_cancel"

	MethodTranscriptGet       = "transcript.get"
	MethodTranscriptSet       = "transcript.set"
	MethodTranscriptCopy      = "transcript.copy"
	MethodTranscriptPasteLast = "transcript.paste_last"
	MethodTranscriptInsert    = "transcript.insert"
	MethodTranscriptProcess   = "transcript.process"
	MethodTransformsProcess   = "transforms.process"
	MethodTransformsApply     = "transforms.apply"

	MethodSTTStart             = "stt.start"
	MethodSTTStop              = "stt.stop"
	MethodSTTReset             = "stt.reset"
	MethodSTTStatus            = "stt.status"
	MethodSTTEventsSubscribe   = "stt.events.subscribe"
	MethodSTTEventsUnsubscribe = "stt.events.unsubscribe"
	MethodSTTModelsList        = "stt.models.list"
	MethodSTTModelGet          = "stt.model.get"
	MethodSTTModelSet          = "stt.model.set"
	MethodSTTModelDownload     = "stt.model.download"

	MethodTTSSpeak           = "tts.speak"
	MethodTTSSave            = "tts.save"
	MethodTTSStop            = "tts.stop"
	MethodTTSStatus          = "tts.status"
	MethodTTSVoicesList      = "tts.voices.list"
	MethodTTSVoicesRefresh   = "tts.voices.refresh"
	MethodTTSConfigGet       = "tts.config.get"
	MethodTTSConfigSet       = "tts.config.set"
	MethodTTSSpeakSelected   = "tts.speak_selected"
	MethodTTSSpeakTranscript = "tts.speak_transcript"

	MethodNotificationAudioCatalog = "notification.audio.catalog"
	MethodNotificationAudioPlay    = "notification.audio.play"
	MethodNotificationAudioStop    = "notification.audio.stop"
	MethodNotificationAudioStatus  = "notification.audio.status"

	MethodAccessibilityStatus            = "accessibility.status"
	MethodAccessibilityRequestPermission = "accessibility.request_permission"
	MethodAccessibilityOpenSettings      = "accessibility.open_settings"
	MethodAccessibilityTreeFocused       = "accessibility.tree.focused"
	MethodAccessibilityTargetsList       = "accessibility.targets.list"
	MethodAccessibilityAppsList          = "accessibility.apps.list"
	MethodAccessibilityInsert            = "accessibility.insert"
	MethodAccessibilityClickInsert       = "accessibility.click_insert"

	MethodScreenClick = "screen.click"

	MethodObservationPermissionStatus  = "observation.permission_status"
	MethodObservationRequestPermission = "observation.request_permission"
	MethodObservationScreenshot        = "observation.screenshot"
	MethodObservationRecordingStart    = "observation.recording.start"
	MethodObservationRecordingStop     = "observation.recording.stop"
	MethodObservationRecordingStatus   = "observation.recording.status"
	MethodObservationRecordingsList    = "observation.recordings.list"
	MethodObservationEventsSubscribe   = "observation.events.subscribe"
	MethodObservationEventsUnsubscribe = "observation.events.unsubscribe"

	MethodAppsOpen     = "apps.open"
	MethodAppsActivate = "apps.activate"
)

var AllMethods = []string{
	MethodSystemDescribe,
	MethodSystemStatus,
	MethodSystemHealth,
	MethodDevicesList,
	MethodPermissionsStatus,
	MethodPermissionsRequestMicrophone,
	MethodPermissionsRequestAccessibility,
	MethodPermissionsOpenAccessibilitySettings,
	MethodPermissionsOpenMicrophoneSettings,
	MethodPermissionsOpenInputMonitoring,
	MethodPermissionsOpenScreenRecording,
	MethodModelsCatalog,
	MethodModelsParakeetEOUStatus,
	MethodModelsParakeetEOUEnsure,
	MethodModelsParakeetEOUDownload,
	MethodModelsParakeetEOUDelete,
	MethodModelsDownloadJob,
	MethodModelsDownloadCancel,
	MethodTranscriptGet,
	MethodTranscriptSet,
	MethodTranscriptCopy,
	MethodTranscriptPasteLast,
	MethodTranscriptInsert,
	MethodTranscriptProcess,
	MethodTransformsProcess,
	MethodTransformsApply,
	MethodSTTStart,
	MethodSTTStop,
	MethodSTTReset,
	MethodSTTStatus,
	MethodSTTEventsSubscribe,
	MethodSTTEventsUnsubscribe,
	MethodSTTModelsList,
	MethodSTTModelGet,
	MethodSTTModelSet,
	MethodSTTModelDownload,
	MethodTTSSpeak,
	MethodTTSSave,
	MethodTTSStop,
	MethodTTSStatus,
	MethodTTSVoicesList,
	MethodTTSVoicesRefresh,
	MethodTTSConfigGet,
	MethodTTSConfigSet,
	MethodTTSSpeakSelected,
	MethodTTSSpeakTranscript,
	MethodNotificationAudioCatalog,
	MethodNotificationAudioPlay,
	MethodNotificationAudioStop,
	MethodNotificationAudioStatus,
	MethodAccessibilityStatus,
	MethodAccessibilityRequestPermission,
	MethodAccessibilityOpenSettings,
	MethodAccessibilityTreeFocused,
	MethodAccessibilityTargetsList,
	MethodAccessibilityAppsList,
	MethodAccessibilityInsert,
	MethodAccessibilityClickInsert,
	MethodScreenClick,
	MethodObservationPermissionStatus,
	MethodObservationRequestPermission,
	MethodObservationScreenshot,
	MethodObservationRecordingStart,
	MethodObservationRecordingStop,
	MethodObservationRecordingStatus,
	MethodObservationRecordingsList,
	MethodObservationEventsSubscribe,
	MethodObservationEventsUnsubscribe,
	MethodAppsOpen,
	MethodAppsActivate,
}

func (c *Client) SystemDescribe(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSystemDescribe, nil)
}

func (c *Client) SystemStatus(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSystemStatus, nil)
}

func (c *Client) SystemHealth(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSystemHealth, nil)
}

func (c *Client) DevicesList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodDevicesList, nil)
}

func (c *Client) PermissionsStatus(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodPermissionsStatus, nil)
}

func (c *Client) PermissionsRequestMicrophone(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodPermissionsRequestMicrophone, nil)
}

func (c *Client) PermissionsRequestAccessibility(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodPermissionsRequestAccessibility, nil)
}

func (c *Client) PermissionsOpenAccessibilitySettings(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodPermissionsOpenAccessibilitySettings, nil)
}

func (c *Client) PermissionsOpenMicrophoneSettings(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodPermissionsOpenMicrophoneSettings, nil)
}

func (c *Client) PermissionsOpenInputMonitoringSettings(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodPermissionsOpenInputMonitoring, nil)
}

func (c *Client) PermissionsOpenScreenRecordingSettings(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodPermissionsOpenScreenRecording, nil)
}

func (c *Client) ModelsCatalog(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodModelsCatalog, nil)
}

func (c *Client) ModelsParakeetEOUStatus(ctx context.Context, params ModelVariantParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodModelsParakeetEOUStatus, params)
}

func (c *Client) ModelsParakeetEOUEnsure(ctx context.Context, params ModelVariantParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodModelsParakeetEOUEnsure, params)
}

func (c *Client) ModelsParakeetEOUDownload(ctx context.Context, params ModelVariantParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodModelsParakeetEOUDownload, params)
}

func (c *Client) ModelsParakeetEOUDelete(ctx context.Context, params ModelVariantParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodModelsParakeetEOUDelete, params)
}

func (c *Client) ModelsDownloadJob(ctx context.Context, params ModelJobParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodModelsDownloadJob, params)
}

func (c *Client) ModelsDownloadCancel(ctx context.Context, params ModelJobParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodModelsDownloadCancel, params)
}

func (c *Client) TranscriptGet(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTranscriptGet, nil)
}

func (c *Client) TranscriptSet(ctx context.Context, params TextParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodTranscriptSet, params)
}

func (c *Client) TranscriptCopy(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTranscriptCopy, nil)
}

func (c *Client) TranscriptPasteLast(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTranscriptPasteLast, nil)
}

func (c *Client) TranscriptInsert(ctx context.Context, params OptionalTextParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodTranscriptInsert, params)
}

func (c *Client) TranscriptProcess(ctx context.Context, params any) (json.RawMessage, error) {
	return c.Call(ctx, MethodTranscriptProcess, params)
}

func (c *Client) TransformsProcess(ctx context.Context, params TransformProcessParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodTransformsProcess, params)
}

func (c *Client) TransformsApply(ctx context.Context, params TransformApplyParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodTransformsApply, params)
}

func (c *Client) STTStart(ctx context.Context, params ...STTStartParams) (json.RawMessage, error) {
	var payload any
	if len(params) > 0 {
		payload = params[0]
	}
	return c.Call(ctx, MethodSTTStart, payload)
}

func (c *Client) STTStop(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTStop, nil)
}

func (c *Client) STTReset(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTReset, nil)
}

func (c *Client) STTStatus(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTStatus, nil)
}

func (c *Client) STTEventsSubscribe(ctx context.Context, params EventSubscribeParams, onNotification func(method string, params json.RawMessage)) (*Subscription, error) {
	return c.StartSubscription(ctx, MethodSTTEventsSubscribe, params, onNotification)
}

func (c *Client) STTEventsUnsubscribe(ctx context.Context, params EventUnsubscribeParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTEventsUnsubscribe, params)
}

func (c *Client) STTModelsList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTModelsList, nil)
}

func (c *Client) STTModelGet(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTModelGet, nil)
}

func (c *Client) STTModelSet(ctx context.Context, params STTModelSetParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTModelSet, params)
}

func (c *Client) STTModelDownload(ctx context.Context, params STTModelDownloadParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodSTTModelDownload, params)
}

func (c *Client) TTSSpeak(ctx context.Context, params TTSSynthesizeParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSSpeak, params)
}

func (c *Client) TTSSave(ctx context.Context, params TTSSynthesizeParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSSave, params)
}

func (c *Client) TTSStop(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSStop, nil)
}

func (c *Client) TTSStatus(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSStatus, nil)
}

func (c *Client) TTSVoicesList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSVoicesList, nil)
}

func (c *Client) TTSVoicesRefresh(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSVoicesRefresh, nil)
}

func (c *Client) TTSConfigGet(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSConfigGet, nil)
}

func (c *Client) TTSConfigSet(ctx context.Context, params TTSConfigSetParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSConfigSet, params)
}

func (c *Client) TTSSpeakSelected(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSSpeakSelected, nil)
}

func (c *Client) TTSSpeakTranscript(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodTTSSpeakTranscript, nil)
}

func (c *Client) NotificationAudioCatalog(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodNotificationAudioCatalog, nil)
}

func (c *Client) NotificationAudioPlay(ctx context.Context, params NotificationAudioPlayParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodNotificationAudioPlay, params)
}

func (c *Client) NotificationAudioStop(ctx context.Context, params NotificationAudioStopParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodNotificationAudioStop, params)
}

func (c *Client) NotificationAudioStatus(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodNotificationAudioStatus, nil)
}

func (c *Client) AccessibilityStatus(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityStatus, nil)
}

func (c *Client) AccessibilityRequestPermission(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityRequestPermission, nil)
}

func (c *Client) AccessibilityOpenSettings(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityOpenSettings, nil)
}

func (c *Client) AccessibilityTreeFocused(ctx context.Context, params TreeParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTreeFocused, params)
}

func (c *Client) AccessibilityTargetsList(ctx context.Context, params TreeParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityTargetsList, params)
}

func (c *Client) AccessibilityAppsList(ctx context.Context, params ApplicationInventoryParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityAppsList, params)
}

func (c *Client) AccessibilityInsert(ctx context.Context, params InsertParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityInsert, params)
}

func (c *Client) AccessibilityClickInsert(ctx context.Context, params CoordinateInsertParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAccessibilityClickInsert, params)
}

func (c *Client) ScreenClick(ctx context.Context, params ScreenClickParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodScreenClick, params)
}

func (c *Client) ObservationPermissionStatus(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodObservationPermissionStatus, nil)
}

func (c *Client) ObservationRequestPermission(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodObservationRequestPermission, nil)
}

func (c *Client) ObservationScreenshot(ctx context.Context, params ObservationScreenshotParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodObservationScreenshot, params)
}

func (c *Client) ObservationRecordingStart(ctx context.Context, params ObservationRecordingStartParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodObservationRecordingStart, params)
}

func (c *Client) ObservationRecordingStop(ctx context.Context, params ObservationRecordingIDParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodObservationRecordingStop, params)
}

func (c *Client) ObservationRecordingStatus(ctx context.Context, params ObservationRecordingIDParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodObservationRecordingStatus, params)
}

func (c *Client) ObservationRecordingsList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, MethodObservationRecordingsList, nil)
}

func (c *Client) ObservationEventsSubscribe(ctx context.Context, params EventSubscribeParams, onNotification func(method string, params json.RawMessage)) (*Subscription, error) {
	return c.StartSubscription(ctx, MethodObservationEventsSubscribe, params, onNotification)
}

func (c *Client) ObservationEventsUnsubscribe(ctx context.Context, params EventUnsubscribeParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodObservationEventsUnsubscribe, params)
}

func (c *Client) AppsOpen(ctx context.Context, params ApplicationOpenParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAppsOpen, params)
}

func (c *Client) AppsActivate(ctx context.Context, params ApplicationActivateParams) (json.RawMessage, error) {
	return c.Call(ctx, MethodAppsActivate, params)
}
