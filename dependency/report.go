package dependency

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const maxObservationBytes = 16 << 20

type securityAdvisoryKey struct {
	repository       string
	advisoryIdentity string
	ecosystem        string
	dependency       string
}

func DecodeUnsealedObservation(reader io.Reader) (Observation, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxObservationBytes+1))
	if err != nil {
		return Observation{}, err
	}
	if len(data) > maxObservationBytes {
		return Observation{}, fmt.Errorf("observation exceeds the input limit")
	}
	var observation Observation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&observation); err != nil {
		return Observation{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Observation{}, fmt.Errorf("observation must contain exactly one JSON document")
	}
	return observation, nil
}

func DecodeObservation(reader io.Reader) (Observation, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxObservationBytes+1))
	if err != nil {
		return Observation{}, err
	}
	if len(data) > maxObservationBytes {
		return Observation{}, fmt.Errorf("observation exceeds the input limit")
	}
	if err := validateSchema("golden-path-dependency-observation-v2.schema.json", data); err != nil {
		return Observation{}, err
	}
	var observation Observation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&observation); err != nil {
		return Observation{}, err
	}
	digest, err := observationDigest(data)
	if err != nil {
		return Observation{}, err
	}
	if observation.Source.SHA256 != digest || observation.Source.Identity != "urn:sha256:"+digest {
		return Observation{}, fmt.Errorf("observation source identity does not match its canonical payload")
	}
	return observation, nil
}

func observationDigest(data []byte) (string, error) {
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return "", err
	}
	source, ok := document["source"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("observation source is missing")
	}
	delete(source, "identity")
	delete(source, "sha256")
	canonical, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

// SealObservation canonicalizes an unsealed observation and sets its source
// SHA-256 URN. Callers still own live source collection and classification.
func SealObservation(observation Observation) ([]byte, error) {
	observation.SchemaVersion = ObservationSchema
	if observation.Repositories == nil {
		observation.Repositories = []ObservedRepository{}
	}
	if observation.PullRequests == nil {
		observation.PullRequests = []ObservedPullRequest{}
	}
	if observation.Alerts == nil {
		observation.Alerts = []ObservedAlert{}
	}
	observation.Source.Identity = ""
	observation.Source.SHA256 = ""
	data, err := json.Marshal(observation)
	if err != nil {
		return nil, err
	}
	digest, err := observationDigest(data)
	if err != nil {
		return nil, err
	}
	observation.Source.SHA256 = digest
	observation.Source.Identity = "urn:sha256:" + digest
	data, err = json.MarshalIndent(observation, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if err := validateSchema("golden-path-dependency-observation-v2.schema.json", data); err != nil {
		return nil, err
	}
	return data, nil
}

func DecodeDefers(reader io.Reader) (DefersFile, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxRepositoryInput+1))
	if err != nil {
		return DefersFile{}, err
	}
	if len(data) > maxRepositoryInput {
		return DefersFile{}, fmt.Errorf("defer file exceeds the input limit")
	}
	var defers DefersFile
	jsonData, err := decodeYAML(data, &defers)
	if err != nil {
		return DefersFile{}, err
	}
	if err := validateSchema("golden-path-dependency-defers-v1.schema.json", jsonData); err != nil {
		return DefersFile{}, err
	}
	digest := sha256.Sum256(data)
	defers.SHA256 = hex.EncodeToString(digest[:])
	return defers, nil
}

