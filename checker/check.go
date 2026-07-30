package checker

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"
)

var validEnforcement = map[string]bool{
	"report-only":       true,
	"policy-required":   true,
	"platform-enforced": true,
}

func Check(options Options) Result {
	result := Result{
		SchemaVersion:   "golden-path-checker-output/v1",
		ContractVersion: ContractVersion,
		StandardVersion: StandardVersion,
		CheckerVersion:  Version,
		EvaluatedAt:     options.EvaluatedAt.UTC().Format(time.RFC3339),
		Enforcement:     options.Enforcement,
		Complete:        false,
		Profiles:        []string{},
		Findings:        []Finding{},
	}

	catalog, digest, err := loadSnapshot()
	if err != nil {
		return internalResult(result, "The bundled standard snapshot could not be verified.")
	}
	result.CatalogDigest = digest
	compatibility, err := loadCompatibility()
	if err != nil {
		return internalResult(result, "The bundled checker compatibility manifest could not be verified.")
	}

	if options.Root == "" {
		return configurationResult(result, "DT-META-001", ".github/golden-path.yaml", "Repository root is required.")
	}
	if options.EvaluatedAt.IsZero() {
		return configurationResult(result, "DT-CONF-001", ".", "An explicit UTC evaluation time is required.")
	}
	if !validEnforcement[options.Enforcement] {
		return configurationResult(result, "DT-CONF-001", ".", "Enforcement must be report-only, policy-required, or platform-enforced.")
	}
	if options.Enforcement != "report-only" {
		return configurationResult(result, "DT-CONF-001", ".", "Checker 0.x supports report-only enforcement only.")
	}

	metadata, err := loadMetadata(options.Root)
	if err != nil {
		message := "Golden Path metadata is missing or invalid."
		if !errors.Is(err, errNotFound) {
			message = "Golden Path metadata does not satisfy golden-path-metadata/v1."
		}
		return configurationResult(result, "DT-META-001", ".github/golden-path.yaml", message)
	}
	result.Profiles = sortedCopy(metadata.Profiles)
	if metadata.StandardVersion != StandardVersion || metadata.ContractVersion != ContractVersion {
		return configurationResult(result, "DT-META-001", ".github/golden-path.yaml", "The selected standard or contract version is unsupported by this checker.")
	}
	if findingPath, message := validateProfileDeclarations(options.Root, metadata); message != "" {
		return configurationResult(result, "DT-META-001", findingPath, message)
	}

	exceptions, exceptionsPresent, err := loadExceptions(options.Root)
	if err != nil {
		return configurationResult(result, "DT-EXC-001", ".github/golden-path-exceptions.yaml", "Golden Path exceptions do not satisfy golden-path-exceptions/v1.")
	}
	if issue := validateExceptionSemantics(exceptions, catalog, options.EvaluatedAt); issue != "" {
		return configurationResult(result, "DT-EXC-001", ".github/golden-path-exceptions.yaml", issue)
	}

	approachingExceptions := map[string]string{}
	for _, rule := range catalog.Rules {
		finding := evaluateRule(options.Root, metadata, rule, exceptionsPresent, options.EvaluatedAt, compatibility)
		if finding.Status == "fail" {
			if exception, expired := matchingException(exceptions.Exceptions, metadata, finding, rule, options.EvaluatedAt); exception != nil {
				if expired {
					finding.Message += " Matching exception " + exception.ID + " expired before the evaluation time."
				} else {
					finding.Status = "waived"
					finding.ExceptionID = &exception.ID
					finding.Message += " Waived by " + exception.ID + "."
					if finding.Extensions == nil {
						finding.Extensions = map[string]any{}
					}
					finding.Extensions["exceptionExpiresAt"] = exception.ExpiresAt
					if exceptionApproachesExpiry(exception.ExpiresAt, options.EvaluatedAt, compatibility.ExceptionExpiryWarningDays) {
						finding.Extensions["exceptionApproachingExpiry"] = true
						approachingExceptions[exception.ID] = exception.ExpiresAt
					}
				}
			}
		}
		result.Findings = append(result.Findings, finding)
	}
	applyExceptionExpiryWarning(result.Findings, approachingExceptions, compatibility.ExceptionExpiryWarningDays)

	sort.Slice(result.Findings, func(left, right int) bool {
		a, b := result.Findings[left], result.Findings[right]
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Secondary < b.Secondary
	})
	result.Complete = true
	result.Summary = summarize(result.Findings)
	if result.Summary.Error > 0 {
		result.ExitCode = errorExitCode(result.Findings)
		result.Complete = false
	} else if result.Summary.Fail > 0 {
		result.ExitCode = 1
	}
	return result
}

