package interaction

type ModelVariantParams struct {
	Variant string `json:"variant,omitempty"`
}

type ModelJobParams struct {
	ID string `json:"id,omitempty"`
}

type TextParams struct {
	Text string `json:"text"`
}

type OptionalTextParams struct {
	Text string `json:"text,omitempty"`
}

type STTModelSetParams struct {
	Model string `json:"model"`
}

type STTModelDownloadParams struct {
	Model  string `json:"model,omitempty"`
	Select *bool  `json:"select,omitempty"`
}

type VADConfigParams struct {
	Mode                string   `json:"mode,omitempty"`
	Threshold           *float64 `json:"threshold,omitempty"`
	PreRollSeconds      *float64 `json:"pre_roll_seconds,omitempty"`
	MinSpeechSeconds    *float64 `json:"min_speech_seconds,omitempty"`
	MinSilenceSeconds   *float64 `json:"min_silence_seconds,omitempty"`
	HangoverSeconds     *float64 `json:"hangover_seconds,omitempty"`
	MaxUtteranceSeconds *float64 `json:"max_utterance_seconds,omitempty"`
}

type STTStartParams struct {
	Provider                        string                           `json:"provider,omitempty"`
	Language                        string                           `json:"language,omitempty"`
	Model                           string                           `json:"model,omitempty"`
	RealtimeModel                   string                           `json:"realtime_model,omitempty"`
	RealtimePunctuationEnabled      *bool                            `json:"realtime_punctuation_enabled,omitempty"`
	InputDeviceID                   *int                             `json:"input_device_id,omitempty"`
	InputDeviceName                 string                           `json:"input_device_name,omitempty"`
	VAD                             *VADConfigParams                 `json:"vad,omitempty"`
	CompletionPolicy                string                           `json:"completion_policy,omitempty"`
	InsertDestination               string                           `json:"insert_destination,omitempty"`
	RealtimeSegmentInsertionEnabled *bool                            `json:"realtime_segment_insertion_enabled,omitempty"`
	RecordingMediaPolicy            string                           `json:"recording_media_policy,omitempty"`
	InsertionMode                   string                           `json:"insertion_mode,omitempty"`
	TargetRegion                    string                           `json:"target_region,omitempty"`
	FocusPolicy                     string                           `json:"focus_policy,omitempty"`
	TargetPath                      string                           `json:"target_path,omitempty"`
	TargetBundleID                  string                           `json:"target_bundle_id,omitempty"`
	TargetAppName                   string                           `json:"target_app_name,omitempty"`
	TargetWindowTitle               string                           `json:"target_window_title,omitempty"`
	TargetPID                       *int                             `json:"target_pid,omitempty"`
	UseSelectedTarget               *bool                            `json:"use_selected_target,omitempty"`
	Transform                       *TranscriptTransformConfigParams `json:"transform,omitempty"`
}

type EventSubscribeParams struct {
	Kinds []string `json:"kinds,omitempty"`
}

type EventUnsubscribeParams struct {
	SubscriptionID string `json:"subscription_id"`
}

type TTSSynthesizeParams struct {
	Text              string   `json:"text"`
	Provider          string   `json:"provider,omitempty"`
	ModelPath         string   `json:"model_path,omitempty"`
	SavePath          string   `json:"save_path,omitempty"`
	Voice             string   `json:"voice,omitempty"`
	KokoroVoice       string   `json:"kokoro_voice,omitempty"`
	AppleVoice        string   `json:"apple_voice,omitempty"`
	UseMetal          *bool    `json:"use_metal,omitempty"`
	Play              *bool    `json:"play,omitempty"`
	SaveWAV           *bool    `json:"save_wav,omitempty"`
	SaveWAVArtifacts  *bool    `json:"save_wav_artifacts,omitempty"`
	OutputDevice      string   `json:"output_device,omitempty"`
	Volume            *float64 `json:"volume,omitempty"`
	Language          string   `json:"language,omitempty"`
	VoiceSpeed        *float64 `json:"voice_speed,omitempty"`
	SpeakerID         *int     `json:"speaker_id,omitempty"`
	VariantPreference string   `json:"variant_preference,omitempty"`
	DeEss             *bool    `json:"de_ess,omitempty"`
	CustomLexiconPath string   `json:"custom_lexicon_path,omitempty"`
	CustomVoiceID     string   `json:"custom_voice_id,omitempty"`
	CustomVoicePath   string   `json:"custom_voice_path,omitempty"`
	ChunkCharacters   *int     `json:"chunk_characters,omitempty"`
	PlaybackMode      string   `json:"playback_mode,omitempty"`
}