func GenerateReport(observation Observation, candidates map[string]Candidate, defers map[string]DefersFile, notApplicable map[string]bool, generatedAt time.Time) (Report, error) {
	observationData, err := json.Marshal(observation)
	if err != nil {
		return Report{}, err
	}
	if err := validateSchema("golden-path-dependency-observation-v2.schema.json", observationData); err != nil {
		return Report{}, fmt.Errorf("observation violates bundled schema: %w", err)
	}
	digest, err := observationDigest(observationData)
	if err != nil || observation.Source.SHA256 != digest || observation.Source.Identity != "urn:sha256:"+digest {
		return Report{}, fmt.Errorf("observation source identity does not match its canonical payload")
	}
	if generatedAt.IsZero() || generatedAt.Location() != time.UTC {
		return Report{}, fmt.Errorf("generatedAt must be explicit UTC")
	}
	if observation.ObservedAt.IsZero() || generatedAt.Before(observation.ObservedAt) {
		return Report{}, fmt.Errorf("generatedAt must not precede observedAt")
	}
	report := Report{
		SchemaVersion:       ReportSchema,
		GeneratedAt:         generatedAt.Format(time.RFC3339),
		ObservedAt:          observation.ObservedAt.UTC().Format(time.RFC3339),
		ObservationIdentity: observation.Source.Identity,
		Repositories:        []RepositoryReport{},
		SecurityAdvisories:  []SecurityAdvisoryReport{},
	}
	report.Scope.Organization = observation.Scope.Organization
	report.Scope.Query = observation.Scope.Query
	rows := make(map[string]*RepositoryReport, len(observation.Repositories))
	for _, source := range observation.Repositories {
		row := RepositoryReport{
			Repository: source.Repository, DefaultBranchSHA: source.DefaultBranchSHA,
			Applicability: "unknown", OwnerRoutingCoverage: 1,
		}
		candidate, present, candidateErr := candidateForRepository(candidates, source.Repository)
		if candidateErr != nil {
			return Report{}, candidateErr
		}
		if present {
			if !repositoryMatches(source.Repository, candidate.Repository) || source.DependencyPolicySHA256 == "" || strings.TrimPrefix(candidate.PolicySHA256, "sha256:") != source.DependencyPolicySHA256 {
				return Report{}, fmt.Errorf("candidate for %s is not bound to the observed dependency policy", source.Repository)
			}
			row.Applicability = "applicable"
			for _, root := range candidate.Roots {
				if root.Status == "classified" {
					row.ClassifiedRoots++
				} else {
					row.PendingRoots++
				}
			}
		}
		if repositoryMarkedNotApplicable(notApplicable, source.Repository) {
			row.Applicability = "not-applicable"
		}
		copyOfRow := row
		rows[source.Repository] = &copyOfRow
	}
	totalPRs := map[string]int{}
	routedPRs := map[string]int{}
	closureEvidenceByPullRequest := map[string][]SecurityClosureEvidence{}
	for _, pullRequest := range observation.PullRequests {
		row := rows[pullRequest.Repository]
		if row == nil {
			continue
		}
		totalPRs[pullRequest.Repository]++
		if pullRequest.OwnerRoute != "" {
			routedPRs[pullRequest.Repository]++
		}
		closureEvidenceByPullRequest[pullRequest.URL] = append(
			closureEvidenceByPullRequest[pullRequest.URL],
			pullRequest.SecurityClosureEvidence...,
		)
		switch pullRequest.Classification {
		case "security-update", "security-remediation":
			row.SecurityOpen++
		default:
			row.RoutineOpen++
			if row.OldestRoutineObservedAt == "" || pullRequest.CreatedAt < row.OldestRoutineObservedAt {
				row.OldestRoutineObservedAt = pullRequest.CreatedAt
			}
		}
	}
	advisoryGroups := make(map[securityAdvisoryKey][]SecurityAlertInstanceReport)
	seenAlerts := make(map[string]bool, len(observation.Alerts))
	for _, alert := range observation.Alerts {
		alertKey := fmt.Sprintf("%s#%d", alert.Repository, alert.Number)
		if seenAlerts[alertKey] {
			return Report{}, fmt.Errorf("observation contains duplicate alert %s", alertKey)
		}
		seenAlerts[alertKey] = true
		if alert.State != "open" {
			continue
		}
		row := rows[alert.Repository]
		if row == nil {
			return Report{}, fmt.Errorf("open alert %s is outside the observed repository scope", alertKey)
		}
		row.OpenAlerts++
		key := securityAdvisoryKey{
			repository:       alert.Repository,
			advisoryIdentity: alert.AdvisoryIdentity,
			ecosystem:        alert.Ecosystem,
			dependency:       alert.Dependency,
		}
		advisoryGroups[key] = append(advisoryGroups[key], SecurityAlertInstanceReport{
			Number: alert.Number, Severity: alert.Severity, Relationship: alert.Relationship, ManifestPath: alert.ManifestPath,
			FixedIn: alert.FixedIn, SecurityUpdatePullRequest: alert.SecurityUpdatePullRequest,
		})
	}
	for key, instances := range advisoryGroups {
		sort.Slice(instances, func(left, right int) bool {
			if instances[left].Number != instances[right].Number {
				return instances[left].Number < instances[right].Number
			}
			return instances[left].ManifestPath < instances[right].ManifestPath
		})
		linked := 0
		for _, instance := range instances {
			if instance.SecurityUpdatePullRequest != "" {
				linked++
			}
		}
		coverage := "none"
		if linked == len(instances) {
			coverage = "all-linked"
		} else if linked > 0 {
			coverage = "partial"
		}
		row := rows[key.repository]
		row.OpenAdvisoryGroups++
		if coverage == "partial" {
			row.PartiallyLinkedAdvisoryGroups++
		}
		closureEvidence := make([]SecurityClosureEvidence, 0)
		for _, instance := range instances {
			closureEvidence = append(
				closureEvidence,
				closureEvidenceByPullRequest[instance.SecurityUpdatePullRequest]...,
			)
		}
		closureEvidence = sortedUniqueClosureEvidence(closureEvidence)
		report.SecurityAdvisories = append(report.SecurityAdvisories, SecurityAdvisoryReport{
			Repository: key.repository, AdvisoryIdentity: key.advisoryIdentity,
			Ecosystem: key.ecosystem, Dependency: key.dependency,
			RemediationCoverage: coverage, OpenAlertInstances: instances,
			SecurityClosureEvidence: closureEvidence,
		})
	}
	for repository, records := range defers {
		observedRepository, found := observedRepositoryForName(observation.Repositories, repository)
		if !found {
			return Report{}, fmt.Errorf("defer file repository %s is outside the observation scope", repository)
		}
		if records.SHA256 == "" || observedRepository.DefersSHA256 == "" || records.SHA256 != observedRepository.DefersSHA256 {
			return Report{}, fmt.Errorf("defer file for %s is not bound to the observation", repository)
		}
		if row := rows[observedRepository.Repository]; row != nil {
			row.Deferred = len(records.Defers)
		}
	}
	for repository, row := range rows {
		if totalPRs[repository] != 0 {
			row.OwnerRoutingCoverage = float64(routedPRs[repository]) / float64(totalPRs[repository])
		}
		row.Stale = generatedAt.Sub(observation.ObservedAt) > 7*24*time.Hour
		report.Repositories = append(report.Repositories, *row)
	}
	sort.Slice(report.Repositories, func(left, right int) bool {
		return report.Repositories[left].Repository < report.Repositories[right].Repository
	})
	sort.Slice(report.SecurityAdvisories, func(left, right int) bool {
		a, b := report.SecurityAdvisories[left], report.SecurityAdvisories[right]
		if a.Repository != b.Repository {
			return a.Repository < b.Repository
		}
		if a.AdvisoryIdentity != b.AdvisoryIdentity {
			return a.AdvisoryIdentity < b.AdvisoryIdentity
		}
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		return a.Dependency < b.Dependency
	})
	data, err := json.Marshal(report)
	if err != nil {
		return Report{}, err
	}
	if err := validateSchema("golden-path-dependency-report-v2.schema.json", data); err != nil {
		return Report{}, err
	}
	return report, nil
}