func configurationResult(result Result, ruleID, findingPath, message string) Result {
	result.ExitCode = 2
	result.Complete = false
	result.Findings = []Finding{{
		RuleID:      ruleID,
		Status:      "error",
		Severity:    "error",
		Assessment:  "automated",
		Path:        findingPath,
		Message:     message,
		Remediation: "Correct the repository-local configuration and run the checker again.",
		ExceptionID: nil,
		Extensions:  map[string]any{"errorKind": "configuration"},
	}}
	result.Summary = summarize(result.Findings)
	return result
}

func errorExitCode(findings []Finding) int {
	hasConfigurationError := false
	for _, finding := range findings {
		if finding.Status != "error" {
			continue
		}
		if finding.Extensions["errorKind"] == "configuration" {
			hasConfigurationError = true
			continue
		}
		return 3
	}
	if hasConfigurationError {
		return 2
	}
	return 3
}

func internalResult(result Result, message string) Result {
	result.ExitCode = 3
	result.Complete = false
	result.Findings = []Finding{{
		RuleID:      "DT-CONF-001",
		Status:      "error",
		Severity:    "error",
		Assessment:  "automated",
		Path:        ".",
		Message:     message,
		Remediation: "Verify the checker release checksum and report the failure through the security or maintenance path.",
		ExceptionID: nil,
	}}
	result.Summary = summarize(result.Findings)
	return result
}

func summarize(findings []Finding) Summary {
	var summary Summary
	for _, finding := range findings {
		switch finding.Status {
		case "pass":
			summary.Pass++
		case "fail":
			summary.Fail++
		case "warn":
			summary.Warn++
		case "skip":
			summary.Skip++
		case "waived":
			summary.Waived++
		case "error":
			summary.Error++
		}
	}
	return summary
}

func sortedCopy(values []string) []string {
	result := slices.Clone(values)
	sort.Strings(result)
	return result
}

func validateExceptionSemantics(exceptions ExceptionsFile, catalog Catalog, evaluatedAt time.Time) string {
	rules := make(map[string]Rule, len(catalog.Rules))
	for _, rule := range catalog.Rules {
		rules[rule.ID] = rule
	}
	ids := make(map[string]struct{}, len(exceptions.Exceptions))
	for _, exception := range exceptions.Exceptions {
		if _, exists := ids[exception.ID]; exists {
			return "Exception identifiers must be unique."
		}
		ids[exception.ID] = struct{}{}

		expiry, err := time.Parse("2006-01-02", exception.ExpiresAt)
		if err != nil {
			return "Exception expiry must be an ISO calendar date."
		}
		for _, authority := range exception.Approval.Authorities {
			approval, parseErr := time.Parse("2006-01-02", authority.ApprovedAt)
			if parseErr != nil {
				return "Exception approval dates must be ISO calendar dates."
			}
			if approval.After(expiry) || approval.After(evaluatedAt.UTC()) {
				return "Exception approval dates cannot be after expiry or the evaluation time."
			}
		}
		for _, ruleID := range exception.Rules {
			rule, exists := rules[ruleID]
			if !exists {
				return "An exception references an unknown rule."
			}
			if !rule.Waivable {
				return "An exception references a non-waivable rule."
			}
			if rule.Level != "MUST" && rule.Level != "MUST_NOT" {
				return "Exceptions may reference only MUST or MUST_NOT rules."
			}
			if rule.HighRisk && exception.RiskClass != "high" {
				return "A high-risk rule requires a high-risk exception."
			}
		}
	}
	return ""
}

