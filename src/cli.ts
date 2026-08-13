#!/usr/bin/env node

import { Command, Option } from "commander";
import { readFile } from "node:fs/promises";

import { GitHubAuthorityClient, readAuthoritySnapshot } from "./authority.js";
import { getPackageMetadata } from "./metadata.js";
import { sha256 } from "./hashing.js";
import { applyPlan, createPlan } from "./plan.js";
import { ExecFileProcessRunner } from "./process.js";
import { createReport, type GoldenPathReport } from "./report.js";
import { inspectRepository } from "./repository.js";
import { checkAgentSkill, installAgentSkill, type AgentHost } from "./skill.js";

const supportedHosts: AgentHost[] = ["codex", "claude-code"];

function selectedHosts(host: "all" | AgentHost): AgentHost[] {
  return host === "all" ? supportedHosts : [host];
}

function renderReport(report: GoldenPathReport): string {
  const lines = [
    `classification: ${report.classification}`,
    `authority: ${report.authority ? `${report.authority.repository}@${report.authority.commit}` : "unavailable"}`,
    `observation: ${report.observation.startedAt} .. ${report.observation.endedAt}`,
    `package: ${report.package.name}@${report.package.version} (${report.package.contentSha256}, ${report.package.contentScope})`,
    `repository HEAD: ${report.repository.snapshot.head}`,
    `worktree: ${report.repository.snapshot.dirty ? "dirty" : "clean"}`,
    `applicability: ${report.repository.applicability.classification} (${report.repository.applicability.detail})`,
    `native profiles: ${report.repository.nativeSurfaces.map((surface) => `${surface.profile}@${surface.root}`).join(", ") || "none observed"}`,
    `inventory query: ${report.repository.observationScope.query}`,
    `skills: ${report.skills.map((skill) => `${skill.host}=${skill.status}${skill.contentSha256 ? `@${skill.contentSha256}` : ""}`).join(", ")}`,
    `root Just recipes: ${report.repository.justRecipes.join(", ") || "none observed"}`,
    `direct just ci workflows: ${report.repository.directJustCiWorkflows.join(", ") || "none observed"}`,
    "observations:",
    ...report.observations.map(
      (observation) =>
        `  ${observation.level.toUpperCase()} ${observation.code}: ${observation.detail}`,
    ),
    `boundary: ${report.disclaimer}`,
  ];
  return lines.join("\n");
}

function writeValue(value: unknown, json: boolean): void {
  if (json) {
    console.log(JSON.stringify(value, null, 2));
  } else if (typeof value === "string") {
    console.log(value);
  } else {
    console.log(JSON.stringify(value, null, 2));
  }
}