func sortedUniqueClosureEvidence(evidence []SecurityClosureEvidence) []SecurityClosureEvidence {
	sort.Slice(evidence, func(left, right int) bool {
		a, b := evidence[left], evidence[right]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Workflow != b.Workflow {
			return a.Workflow < b.Workflow
		}
		if a.Job != b.Job {
			return a.Job < b.Job
		}
		if a.RunID != b.RunID {
			return a.RunID < b.RunID
		}
		if a.RunAttempt != b.RunAttempt {
			return a.RunAttempt < b.RunAttempt
		}
		if a.HeadSHA != b.HeadSHA {
			return a.HeadSHA < b.HeadSHA
		}
		if a.RunURL != b.RunURL {
			return a.RunURL < b.RunURL
		}
		if a.Status != b.Status {
			return a.Status < b.Status
		}
		if a.Conclusion != b.Conclusion {
			return a.Conclusion < b.Conclusion
		}
		return a.ObservedAt < b.ObservedAt
	})
	unique := make([]SecurityClosureEvidence, 0, len(evidence))
	for _, item := range evidence {
		if len(unique) == 0 || unique[len(unique)-1] != item {
			unique = append(unique, item)
		}
	}
	return unique
}

func candidateForRepository(candidates map[string]Candidate, repository string) (Candidate, bool, error) {
	if candidate, ok := candidates[repository]; ok {
		return candidate, true, nil
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		if repositoryMatches(repository, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > 1 {
		return Candidate{}, false, fmt.Errorf("multiple candidates match repository %s", repository)
	}
	if len(names) == 1 {
		return candidates[names[0]], true, nil
	}
	return Candidate{}, false, nil
}

func observedRepositoryForName(repositories []ObservedRepository, name string) (ObservedRepository, bool) {
	var matches []ObservedRepository
	for _, repository := range repositories {
		if repositoryMatches(repository.Repository, name) {
			matches = append(matches, repository)
		}
	}
	if len(matches) != 1 {
		return ObservedRepository{}, false
	}
	return matches[0], true
}

func repositoryMarkedNotApplicable(notApplicable map[string]bool, repository string) bool {
	for name, selected := range notApplicable {
		if selected && repositoryMatches(repository, name) {
			return true
		}
	}
	return false
}
