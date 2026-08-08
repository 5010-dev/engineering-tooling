// Package checker implements offline Golden Path structural conformance.
package checker

import "time"

const (
	Version                 = "1.5.0"
	StandardVersion         = "2026.08.4"
	ContractVersion         = "golden-path/v1"
	CatalogDigest           = "sha256:ebe93eec9b095e8d6763207ffcac12b9836eb6bae366084a9f98d2cdbb4fddd9"
	SnapshotAggregateDigest = "sha256:55cc033ca9df35837c6b5f2b7424128d6899cab02978ae90db8595538ee0f881"
	SnapshotSourceCommit    = "416dec3053befbc5f54371bbb51950dd5cdee305"
	SnapshotSourceTree      = "61b248457e0f2210da3acc4dd8138350bcee270e"
)

type Options struct {
	Root        string
	EvaluatedAt time.Time
	Enforcement string
}

type Result struct {
	SchemaVersion   string         `json:"schemaVersion"`
	ContractVersion string         `json:"contractVersion"`
	StandardVersion string         `json:"standardVersion"`
	CheckerVersion  string         `json:"checkerVersion"`
	CatalogDigest   string         `json:"catalogDigest"`
	EvaluatedAt     string         `json:"evaluatedAt"`
	Enforcement     string         `json:"enforcement"`
	Profiles        []string       `json:"profiles"`
	ExitCode        int            `json:"exitCode"`
	Complete        bool           `json:"complete"`
	Summary         Summary        `json:"summary"`
	Findings        []Finding      `json:"findings"`
	Extensions      map[string]any `json:"extensions,omitempty"`
}

type Summary struct {
	Pass   int `json:"pass"`
	Fail   int `json:"fail"`
	Warn   int `json:"warn"`
	Skip   int `json:"skip"`
	Waived int `json:"waived"`
	Error  int `json:"error"`
}

type Finding struct {
	RuleID      string         `json:"ruleId"`
	Status      string         `json:"status"`
	Severity    string         `json:"severity"`
	Assessment  string         `json:"assessment"`
	Path        string         `json:"path"`
	Secondary   string         `json:"secondaryKey,omitempty"`
	Message     string         `json:"message"`
	Remediation string         `json:"remediation"`
	ExceptionID *string        `json:"exceptionId"`
	Extensions  map[string]any `json:"extensions,omitempty"`
}

type Metadata struct {
	SchemaVersion      string               `json:"schemaVersion"`
	ContractVersion    string               `json:"contractVersion"`
	StandardVersion    string               `json:"standardVersion"`
	AssetBundleVersion string               `json:"assetBundleVersion"`
	Applicability      Applicability        `json:"applicability,omitempty"`
	Profiles           []string             `json:"profiles"`
	ArtifactTypes      []string             `json:"artifactTypes"`
	Capabilities       []string             `json:"capabilities"`
	Targets            []Target             `json:"targets,omitempty"`
	Components         []MetadataComponent  `json:"-"`
	NativeRoots        []MetadataNativeRoot `json:"-"`
	Extensions         map[string]any       `json:"extensions,omitempty"`
	ComponentPath      string               `json:"-"`
	NativeRootID       string               `json:"-"`
}

type MetadataComponent struct {
	Path          string   `json:"path"`
	Profiles      []string `json:"profiles"`
	ArtifactTypes []string `json:"artifactTypes"`
	Capabilities  []string `json:"capabilities"`
}

type NativeRootsFile struct {
	SchemaVersion string               `json:"schemaVersion"`
	Roots         []MetadataNativeRoot `json:"roots"`
}

type MetadataNativeRoot struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Profiles []string `json:"profiles"`
}

type Applicability struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type Target struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Runtime      string `json:"runtime,omitempty"`
	TargetTriple string `json:"targetTriple,omitempty"`
	Tier         string `json:"tier"`
	Execution    *bool  `json:"execution,omitempty"`
}

