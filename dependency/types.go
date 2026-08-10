// Package dependency compiles repository-owned dependency facts into bounded
// adapter candidates without owning native graphs, release units, or CI.
package dependency

import "time"

const (
	PolicySchema      = "golden-path-dependency-policy/v1"
	DefersSchema      = "golden-path-dependency-defers/v1"
	ObservationSchema = "golden-path-dependency-observation/v2"
	CandidateSchema   = "golden-path-dependency-candidate/v1"
	ReportSchema      = "golden-path-dependency-report/v2"
	DefaultPRBudget   = 3
)

type Owner struct {
	Kind      string `json:"kind" yaml:"kind"`
	Reference string `json:"reference" yaml:"reference"`
}

type ReleaseFlow struct {
	IntegrationBranch string `json:"integrationBranch" yaml:"integrationBranch"`
	ReleaseBranch     string `json:"releaseBranch" yaml:"releaseBranch"`
}

type CommandReference struct {
	Kind             string `json:"kind" yaml:"kind"`
	WorkingDirectory string `json:"workingDirectory" yaml:"workingDirectory"`
	Recipe           string `json:"recipe" yaml:"recipe"`
}

type CIEvidenceReference struct {
	Kind     string `json:"kind" yaml:"kind"`
	Workflow string `json:"workflow" yaml:"workflow"`
	Job      string `json:"job" yaml:"job"`
}

type GateReference struct {
	Command    CommandReference      `json:"command" yaml:"command"`
	CIEvidence []CIEvidenceReference `json:"ciEvidence,omitempty" yaml:"ciEvidence,omitempty"`
}

type BudgetOverride struct {
	Reason      string `json:"reason" yaml:"reason"`
	Owner       Owner  `json:"owner" yaml:"owner"`
	ReviewAfter string `json:"reviewAfter" yaml:"reviewAfter"`
}

type RoutinePolicy struct {
	MaxOpenPullRequests int             `json:"maxOpenPullRequests,omitempty" yaml:"maxOpenPullRequests,omitempty"`
	BudgetOverride      *BudgetOverride `json:"budgetOverride,omitempty" yaml:"budgetOverride,omitempty"`
}

type SecurityRoute struct {
	Owner             Owner          `json:"owner" yaml:"owner"`
	CanonicalGate     *GateReference `json:"canonicalGate,omitempty" yaml:"canonicalGate,omitempty"`
	ManualRemediation string         `json:"manualRemediation,omitempty" yaml:"manualRemediation,omitempty"`
}

type RootPolicy struct {
	Owner            Owner         `json:"owner" yaml:"owner"`
	ReleaseFlow      ReleaseFlow   `json:"releaseFlow" yaml:"releaseFlow"`
	CanonicalGate    GateReference `json:"canonicalGate" yaml:"canonicalGate"`
	Routine          RoutinePolicy `json:"routine" yaml:"routine"`
	SecurityFallback SecurityRoute `json:"securityFallback" yaml:"securityFallback"`
}

type ValidationInvariant struct {
	Kind                    string `json:"kind" yaml:"kind"`
	InvariantReleaseUnitRef string `json:"invariantReleaseUnitRef" yaml:"invariantReleaseUnitRef"`
}

type AffectedArtifact struct {
	ReleaseUnitRef string `json:"releaseUnitRef" yaml:"releaseUnitRef"`
}

type RootBinding struct {
	NativeRootRef       string                `json:"nativeRootRef" yaml:"nativeRootRef"`
	ClassificationState string                `json:"classificationState" yaml:"classificationState"`
	AffectedArtifacts   []AffectedArtifact    `json:"affectedArtifacts,omitempty" yaml:"affectedArtifacts,omitempty"`
	ValidationOnly      []ValidationInvariant `json:"validationOnly,omitempty" yaml:"validationOnly,omitempty"`
	Owner               *Owner                `json:"owner,omitempty" yaml:"owner,omitempty"`
	ReleaseFlow         *ReleaseFlow          `json:"releaseFlow,omitempty" yaml:"releaseFlow,omitempty"`
	CanonicalGate       *GateReference        `json:"canonicalGate,omitempty" yaml:"canonicalGate,omitempty"`
	Routine             *RoutinePolicy        `json:"routine,omitempty" yaml:"routine,omitempty"`
	Security            *SecurityRoute        `json:"security,omitempty" yaml:"security,omitempty"`
	Reason              string                `json:"reason,omitempty" yaml:"reason,omitempty"`
	ReviewAfter         string                `json:"reviewAfter,omitempty" yaml:"reviewAfter,omitempty"`
}

