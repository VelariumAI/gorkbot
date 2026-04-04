package harness

import "time"

// FeatureStatus represents the current state of a feature.
type FeatureStatus string

const (
	StatusFailing    FeatureStatus = "failing"
	StatusIncomplete FeatureStatus = "incomplete"
	StatusInProgress FeatureStatus = "in_progress"
	StatusPassing    FeatureStatus = "passing"
	StatusSkipped    FeatureStatus = "skipped"
)

// Feature represents a single trackable unit of work.
type Feature struct {
	ID               string           `json:"id"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	Status           FeatureStatus    `json:"status"`
	Dependencies     []string         `json:"dependencies,omitempty"`
	Priority         int              `json:"priority"`
	Tags             []string         `json:"tags,omitempty"`
	VerificationSpec *VerificationSpec `json:"verification_spec,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	CommitHash       string           `json:"commit_hash,omitempty"`
	ErrorLog         string           `json:"error_log,omitempty"`
}

// FeatureList is the top-level project feature manifest.
type FeatureList struct {
	ProjectName string    `json:"project_name"`
	Goal        string    `json:"goal"`
	Features    []Feature `json:"features"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// VerificationSpec defines how to verify a feature is complete.
type VerificationSpec struct {
	Commands     []VerifyCommand `json:"commands,omitempty"`
	RequireBuild bool            `json:"require_build"`
	RequireTests bool            `json:"require_tests"`
	TestPattern  string          `json:"test_pattern,omitempty"`
}

// VerifyCommand is a single verification command.
type VerifyCommand struct {
	Name    string        `json:"name"`
	Command string        `json:"command"`
	Timeout time.Duration `json:"timeout,omitempty"`
}

// ProgressEntry records a single action in the progress log.
type ProgressEntry struct {
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id"`
	Action    string    `json:"action"`
	FeatureID string    `json:"feature_id,omitempty"`
	Details   string    `json:"details,omitempty"`
}

// HarnessState tracks cross-session harness state.
type HarnessState struct {
	ActiveFeatureID string    `json:"active_feature_id,omitempty"`
	SessionID       string    `json:"session_id"`
	ProjectRoot     string    `json:"project_root"`
	Initialized     bool      `json:"initialized"`
	LastBootAt      time.Time `json:"last_boot_at"`
	TotalSessions   int       `json:"total_sessions"`
}

// VerificationReport is the result of verifying a feature.
type VerificationReport struct {
	FeatureID string             `json:"feature_id"`
	Passed    bool               `json:"passed"`
	Steps     []VerificationStep `json:"steps"`
	Summary   string             `json:"summary"`
}

// VerificationStep is a single step within a verification report.
type VerificationStep struct {
	Name     string        `json:"name"`
	Command  string        `json:"command"`
	Passed   bool          `json:"passed"`
	Output   string        `json:"output"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
}

// BootReport is returned when a worker boots into a project.
type BootReport struct {
	ProjectRoot     string          `json:"project_root"`
	TotalFeatures   int             `json:"total_features"`
	PassingCount    int             `json:"passing_count"`
	FailingCount    int             `json:"failing_count"`
	InProgressCount int             `json:"in_progress_count"`
	RecentCommits   string          `json:"recent_commits"`
	RecentProgress  []ProgressEntry `json:"recent_progress"`
	ActiveFeature   *Feature        `json:"active_feature,omitempty"`
	NextSuggested   *Feature        `json:"next_suggested,omitempty"`
}

// FeatureInput is used during project initialization to define features.
type FeatureInput struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Dependencies []string `json:"dependencies,omitempty"`
	Priority     int      `json:"priority"`
}
