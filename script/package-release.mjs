#!/usr/bin/env node
// Official local macOS release entry. It never publishes to GitHub.
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { accessSync, constants, existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, rmSync } from "node:fs";
import { homedir } from "node:os";
import { delimiter, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const APP_NAME = "pzssh";
const ARCHITECTURES = new Set(["arm64", "amd64", "universal"]);

export function parseArgs(args) {
  const options = { arch: process.arch === "x64" ? "amd64" : "arm64", check: false, help: false };
  for (let index = 0; index < args.length; index++) {
    const arg = args[index];
    if (arg === "--check") options.check = true;
    else if (arg === "--help" || arg === "-h") options.help = true;
    else if (arg === "--arch" || arg === "--out") {
      const value = args[++index];
      if (!value || value.startsWith("--")) throw new Error(`${arg} requires a value`);
      options[arg.slice(2)] = value;
    } else throw new Error(`Unknown argument: ${arg}`);
  }
  if (!ARCHITECTURES.has(options.arch)) throw new Error("--arch must be arm64, amd64, or universal");
  return options;
}

function plistString(text, name) {
  const value = text.match(new RegExp(`<key>\\s*${name}\\s*</key>\\s*<string>([^<]+)</string>`))?.[1];
  if (!value) throw new Error(`Info.plist is missing ${name}`);
  return value.trim();
}

export function readReleaseVersion(root) {
  const plist = readFileSync(join(root, "build/darwin/Info.plist"), "utf8");
  const config = readFileSync(join(root, "build/config.yml"), "utf8");
  const info = config.match(/^info:\s*\n([\s\S]*?)(?=^[^\s#]|$(?![\s\S]))/m)?.[1];
  const yamlValue = (key) => info?.match(new RegExp(`^  ${key}:\\s*(?:"([^"]+)"|'([^']+)'|([^#\\s]+))`, "m"))?.slice(1).find(Boolean);
  const version = plistString(plist, "CFBundleShortVersionString");
  if (!/^[0-9]+(?:\.[0-9]+){1,3}(?:[-+][A-Za-z0-9.-]+)?$/.test(version)) throw new Error("Invalid release version in Info.plist");
  if (plistString(plist, "CFBundleVersion") !== version || yamlValue("version") !== version) {
    throw new Error("Release versions differ: build/config.yml info.version and both Info.plist versions must agree");
  }
  if (plistString(plist, "CFBundleExecutable") !== APP_NAME || plistString(plist, "CFBundleIdentifier") !== yamlValue("productIdentifier")) {
    throw new Error("Wails configuration and macOS bundle identity differ");
  }
  return version;
}

function run(label, command, args, options = {}) {
  const { includeStderr = false, ...executionOptions } = options;
  const response = spawnSync(command, args, {
    encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], maxBuffer: 32 * 1024 * 1024,
    ...executionOptions,
  });
  if (response.error || response.status !== 0) throw new Error(`${label} failed (exit ${response.status ?? response.error?.code ?? "unknown"})`);
  return [response.stdout, includeStderr ? response.stderr : ""].filter(Boolean).join("\n").trim();
}

export function resolveWails(env = process.env) {
  const candidates = env.WAILS_BIN
    ? [resolve(env.WAILS_BIN)]
    : [...(env.PATH ?? "").split(delimiter).filter(Boolean).map((folder) => join(folder, "wails3")), join(homedir(), "go/bin/wails3")];
  for (const candidate of candidates) {
    try { accessSync(candidate, constants.X_OK); return candidate; } catch { /* Try the next configured executable. */ }
  }
  throw new Error("The project-matched Wails CLI is unavailable; install it or set WAILS_BIN");
}

function ensureClean(root, execute) {
  if (execute("Source cleanliness check", "git", ["status", "--porcelain"], { cwd: root })) {
    throw new Error("Official releases require a clean source tree; commit the intended source changes before packaging");
  }
}


function ensureSourceRevision(root, sourceCommit, execute) {
  ensureClean(root, execute);
  const current = execute("Source revision recheck", "git", ["rev-parse", "--verify", "HEAD"], { cwd: root });
  if (current !== sourceCommit) throw new Error("Source revision changed during packaging; the release cannot be marked READY");
}

function verifyBundle(app, version, arch, execute) {
  if (!existsSync(app) || !lstatSync(app).isDirectory()) throw new Error("The staged application bundle is missing");
  const plist = readFileSync(join(app, "Contents/Info.plist"), "utf8");
  if (plistString(plist, "CFBundleShortVersionString") !== version || plistString(plist, "CFBundleVersion") !== version || plistString(plist, "CFBundleExecutable") !== APP_NAME) {
    throw new Error("The staged application does not match the release version or executable");
  }
  const actual = new Set(execute("App architecture check", "/usr/bin/lipo", ["-archs", join(app, "Contents/MacOS", APP_NAME)]).split(/\s+/));
  const wanted = arch === "universal" ? ["arm64", "x86_64"] : [arch === "amd64" ? "x86_64" : arch];
  if (actual.size !== wanted.length || wanted.some((value) => !actual.has(value))) throw new Error("The staged application architecture differs from --arch");
  return true;
}

function verifyZip(archive, app, version, arch, signing, core, execute, workDir) {
  if (!existsSync(archive)) throw new Error("The final release ZIP is missing");
  const scratch = mkdtempSync(join(workDir, ".zip-verify-"));
  try {
    execute("Release ZIP extraction", "/usr/bin/ditto", ["-xk", archive, scratch]);
    const restored = join(scratch, `${APP_NAME}.app`);
    verifyBundle(restored, version, arch, execute);
    core.verifyApplication(restored, signing, { notarized: true });
    const hash = (path) => createHash("sha256").update(readFileSync(path)).digest("hex");
    if (hash(join(restored, "Contents/MacOS", APP_NAME)) !== hash(join(app, "Contents/MacOS", APP_NAME))) throw new Error("The ZIP contains a different application executable");
    return true;
  } finally { rmSync(scratch, { recursive: true, force: true }); }
}

export async function main(argv, { projectRoot = ROOT, core: providedCore, execute = run, findWails = resolveWails } = {}) {
  const options = parseArgs(argv);
  if (options.help) {
    console.log("Usage: node script/package-release.mjs [--check] [--arch arm64|amd64|universal] [--out directory]");
    console.log("The same command resumes an interrupted release for the same source commit and version.");
    return;
  }
  const version = readReleaseVersion(projectRoot);
  const wails = findWails();
  const requiredWails = readFileSync(join(projectRoot, "go.mod"), "utf8").match(/^\s*(?:require\s+)?github\.com\/wailsapp\/wails\/v3\s+(\S+)/m)?.[1];
  // The Wails CLI writes its successful version result to stderr.
  const installedWails = execute("Wails CLI version check", wails, ["version"], { cwd: projectRoot, includeStderr: true }).match(/v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?/)?.[0];
  if (!requiredWails || installedWails !== requiredWails) throw new Error("The installed Wails CLI must match the version in go.mod");
  const core = providedCore ?? await import("./lib/macos-signing.mjs");
  const signing = core.preflightSigning(core.loadSigningConfig(projectRoot));
  if (options.check) {
    console.log(`Release preflight passed: v${version}, ${options.arch}`);
    return;
  }
  ensureClean(projectRoot, execute);
  const sourceCommit = execute("Source revision check", "git", ["rev-parse", "--verify", "HEAD"], { cwd: projectRoot });
  const outDir = resolve(projectRoot, options.out ?? join("release", `macos-v${version}-${options.arch}`));
  const session = core.createReleaseSession({ outDir, version, sourceCommit, resume: true });
  try {
    const app = join(session.workDir, `${APP_NAME}.app`);
    const buildDir = join(session.workDir, "build");
    const archive = join(session.assetsDir, `${APP_NAME}-macos-${options.arch}.zip`);

    session.step("build", () => {
      const env = { ...process.env, PATH: `${dirname(wails)}${delimiter}${process.env.PATH ?? ""}` };
      const task = options.arch === "universal" ? "darwin:package:universal" : "darwin:package";
      const vars = options.arch === "universal" ? ["DEV=false"] : [`ARCH=${options.arch}`, "DEV=false"];
      // Wails' package task copies into an existing directory, so build only in
      // this session's empty output instead of reusing the developer's bin App.
      rmSync(buildDir, { recursive: true, force: true });
      mkdirSync(buildDir, { recursive: true });
      execute("Wails production build", wails, ["task", task, ...vars, `BIN_DIR=${buildDir}`], { cwd: projectRoot, env });
      ensureSourceRevision(projectRoot, sourceCommit, execute);
      rmSync(app, { recursive: true, force: true });
      execute("Release App staging", "/usr/bin/ditto", [join(buildDir, `${APP_NAME}.app`), app]);
    }, () => verifyBundle(app, version, options.arch, execute));
    session.step("sign", () => core.signApplication(app, signing, { nested: [] }), () => {
      core.verifyApplication(app, signing);
      return true;
    });
    session.step("notarize", () => core.notarizeAndStaple(app, signing, { stateDir: session.stateDir }), () => {
      core.verifyApplication(app, signing, { notarized: true });
      return true;
    });
    const checkZip = () => verifyZip(archive, app, version, options.arch, signing, core, execute, session.workDir);
    session.step("archive", () => {
      const temporary = `${archive}.partial.zip`;
      rmSync(temporary, { force: true });
      execute("Release ZIP creation", "/usr/bin/ditto", ["-c", "-k", "--sequesterRsrc", "--keepParent", app, temporary]);
      renameSync(temporary, archive);
      checkZip();
    }, checkZip);
    ensureSourceRevision(projectRoot, sourceCommit, execute);
    const ready = session.markReady([archive]);
    console.log(`Ready: ${archive}`);
    console.log("Only the assets listed in release-assets.json may be uploaded after explicit release authorization.");
    return ready;
  } finally {
    session.close();
  }
}

if (process.argv[1] && existsSync(resolve(process.argv[1])) && realpathSync(fileURLToPath(import.meta.url)) === realpathSync(resolve(process.argv[1]))) {
  try { await main(process.argv.slice(2)); }
  catch (error) { console.error(`Release stopped: ${error.message}`); process.exitCode = 1; }
}