type Policy struct {
	SchemaVersion string `json:"schemaVersion" yaml:"schemaVersion"`
	ReleaseUnits  struct {
		Path string `json:"path" yaml:"path"`
	} `json:"releaseUnits" yaml:"releaseUnits"`
	Adapter      string        `json:"adapter,omitempty" yaml:"adapter,omitempty"`
	Defaults     RootPolicy    `json:"defaults" yaml:"defaults"`
	RootBindings []RootBinding `json:"rootBindings" yaml:"rootBindings"`
}

type NativeRoot struct {
	ID       string   `json:"id" yaml:"id"`
	Path     string   `json:"path" yaml:"path"`
	Profiles []string `json:"profiles" yaml:"profiles"`
}

type EffectiveRoot struct {
	RootID               string                `json:"rootId"`
	Path                 string                `json:"path"`
	Profiles             []string              `json:"profiles"`
	Classification       string                `json:"classification"`
	AffectedReleaseUnits []string              `json:"affectedReleaseUnits"`
	ValidationOnly       []ValidationInvariant `json:"validationOnly"`
	Owner                Owner                 `json:"owner"`
	ReleaseFlow          ReleaseFlow           `json:"releaseFlow"`
	CanonicalGate        GateReference         `json:"canonicalGate"`
	RoutinePRBudget      int                   `json:"routinePRBudget"`
	Security             SecurityRoute         `json:"security"`
	GroupingKey          string                `json:"groupingKey,omitempty"`
	Reason               string                `json:"reason,omitempty"`
}

type Finding struct {
	RuleID      string `json:"ruleId"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Path        string `json:"path"`
	Secondary   string `json:"secondaryKey,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
}

type Evaluation struct {
	SchemaVersion string            `json:"schemaVersion"`
	Complete      bool              `json:"complete"`
	ExitCode      int               `json:"exitCode"`
	Roots         []EffectiveRoot   `json:"roots"`
	Findings      []Finding         `json:"findings"`
	RenderedFiles map[string][]byte `json:"-"`
}

type CandidateChange struct {
	Path          string `json:"path"`
	Ownership     string `json:"ownership"`
	Status        string `json:"status"`
	DesiredSHA256 string `json:"desiredSHA256,omitempty"`
	CurrentSHA256 string `json:"currentSHA256,omitempty"`
}

type CandidateRoot struct {
	NativeRootRef string `json:"nativeRootRef"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
}

type QueueAction struct {
	PullRequest string `json:"pullRequest"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
}

type Candidate struct {
	SchemaVersion string            `json:"schemaVersion"`
	Repository    string            `json:"repository"`
	PolicySHA256  string            `json:"policySHA256"`
	Changes       []CandidateChange `json:"changes"`
	Roots         []CandidateRoot   `json:"roots"`
	QueueActions  []QueueAction     `json:"queueActions"`
	ConflictCount int               `json:"conflictCount"`
}

type Observation struct {
	SchemaVersion string    `json:"schemaVersion"`
	ObservedAt    time.Time `json:"observedAt"`
	Scope         struct {
		Organization string   `json:"organization"`
		Query        string   `json:"query"`
		Repositories []string `json:"repositories,omitempty"`
	} `json:"scope"`
	Source struct {
		Kind       string `json:"kind"`
		APIVersion string `json:"apiVersion"`
		Identity   string `json:"identity"`
		SHA256     string `json:"sha256"`
	} `json:"source"`
	Repositories              []ObservedRepository       `json:"repositories"`
	PullRequests              []ObservedPullRequest      `json:"pullRequests"`
	Alerts                    []ObservedAlert            `json:"alerts"`
	NativeManagerObservations []NativeManagerObservation `json:"nativeManagerObservations,omitempty"`
}

type ObservedRepository struct {
	Repository             string `json:"repository"`
	RepositoryNodeID       string `json:"repositoryNodeId"`
	DefaultBranch          string `json:"defaultBranch"`
	DefaultBranchSHA       string `json:"defaultBranchSHA"`
	DependencyPolicySHA256 string `json:"dependencyPolicySHA256,omitempty"`
	NativeRootsSHA256      string `json:"nativeRootsSHA256,omitempty"`
	ReleaseUnitsSHA256     string `json:"releaseUnitsSHA256,omitempty"`
	DefersSHA256           string `json:"defersSHA256,omitempty"`
}

type RefIdentity struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}
type CheckRollup struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type ObservedPullRequest struct {
	Repository       string      `json:"repository"`
	RepositoryNodeID string      `json:"repositoryNodeId"`
	Number           int         `json:"number"`
	NodeID           string      `json:"nodeId"`
	URL              string      `json:"url"`
	Base             RefIdentity `json:"base"`
	Head             RefIdentity `json:"head"`
	Classification   string      `json:"classification"`
	State            string      `json:"state"`
	CreatedAt        string      `json:"createdAt"`
	UpdatedAt        string      `json:"updatedAt"`
	CheckRollup      CheckRollup `json:"checkRollup"`
	NativeRootRef    string      `json:"nativeRootRef,omitempty"`
	OwnerRoute       string      `json:"ownerRoute,omitempty"`
}

