import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

export interface RunOptions {
  cwd?: string;
  env?: NodeJS.ProcessEnv;
  timeoutMilliseconds?: number;
}

export interface RunResult {
  stderr: string;
  stdout: string;
}

export interface ProcessRunner {
  run(
    command: string,
    args: readonly string[],
    options?: RunOptions,
  ): Promise<RunResult>;
}

export class ProcessFailure extends Error {
  override readonly name = "ProcessFailure";
  readonly command: string;
  readonly causeValue: unknown;

  constructor(command: string, causeValue: unknown) {
    const detail =
      causeValue instanceof Error ? causeValue.message : String(causeValue);
    super(`${command} failed: ${detail}`);
    this.command = command;
    this.causeValue = causeValue;
  }
}

export class ExecFileProcessRunner implements ProcessRunner {
  async run(
    command: string,
    args: readonly string[],
    options: RunOptions = {},
  ): Promise<RunResult> {
    try {
      const result = await execFileAsync(command, [...args], {
        cwd: options.cwd,
        encoding: "utf8",
        env: options.env ?? process.env,
        maxBuffer: 5 * 1024 * 1024,
        timeout: options.timeoutMilliseconds ?? 30_000,
        windowsHide: true,
      });
      return { stderr: result.stderr, stdout: result.stdout };
    } catch (error) {
      throw new ProcessFailure([command, ...args].join(" "), error);
    }
  }
}