type TTSConfigSetParams struct {
	Provider          string   `json:"provider,omitempty"`
	Voice             string   `json:"voice,omitempty"`
	KokoroVoice       string   `json:"kokoro_voice,omitempty"`
	AppleVoice        string   `json:"apple_voice,omitempty"`
	VoiceSpeed        *float64 `json:"voice_speed,omitempty"`
	SpeakerID         *int     `json:"speaker_id,omitempty"`
	VariantPreference string   `json:"variant_preference,omitempty"`
	DeEss             *bool    `json:"de_ess,omitempty"`
	CustomLexiconPath string   `json:"custom_lexicon_path,omitempty"`
	CustomVoiceID     string   `json:"custom_voice_id,omitempty"`
	CustomVoicePath   string   `json:"custom_voice_path,omitempty"`
	ChunkCharacters   *int     `json:"chunk_characters,omitempty"`
	PlaybackMode      string   `json:"playback_mode,omitempty"`
	SaveWAVArtifacts  *bool    `json:"save_wav_artifacts,omitempty"`
	OutputDevice      string   `json:"output_device,omitempty"`
	Volume            *float64 `json:"volume,omitempty"`
}

type TTSControlParams struct {
	Text string `json:"text,omitempty"`
}

type NotificationAudioPlayParams struct {
	SystemSound string   `json:"system_sound,omitempty"`
	Path        string   `json:"path,omitempty"`
	Event       string   `json:"event,omitempty"`
	Volume      *float64 `json:"volume,omitempty"`
	Interrupt   *bool    `json:"interrupt,omitempty"`
	Route       string   `json:"route,omitempty"`
}

type NotificationAudioStopParams struct {
	ID  string `json:"id,omitempty"`
	All *bool  `json:"all,omitempty"`
}

type TreeParams struct {
	MaxDepth    *int `json:"max_depth,omitempty"`
	MaxChildren *int `json:"max_children,omitempty"`
}

type ApplicationInventoryParams struct {
	MaxDepth              *int  `json:"max_depth,omitempty"`
	MaxChildren           *int  `json:"max_children,omitempty"`
	IncludeBackgroundApps *bool `json:"include_background_apps,omitempty"`
}

type InsertParams struct {
	Text              string `json:"text"`
	TargetPath        string `json:"target_path,omitempty"`
	TargetBundleID    string `json:"target_bundle_id,omitempty"`
	TargetAppName     string `json:"target_app_name,omitempty"`
	TargetWindowTitle string `json:"target_window_title,omitempty"`
	TargetPID         *int   `json:"target_pid,omitempty"`
	Mode              string `json:"mode,omitempty"`
	TargetRegion      string `json:"target_region,omitempty"`
	FocusPolicy       string `json:"focus_policy,omitempty"`
}

type CoordinateInsertParams struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type ScreenClickParams struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type ObservationScreenshotParams struct {
	Target   map[string]any `json:"target,omitempty"`
	SavePath string         `json:"save_path,omitempty"`
	Format   string         `json:"format,omitempty"`
}

type ObservationRecordingStartParams struct {
	Target           map[string]any `json:"target,omitempty"`
	SavePath         string         `json:"save_path,omitempty"`
	CountdownSeconds *float64       `json:"countdown_seconds,omitempty"`
	RecordForSeconds *float64       `json:"record_for_seconds,omitempty"`
	FPS              *float64       `json:"fps,omitempty"`
}

type ObservationRecordingIDParams struct {
	RecordingID string `json:"recording_id,omitempty"`
}

type ApplicationOpenParams struct {
	Path     string `json:"path,omitempty"`
	BundleID string `json:"bundle_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Activate *bool  `json:"activate,omitempty"`
}

type ApplicationActivateParams struct {
	PID      *int   `json:"pid,omitempty"`
	BundleID string `json:"bundle_id,omitempty"`
}