type ObservedAlert struct {
	Repository                string `json:"repository"`
	Number                    int    `json:"number"`
	AdvisoryIdentity          string `json:"advisoryIdentity"`
	State                     string `json:"state"`
	Severity                  string `json:"severity"`
	Dependency                string `json:"dependency"`
	Relationship              string `json:"relationship"`
	ManifestPath              string `json:"manifestPath,omitempty"`
	FixedIn                   string `json:"fixedIn,omitempty"`
	SecurityUpdatePullRequest string `json:"securityUpdatePullRequest,omitempty"`
}

type NativeManagerObservation struct {
	Repository    string `json:"repository"`
	NativeRootRef string `json:"nativeRootRef"`
	SourceURI     string `json:"sourceURI"`
	SourceVersion string `json:"sourceVersion"`
	ObservedAt    string `json:"observedAt"`
	SHA256        string `json:"sha256"`
}

type DefersFile struct {
	SchemaVersion string        `json:"schemaVersion" yaml:"schemaVersion"`
	Defers        []DeferRecord `json:"defers" yaml:"defers"`
	SHA256        string        `json:"-" yaml:"-"`
}

type DeferRecord struct {
	Ecosystem        string `json:"ecosystem" yaml:"ecosystem"`
	NativeRootRef    string `json:"nativeRootRef" yaml:"nativeRootRef"`
	Dependency       string `json:"dependency" yaml:"dependency"`
	VersionRange     string `json:"versionRange" yaml:"versionRange"`
	CurrentVersion   string `json:"currentVersion" yaml:"currentVersion"`
	AvailableVersion string `json:"availableVersion" yaml:"availableVersion"`
	RiskClass        string `json:"riskClass" yaml:"riskClass"`
	Reason           string `json:"reason" yaml:"reason"`
	Owner            string `json:"owner" yaml:"owner"`
	ObservedAt       string `json:"observedAt" yaml:"observedAt"`
	ReviewedAt       string `json:"reviewedAt" yaml:"reviewedAt"`
	ReviewAfter      string `json:"reviewAfter" yaml:"reviewAfter"`
	Source           string `json:"source,omitempty" yaml:"source,omitempty"`
	WorkLink         string `json:"workLink,omitempty" yaml:"workLink,omitempty"`
}

type RepositoryReport struct {
	Repository                string  `json:"repository"`
	DefaultBranchSHA          string  `json:"defaultBranchSHA"`
	Applicability             string  `json:"applicability"`
	ClassifiedRoots           int     `json:"classifiedRoots"`
	PendingRoots              int     `json:"pendingRoots"`
	RoutineOpen               int     `json:"routineOpen"`
	SecurityOpen              int     `json:"securityOpen"`
	OpenAlerts                int     `json:"openAlerts"`
	OpenAdvisoryGroups        int     `json:"openAdvisoryGroups"`
	PartialSecurityAdvisories int     `json:"partialSecurityAdvisories"`
	OwnerRoutingCoverage      float64 `json:"ownerRoutingCoverage"`
	OldestRoutineObservedAt   string  `json:"oldestRoutineObservedAt,omitempty"`
	Deferred                  int     `json:"deferred,omitempty"`
	Stale                     bool    `json:"stale,omitempty"`
}

type SecurityAlertInstanceReport struct {
	Number                    int    `json:"number"`
	Severity                  string `json:"severity"`
	Relationship              string `json:"relationship"`
	ManifestPath              string `json:"manifestPath,omitempty"`
	FixedIn                   string `json:"fixedIn,omitempty"`
	SecurityUpdatePullRequest string `json:"securityUpdatePullRequest,omitempty"`
}

type SecurityAdvisoryReport struct {
	Repository          string                        `json:"repository"`
	AdvisoryIdentity    string                        `json:"advisoryIdentity"`
	Dependency          string                        `json:"dependency"`
	RemediationCoverage string                        `json:"remediationCoverage"`
	OpenAlertInstances  []SecurityAlertInstanceReport `json:"openAlertInstances"`
}

type Report struct {
	SchemaVersion       string `json:"schemaVersion"`
	GeneratedAt         string `json:"generatedAt"`
	ObservedAt          string `json:"observedAt"`
	ObservationIdentity string `json:"observationIdentity"`
	Scope               struct {
		Organization string `json:"organization"`
		Query        string `json:"query"`
	} `json:"scope"`
	Repositories       []RepositoryReport       `json:"repositories"`
	SecurityAdvisories []SecurityAdvisoryReport `json:"securityAdvisories"`
}
