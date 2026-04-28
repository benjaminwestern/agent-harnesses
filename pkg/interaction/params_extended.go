package interaction

type DiagnosticsEventSubscribeParams struct {
	Severities []string `json:"severities,omitempty"`
	Domains    []string `json:"domains,omitempty"`
}

type DiagnosticsListParams struct {
	Limit       *int   `json:"limit,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Operation   string `json:"operation,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}

type DiagnosticsLogParams struct {
	Limit       *int   `json:"limit,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Operation   string `json:"operation,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
	Since       string `json:"since,omitempty"`
	MaxBytes    *int   `json:"max_bytes,omitempty"`
}

type DiagnosticsIssueParams struct {
	IssueID string `json:"issue_id"`
}

type DiagnosticsRepairParams struct {
	IssueID    string            `json:"issue_id,omitempty"`
	ActionID   string            `json:"action_id,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Target     string            `json:"target,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

type TranscriptReplacementRequest struct {
	Source      string `json:"source"`
	Replacement string `json:"replacement"`
}

type TranscriptTransformConfigParams struct {
	Enabled      *bool                          `json:"enabled,omitempty"`
	RemovalTerms []string                       `json:"removal_terms,omitempty"`
	Replacements []TranscriptReplacementRequest `json:"replacements,omitempty"`
}

type TransformProcessParams struct {
	Text         string                         `json:"text"`
	Enabled      *bool                          `json:"enabled,omitempty"`
	RemovalTerms []string                       `json:"removal_terms,omitempty"`
	Replacements []TranscriptReplacementRequest `json:"replacements,omitempty"`
}

type TransformApplyParams struct {
	Text         string                         `json:"text,omitempty"`
	Enabled      *bool                          `json:"enabled,omitempty"`
	RemovalTerms []string                       `json:"removal_terms,omitempty"`
	Replacements []TranscriptReplacementRequest `json:"replacements,omitempty"`
}

type AccessibilityContextParams struct {
	MaxDepth       *int   `json:"max_depth,omitempty"`
	MaxChildren    *int   `json:"max_children,omitempty"`
	MaxElements    *int   `json:"max_elements,omitempty"`
	TargetBundleID string `json:"target_bundle_id,omitempty"`
	TargetPID      *int   `json:"target_pid,omitempty"`
	IncludePrompt  *bool  `json:"include_prompt,omitempty"`
}

type AccessibilityFindParams struct {
	Query          string `json:"query"`
	Role           string `json:"role,omitempty"`
	Category       string `json:"category,omitempty"`
	MaxResults     *int   `json:"max_results,omitempty"`
	MaxDepth       *int   `json:"max_depth,omitempty"`
	MaxChildren    *int   `json:"max_children,omitempty"`
	MaxElements    *int   `json:"max_elements,omitempty"`
	TargetBundleID string `json:"target_bundle_id,omitempty"`
	TargetPID      *int   `json:"target_pid,omitempty"`
}

type AccessibilityActionParams struct {
	TargetPath         string `json:"target_path"`
	TargetBundleID     string `json:"target_bundle_id,omitempty"`
	TargetPID          *int   `json:"target_pid,omitempty"`
	Action             string `json:"action,omitempty"`
	AllowFallbackClick *bool  `json:"allow_fallback_click,omitempty"`
	FocusPolicy        string `json:"focus_policy,omitempty"`
}

type AccessibilitySelectionSetParams struct {
	TargetPath            string         `json:"target_path"`
	TargetBundleID        string         `json:"target_bundle_id,omitempty"`
	TargetPID             *int           `json:"target_pid,omitempty"`
	Range                 map[string]any `json:"range,omitempty"`
	MatchText             string         `json:"match_text,omitempty"`
	Occurrence            string         `json:"occurrence,omitempty"`
	TargetRegion          string         `json:"target_region,omitempty"`
	Selected              *bool          `json:"selected,omitempty"`
	ClearSiblingSelection *bool          `json:"clear_sibling_selection,omitempty"`
	FocusPolicy           string         `json:"focus_policy,omitempty"`
}

type AccessibilityTextReplaceParams struct {
	TargetPath      string `json:"target_path"`
	TargetBundleID  string `json:"target_bundle_id,omitempty"`
	TargetPID       *int   `json:"target_pid,omitempty"`
	FindText        string `json:"find_text"`
	ReplacementText string `json:"replacement_text"`
	Occurrence      string `json:"occurrence,omitempty"`
	TargetRegion    string `json:"target_region,omitempty"`
	PreserveStyle   *bool  `json:"preserve_style,omitempty"`
	FocusPolicy     string `json:"focus_policy,omitempty"`
}

type AccessibilityTextInspectParams struct {
	TargetPath     string         `json:"target_path"`
	TargetBundleID string         `json:"target_bundle_id,omitempty"`
	TargetPID      *int           `json:"target_pid,omitempty"`
	Range          map[string]any `json:"range,omitempty"`
	MatchText      string         `json:"match_text,omitempty"`
	Occurrence     string         `json:"occurrence,omitempty"`
	TargetRegion   string         `json:"target_region,omitempty"`
	FocusPolicy    string         `json:"focus_policy,omitempty"`
}

type AccessibilityDiagnosticsParams struct {
	Text              string `json:"text,omitempty"`
	TargetPath        string `json:"target_path,omitempty"`
	TargetBundleID    string `json:"target_bundle_id,omitempty"`
	TargetAppName     string `json:"target_app_name,omitempty"`
	TargetWindowTitle string `json:"target_window_title,omitempty"`
	TargetPID         *int   `json:"target_pid,omitempty"`
	Mode              string `json:"mode,omitempty"`
	MaxDepth          *int   `json:"max_depth,omitempty"`
	MaxChildren       *int   `json:"max_children,omitempty"`
}

type AccessibilityTargetHighlightParams struct {
	TargetPath      string   `json:"target_path,omitempty"`
	TargetBundleID  string   `json:"target_bundle_id,omitempty"`
	TargetPID       *int     `json:"target_pid,omitempty"`
	X               *float64 `json:"x,omitempty"`
	Y               *float64 `json:"y,omitempty"`
	Width           *float64 `json:"width,omitempty"`
	Height          *float64 `json:"height,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
}

type AccessibilityTargetProfileSaveParams struct {
	ID           string `json:"id,omitempty"`
	BundleID     string `json:"bundle_id,omitempty"`
	AppName      string `json:"app_name,omitempty"`
	WindowTitle  string `json:"window_title,omitempty"`
	TargetPath   string `json:"target_path,omitempty"`
	TargetLabel  string `json:"target_label,omitempty"`
	TargetPID    *int   `json:"target_pid,omitempty"`
	Mode         string `json:"mode,omitempty"`
	TargetRegion string `json:"target_region,omitempty"`
	FocusPolicy  string `json:"focus_policy,omitempty"`
}

type AccessibilityTargetProfileSelectParams struct {
	ID       string `json:"id,omitempty"`
	BundleID string `json:"bundle_id,omitempty"`
	AppName  string `json:"app_name,omitempty"`
}

type ObservationTargetHighlightParams struct {
	Target          map[string]any `json:"target,omitempty"`
	DurationSeconds *float64       `json:"duration_seconds,omitempty"`
}

type STTJobParams struct {
	JobID string `json:"job_id"`
}

type InstalledApplicationsParams struct {
	Query             string `json:"query,omitempty"`
	MaxResults        *int   `json:"max_results,omitempty"`
	IncludeSystemApps *bool  `json:"include_system_apps,omitempty"`
	IncludeUserApps   *bool  `json:"include_user_apps,omitempty"`
}

type ApplicationFindParams struct {
	Query             string `json:"query,omitempty"`
	BundleID          string `json:"bundle_id,omitempty"`
	Name              string `json:"name,omitempty"`
	Path              string `json:"path,omitempty"`
	MaxResults        *int   `json:"max_results,omitempty"`
	IncludeSystemApps *bool  `json:"include_system_apps,omitempty"`
	IncludeUserApps   *bool  `json:"include_user_apps,omitempty"`
}