func matchingException(exceptions []Exception, metadata Metadata, finding Finding, rule Rule, evaluatedAt time.Time) (*Exception, bool) {
	var validMatch, expiredMatch *Exception
	var validExpiry, expiredExpiry time.Time
	for index := range exceptions {
		exception := &exceptions[index]
		if !slices.Contains(exception.Rules, rule.ID) || !exceptionScopeMatches(exception.Scope, metadata, finding.Path) {
			continue
		}
		expiry, _ := time.Parse("2006-01-02", exception.ExpiresAt)
		expired := evaluatedAt.UTC().Format("2006-01-02") > expiry.Format("2006-01-02")
		if expired {
			if preferredException(exception, expiry, expiredMatch, expiredExpiry) {
				expiredMatch, expiredExpiry = exception, expiry
			}
			continue
		}
		if preferredException(exception, expiry, validMatch, validExpiry) {
			validMatch, validExpiry = exception, expiry
		}
	}
	if validMatch != nil {
		return validMatch, false
	}
	if expiredMatch != nil {
		return expiredMatch, true
	}
	return nil, false
}

func preferredException(candidate *Exception, candidateExpiry time.Time, current *Exception, currentExpiry time.Time) bool {
	return current == nil ||
		candidateExpiry.After(currentExpiry) ||
		candidateExpiry.Equal(currentExpiry) && candidate.ID < current.ID
}

func exceptionApproachesExpiry(expiresAt string, evaluatedAt time.Time, warningDays int) bool {
	expiry, err := time.Parse("2006-01-02", expiresAt)
	if err != nil {
		return false
	}
	evaluatedAt = evaluatedAt.UTC()
	evaluationDate := time.Date(evaluatedAt.Year(), evaluatedAt.Month(), evaluatedAt.Day(), 0, 0, 0, 0, time.UTC)
	days := int(expiry.Sub(evaluationDate).Hours() / 24)
	return days >= 0 && days <= warningDays
}

func applyExceptionExpiryWarning(findings []Finding, exceptions map[string]string, warningDays int) {
	if len(exceptions) == 0 {
		return
	}
	ids := make([]string, 0, len(exceptions))
	for id := range exceptions {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	details := make([]string, 0, len(ids))
	for _, id := range ids {
		details = append(details, id+" expires "+exceptions[id])
	}
	for index := range findings {
		if findings[index].RuleID != "DT-EXC-001" {
			continue
		}
		findings[index].Status = "warn"
		findings[index].Severity = "warning"
		findings[index].Message = fmt.Sprintf(
			"Matched exceptions are within the checker release's %d-day expiry warning window: %s.",
			warningDays,
			strings.Join(details, "; "),
		)
		if findings[index].Extensions == nil {
			findings[index].Extensions = map[string]any{}
		}
		findings[index].Extensions["exceptionExpiryWarningDays"] = warningDays
		findings[index].Extensions["approachingExceptionIds"] = ids
		return
	}
}

func exceptionScopeMatches(scope ExceptionScope, metadata Metadata, findingPath string) bool {
	if len(scope.Profiles) > 0 && !intersects(scope.Profiles, metadata.Profiles) {
		return false
	}
	if len(scope.ArtifactTypes) > 0 && !intersects(scope.ArtifactTypes, metadata.ArtifactTypes) {
		return false
	}
	if len(scope.Capabilities) > 0 && !intersects(scope.Capabilities, metadata.Capabilities) {
		return false
	}
	if len(scope.Paths) == 0 {
		return true
	}
	for _, candidate := range scope.Paths {
		if candidate == findingPath || strings.HasSuffix(candidate, "/") && strings.HasPrefix(findingPath, candidate) {
			return true
		}
		if matched, err := path.Match(candidate, findingPath); err == nil && matched {
			return true
		}
	}
	return false
}

func intersects(left, right []string) bool {
	for _, candidate := range left {
		if slices.Contains(right, candidate) {
			return true
		}
	}
	return false
}

func baseFinding(rule Rule, status, findingPath, message string) Finding {
	severity := rule.Severity
	if status == "warn" {
		severity = "warning"
	}
	return Finding{
		RuleID:      rule.ID,
		Status:      status,
		Severity:    severity,
		Assessment:  rule.Assessment,
		Path:        findingPath,
		Message:     message,
		Remediation: rule.Remediation,
		ExceptionID: nil,
	}
}

func deviationStatus(rule Rule) string {
	switch rule.Level {
	case "MUST", "MUST_NOT":
		return "fail"
	case "SHOULD", "SHOULD_NOT":
		return "warn"
	default:
		return "skip"
	}
}