export async function runCli(
  argv: readonly string[] = process.argv,
): Promise<void> {
  const metadata = await getPackageMetadata();
  const runner = new ExecFileProcessRunner();
  const authorityClient = new GitHubAuthorityClient(runner);
  const program = new Command()
    .name("golden-path-agent")
    .description(
      "Explicit local support for the contract-backed, repository-owned FiftyTen Developer Tooling Golden Path.",
    )
    .version(metadata.version)
    .showHelpAfterError();

  const skill = program
    .command("skill")
    .description("Install or check the explicit-invocation local Skill");

  skill
    .command("install")
    .description(
      "Install or update the packaged Skill in official user locations",
    )
    .addOption(
      new Option("--host <host>", "target host")
        .choices(["codex", "claude-code", "all"])
        .makeOptionMandatory(),
    )
    .option(
      "--home <directory>",
      "override the user home used for destination resolution",
    )
    .option("--force", "replace locally modified managed content", false)
    .action(
      async (options: {
        force: boolean;
        home?: string;
        host: "all" | AgentHost;
      }) => {
        for (const host of selectedHosts(options.host)) {
          const result = await installAgentSkill({
            force: options.force,
            host,
            ...(options.home ? { homeDirectory: options.home } : {}),
          });
          console.log(`${result.status} ${result.host}: ${result.destination}`);
        }
      },
    );

  skill
    .command("check")
    .description(
      "Check installed Skill ownership, package version, and content",
    )
    .addOption(
      new Option("--host <host>", "target host")
        .choices(["codex", "claude-code", "all"])
        .makeOptionMandatory(),
    )
    .option(
      "--home <directory>",
      "override the user home used for destination resolution",
    )
    .action(async (options: { home?: string; host: "all" | AgentHost }) => {
      for (const host of selectedHosts(options.host)) {
        const result = await checkAgentSkill({
          host,
          ...(options.home ? { homeDirectory: options.home } : {}),
        });
        console.log(
          `${result.status} ${result.host}: ${result.destination} (${result.detail})`,
        );
        if (result.status !== "current") {
          process.exitCode = 1;
        }
      }
    });

  program
    .command("info")
    .description(
      "Resolve exact current authority and repository identity without writing",
    )
    .option("--root <directory>", "target repository", ".")
    .option("--json", "emit JSON", false)
    .action(async (options: { json: boolean; root: string }) => {
      const startedAt = new Date().toISOString();
      const [authority, repository] = await Promise.all([
        readAuthoritySnapshot(authorityClient),
        inspectRepository(runner, options.root),
      ]);
      writeValue(
        {
          authority,
          observation: {
            endedAt: new Date().toISOString(),
            startedAt,
          },
          package: metadata,
          repository,
        },
        options.json,
      );
    });

  for (const commandName of ["doctor", "check"] as const) {
    program
      .command(commandName)
      .description(
        commandName === "doctor"
          ? "Inspect local readiness and repository-owned surfaces without writing"
          : "Emit a report-only factual inventory without running repository CI",
      )
      .option("--root <directory>", "target repository", ".")
      .option("--json", "emit JSON", false)
      .action(async (options: { json: boolean; root: string }) => {
        const report = await createReport({
          authorityClient,
          root: options.root,
          runner,
        });
        writeValue(options.json ? report : renderReport(report), options.json);
        if (
          report.observations.some(
            (observation) => observation.level === "error",
          )
        ) {
          process.exitCode = 2;
        }
      });
  }

  program
    .command("plan")
    .description(
      "Create an external hash-bound plan for named copy-once examples",
    )
    .option("--root <directory>", "target repository", ".")
    .requiredOption("--request <file>", "external JSON copy request")
    .requiredOption("--output <file>", "new external plan output")
    .option("--json", "emit the complete plan JSON", false)
    .action(
      async (options: {
        json: boolean;
        output: string;
        request: string;
        root: string;
      }) => {
        const plan = await createPlan({ authorityClient, runner, ...options });
        const planSha256 = sha256(await readFile(options.output));
        if (options.json) {
          writeValue(plan, true);
          console.error(`approved plan digest: ${planSha256}`);
        } else {
          console.log(
            `planned ${plan.operations.length} copy-once operation(s) from ${plan.source.repository}@${plan.source.commit}`,
          );
          console.log("No repository files were written.");
          console.log(`approved plan digest: ${planSha256}`);
        }
      },
    );

  program
    .command("apply")
    .description(
      "Apply one unchanged external plan after revalidating every boundary",
    )
    .option("--root <directory>", "target repository", ".")
    .requiredOption(
      "--plan <file>",
      "external plan produced by this exact package version",
    )
    .requiredOption(
      "--approved-plan-sha256 <digest>",
      "exact sha256:<hex> digest reviewed and approved by the user",
    )
    .option("--json", "emit complete apply evidence as JSON", false)
    .action(
      async (options: {
        approvedPlanSha256: string;
        json: boolean;
        plan: string;
        root: string;
      }) => {
        const result = await applyPlan({ authorityClient, runner, ...options });
        if (options.json) {
          writeValue(result, true);
          return;
        }
        for (const destination of result.written) {
          console.log(`written ${destination}`);
        }
        console.log(`source ${result.sourceCommit}`);
        console.log(`approved plan ${result.approvedPlanSha256}`);
        console.log(
          `observation ${result.observation.startedAt} .. ${result.observation.endedAt}`,
        );
        console.log(
          "Copied files are now repository-owned; no managed footprint was recorded.",
        );
      },
    );

  await program.parseAsync([...argv]);
}

runCli().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  console.error(`golden-path-agent: ${message}`);
  process.exitCode = 1;
});
