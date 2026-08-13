import type { AuthorityClient, AuthoritySnapshot } from "./authority.js";
import { readAuthoritySnapshot } from "./authority.js";
import { getPackageMetadata, type PackageMetadata } from "./metadata.js";
import type { ProcessRunner } from "./process.js";
import type { RepositoryInventory } from "./repository.js";
import { inspectRepository } from "./repository.js";
import {
  checkAgentSkill,
  resolveAgentSkillDestination,
  type AgentHost,
  type SkillCheckResult,
} from "./skill.js";

export type ObservationLevel = "error" | "info" | "warning";

export interface Observation {
  code: string;
  detail: string;
  level: ObservationLevel;
}

export interface GoldenPathReport {
  authority: AuthoritySnapshot | null;
  authorityError: string | null;
  classification: "report-only";
  disclaimer: string;
  observation: {
    endedAt: string;
    startedAt: string;
  };
  observations: Observation[];
  package: PackageMetadata;
  repository: RepositoryInventory;
  schemaVersion: 1;
  skills: ReportSkillEvidence[];
}

export interface ReportSkillEvidence extends Omit<SkillCheckResult, "status"> {
  status: SkillCheckResult["status"] | "error";
}

function observeRepository(repository: RepositoryInventory): Observation[] {
  const observations: Observation[] = [];
  observations.push({
    code: `repository.applicability.${repository.applicability.classification}`,
    detail: repository.applicability.detail,
    level:
      repository.applicability.classification === "pending-classification"
        ? "warning"
        : "info",
  });
  if (!repository.files.justfile) {
    observations.push({
      code: "repository.justfile.missing",
      detail: "No root justfile was observed.",
      level: "warning",
    });
  } else {
    for (const recipe of ["init", "check", "ci"]) {
      if (!repository.justRecipes.includes(recipe)) {
        observations.push({
          code: `repository.just.${recipe}.not-observed`,
          detail: `The off-the-shelf Just summary did not expose a root ${recipe} recipe.`,
          level: "warning",
        });
      }
    }
  }
  for (const surface of repository.nativeSurfaces) {
    if (!surface.lockPresent && surface.lock) {
      observations.push({
        code: "repository.native-lock.not-observed",
        detail: `${surface.manifest} was observed without ${surface.lock}.`,
        level: "warning",
      });
    }
  }
  if (repository.workflowFiles.length === 0) {
    observations.push({
      code: "repository.workflow.not-observed",
      detail: "No repository-owned YAML workflow was observed.",
      level: "warning",
    });
  } else if (repository.directJustCiWorkflows.length === 0) {
    observations.push({
      code: "repository.workflow.just-ci.not-observed",
      detail: "No workflow with a direct `just ci` invocation was observed.",
      level: "warning",
    });
  }
  observations.push({
    code: repository.snapshot.dirty
      ? "repository.worktree.dirty"
      : "repository.worktree.clean",
    detail: repository.snapshot.dirty
      ? "The repository worktree has observed changes."
      : "The repository worktree is clean.",
    level: "info",
  });
  return observations;
}

export async function createReport(options: {
  authorityClient: AuthorityClient;
  homeDirectory?: string;
  root: string;
  runner: ProcessRunner;
}): Promise<GoldenPathReport> {
  const startedAt = new Date().toISOString();
  async function observeSkill(host: AgentHost): Promise<ReportSkillEvidence> {
    const skillOptions = {
      host,
      ...(options.homeDirectory
        ? { homeDirectory: options.homeDirectory }
        : {}),
    };
    try {
      return await checkAgentSkill(skillOptions);
    } catch (error) {
      return {
        contentSha256: null,
        destination: resolveAgentSkillDestination(skillOptions),
        detail: error instanceof Error ? error.message : String(error),
        host,
        packageVersion: null,
        status: "error",
      };
    }
  }
  const [repository, packageMetadata, skills] = await Promise.all([
    inspectRepository(options.runner, options.root),
    getPackageMetadata(),
    Promise.all(
      (["codex", "claude-code"] as const).map((host) => observeSkill(host)),
    ),
  ]);
  let authority: AuthoritySnapshot | null = null;
  let authorityError: string | null = null;
  try {
    authority = await readAuthoritySnapshot(options.authorityClient);
  } catch (error) {
    authorityError = error instanceof Error ? error.message : String(error);
  }

  const observations = observeRepository(repository);
  observations.unshift(
    authority
      ? {
          code: "authority.current-source.resolved",
          detail: `Resolved ${authority.repository}@${authority.commit}.`,
          level: "info",
        }
      : {
          code: "authority.current-source.unavailable",
          detail: authorityError ?? "Current authority source is unavailable.",
          level: "error",
        },
  );

  return {
    authority,
    authorityError,
    classification: "report-only",
    disclaimer:
      "This local inventory does not establish policy-required or platform-enforced conformance and does not run repository CI.",
    observation: { endedAt: new Date().toISOString(), startedAt },
    observations,
    package: packageMetadata,
    repository,
    schemaVersion: 1,
    skills,
  };
}
