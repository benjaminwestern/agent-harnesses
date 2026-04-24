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
	UseMetal          *bool    `json:"use_metal,omitempty"`
	Play              *bool    `json:"play,omitempty"`
	SaveWAV           *bool    `json:"save_wav,omitempty"`
	OutputDevice      string   `json:"output_device,omitempty"`
	Volume            *float64 `json:"volume,omitempty"`
	VoiceSpeed        *float64 `json:"voice_speed,omitempty"`
	SpeakerID         *int     `json:"speaker_id,omitempty"`
	VariantPreference string   `json:"variant_preference,omitempty"`
	DeEss             *bool    `json:"de_ess,omitempty"`
	CustomLexiconPath string   `json:"custom_lexicon_path,omitempty"`
	CustomVoiceID     string   `json:"custom_voice_id,omitempty"`
	CustomVoicePath   string   `json:"custom_voice_path,omitempty"`
	ChunkCharacters   *int     `json:"chunk_characters,omitempty"`
}

type TTSConfigSetParams struct {
	Voice             string   `json:"voice,omitempty"`
	VoiceSpeed        *float64 `json:"voice_speed,omitempty"`
	SpeakerID         *int     `json:"speaker_id,omitempty"`
	VariantPreference string   `json:"variant_preference,omitempty"`
	DeEss             *bool    `json:"de_ess,omitempty"`
	CustomLexiconPath string   `json:"custom_lexicon_path,omitempty"`
	CustomVoiceID     string   `json:"custom_voice_id,omitempty"`
	CustomVoicePath   string   `json:"custom_voice_path,omitempty"`
	ChunkCharacters   *int     `json:"chunk_characters,omitempty"`
	SaveWAVArtifacts  *bool    `json:"save_wav_artifacts,omitempty"`
	OutputDevice      string   `json:"output_device,omitempty"`
	Volume            *float64 `json:"volume,omitempty"`
}

type NotificationAudioPlayParams struct {
	SystemSound string   `json:"system_sound,omitempty"`
	Path        string   `json:"path,omitempty"`
	Event       string   `json:"event,omitempty"`
	Volume      *float64 `json:"volume,omitempty"`
	Interrupt   *bool    `json:"interrupt,omitempty"`
}

type NotificationAudioStopParams struct {
	All *bool `json:"all,omitempty"`
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
