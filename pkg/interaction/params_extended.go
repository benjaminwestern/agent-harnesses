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