type ExceptionsFile struct {
	SchemaVersion string      `json:"schemaVersion"`
	Exceptions    []Exception `json:"exceptions"`
}

type Exception struct {
	ID                   string            `json:"id"`
	Rules                []string          `json:"rules"`
	Scope                ExceptionScope    `json:"scope"`
	Reason               string            `json:"reason"`
	Owner                string            `json:"owner"`
	RiskClass            string            `json:"riskClass"`
	Approval             ExceptionApproval `json:"approval"`
	ExpiresAt            string            `json:"expiresAt"`
	TrackingIssue        string            `json:"trackingIssue,omitempty"`
	Risk                 string            `json:"risk,omitempty"`
	CompensatingControls []string          `json:"compensatingControls,omitempty"`
	RenewedFrom          string            `json:"renewedFrom,omitempty"`
}

type ExceptionScope struct {
	Profiles      []string `json:"profiles,omitempty"`
	ArtifactTypes []string `json:"artifactTypes,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Paths         []string `json:"paths,omitempty"`
}

type ExceptionApproval struct {
	Authorities []ExceptionAuthority `json:"authorities"`
}

type ExceptionAuthority struct {
	Role       string `json:"role"`
	Reference  string `json:"reference"`
	ApprovedAt string `json:"approvedAt"`
}

type Catalog struct {
	SchemaVersion   string `json:"schemaVersion"`
	ContractVersion string `json:"contractVersion"`
	StandardVersion string `json:"standardVersion"`
	Rules           []Rule `json:"rules"`
}

type Rule struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Level         string            `json:"level"`
	Source        RuleSource        `json:"source"`
	Applicability RuleApplicability `json:"applicability"`
	Assessment    string            `json:"assessment"`
	Severity      string            `json:"severity"`
	Waivable      bool              `json:"waivable"`
	HighRisk      bool              `json:"highRisk"`
	Assertion     string            `json:"assertion"`
	Remediation   string            `json:"remediation"`
	IntroducedIn  string            `json:"introducedIn"`
	RetiredIn     *string           `json:"retiredIn"`
	Replacement   *string           `json:"replacement"`
}

type RuleSource struct {
	Decision string `json:"decision"`
	Document string `json:"document"`
	Anchor   string `json:"anchor"`
}

type RuleApplicability struct {
	Profiles      []string `json:"profiles"`
	ArtifactTypes []string `json:"artifactTypes"`
	Capabilities  []string `json:"capabilities"`
	Condition     string   `json:"condition"`
}

type CompatibilityManifest struct {
	SchemaVersion              string               `json:"schemaVersion"`
	CheckerVersion             string               `json:"checkerVersion"`
	Lifecycle                  string               `json:"lifecycle"`
	Enforcement                []string             `json:"enforcement"`
	ExceptionExpiryWarningDays int                  `json:"exceptionExpiryWarningDays"`
	Standards                  []CompatibleStandard `json:"standards"`
	RuntimeSelections          []RuntimeSelection   `json:"runtimeSelections"`
	SupportedTargets           []SupportedTarget    `json:"supportedTargets"`
}

type CompatibleStandard struct {
	StandardVersion         string   `json:"standardVersion"`
	ContractVersion         string   `json:"contractVersion"`
	SchemaVersions          []string `json:"schemaVersions"`
	Snapshot                string   `json:"snapshot"`
	SourceCommit            string   `json:"sourceCommit"`
	CatalogDigest           string   `json:"catalogDigest"`
	SnapshotAggregateDigest string   `json:"snapshotAggregateDigest"`
}

type RuntimeSelection struct {
	Profile  string                    `json:"profile"`
	Tool     string                    `json:"tool"`
	Versions []RuntimeSelectionVersion `json:"versions"`
}

type RuntimeSelectionVersion struct {
	Version       string `json:"version"`
	Disposition   string `json:"disposition"`
	SupportEndsAt string `json:"supportEndsAt,omitempty"`
}

type SupportedTarget struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}
