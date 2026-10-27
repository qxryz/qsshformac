import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { chmodSync, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { main, parseArgs, readReleaseVersion, resolveWails } from "./package-release.mjs";
import { createReleaseSession } from "./lib/macos-signing.mjs";

const VERSION = "0.3.2.1";
const plist = (version = VERSION) => `<plist><dict>
<key>CFBundleVersion</key><string>${version}</string>
<key>CFBundleShortVersionString</key><string>${version}</string>
<key>CFBundleExecutable</key><string>pzssh</string>
<key>CFBundleIdentifier</key><string>com.example.qssh</string>
</dict></plist>`;

function fixture(options = {}) {
  const root = mkdtempSync(join(tmpdir(), "qssh-release-test-"));
  mkdirSync(join(root, "build/darwin"), { recursive: true });
  writeFileSync(join(root, "build/darwin/Info.plist"), plist());
  writeFileSync(join(root, "build/config.yml"), `version: '3'\ninfo:\n  version: "${VERSION}"\n  productIdentifier: "com.example.qssh"\ndev_mode:\n  root_path: .\n`);
  writeFileSync(join(root, "go.mod"), "module example.test/qssh\nrequire github.com/wailsapp/wails/v3 v3.0.0-alpha.74\n");
  const calls = [];
  const core = {
    loadSigningConfig() { calls.push("config"); return {}; },
    preflightSigning() {
      calls.push("preflight");
      if (options.authorizationFails) throw new Error("Notarization authorization failed");
      return {};
    },
    createReleaseSession,
    signApplication(app) { calls.push("sign"); writeFileSync(join(app, "signed.flag"), "signed"); },
    notarizeAndStaple(app) { calls.push("notarize"); writeFileSync(join(app, "notarized.flag"), "notarized"); },
    verifyApplication(app, _signing, flags = {}) {
      assert.ok(existsSync(join(app, "signed.flag")));
      if (flags.notarized) {
        if (options.gatekeeperFails) throw new Error("Application Gatekeeper assessment failed");
        assert.ok(existsSync(join(app, "notarized.flag")));
      }
      return true;
    },
  };
  function files(directory, prefix = "") {
    return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
      const name = prefix ? `${prefix}/${entry.name}` : entry.name;
      return entry.isDirectory() ? files(join(directory, entry.name), name) : [[name, readFileSync(join(directory, entry.name)).toString("base64")]];
    });
  }
  const execute = (_label, command, args) => {
    if (command === "git") {
      if (args[0] === "status") return options.dirty ? " M main.go" : "";
      const changed = (options.headAfterBuild && calls.includes("build"))
        || (options.headAfterNotary && calls.includes("notarize"))
        || (options.headAfterArchive && calls.includes("archive"));
      return (changed ? "b" : "a").repeat(40);
    }
    if (command === "/mock/wails3" && args[0] === "version") return "v3.0.0-alpha.74";
    if (command === "/mock/wails3") {
      calls.push("build");
      const binDir = args.find((value) => value.startsWith("BIN_DIR="))?.slice("BIN_DIR=".length) ?? join(root, "bin");
      const app = join(binDir, "pzssh.app");
      mkdirSync(join(app, "Contents/MacOS"), { recursive: true });
      writeFileSync(join(app, "Contents/Info.plist"), plist());
      writeFileSync(join(app, "Contents/MacOS/pzssh"), "test executable");
      return "";
    }
    if (command === "/usr/bin/lipo") return "arm64";
    assert.equal(command, "/usr/bin/ditto");
    if (args[0] === "-c") {
      calls.push("archive");
      writeFileSync(args.at(-1), JSON.stringify(files(args.at(-2))));
    } else if (args[0] === "-xk") {
      for (const [name, value] of JSON.parse(readFileSync(args[1], "utf8"))) {
        const path = join(args[2], "pzssh.app", name);
        mkdirSync(dirname(path), { recursive: true });
        writeFileSync(path, Buffer.from(value, "base64"));
      }
    } else cpSync(args[0], args[1], { recursive: true });
    return "";
  };
  return { root, calls, context: { projectRoot: root, core, execute, findWails: () => "/mock/wails3" },
    cleanup() { rmSync(root, { recursive: true, force: true }); } };
}

test("release version disagreement is detected before signing", () => {
  const f = fixture();
  try {
    writeFileSync(join(f.root, "build/darwin/Info.plist"), plist("0.3.2.2"));
    assert.throws(() => readReleaseVersion(f.root), /versions differ/);
  } finally { f.cleanup(); }
});

test("missing notarization authorization stops before build or output creation", async () => {
  const f = fixture({ authorizationFails: true });
  try {
    await assert.rejects(main(["--arch", "arm64"], f.context), /authorization failed/);
    assert.deepEqual(f.calls, ["config", "preflight"]);
    assert.equal(existsSync(join(f.root, "release")), false);
  } finally { f.cleanup(); }
});

test("dirty source is rejected while --check remains available", async () => {
  const f = fixture({ dirty: true });
  try {
    await main(["--check"], f.context);
    await assert.rejects(main(["--arch", "arm64"], f.context), /clean source tree/);
    assert.equal(f.calls.includes("build"), false);
  } finally { f.cleanup(); }
});

test("Gatekeeper failure prevents final ZIP and READY publication", async () => {
  const f = fixture({ gatekeeperFails: true });
  try {
    await assert.rejects(main(["--arch", "arm64"], f.context), /Gatekeeper assessment failed/);
    assert.equal(f.calls.includes("archive"), false);
    assert.equal(existsSync(join(f.root, `release/macos-v${VERSION}-arm64/release-assets.json`)), false);
  } finally { f.cleanup(); }
});

test("an interrupted or completed release resumes without rebuilding, resigning, or resubmitting", async () => {
  const f = fixture();
  try {
    const first = await main(["--arch", "arm64"], f.context);
    const second = await main(["--arch", "arm64"], f.context);
    assert.equal(first.status, "READY");
    assert.deepEqual(second, first);
    for (const name of ["build", "sign", "notarize", "archive"]) assert.equal(f.calls.filter((value) => value === name).length, 1);
    const asset = first.assets[0];
    assert.equal(asset.name, "pzssh-macos-arm64.zip");
    assert.equal(first.assets.length, 1);
    assert.deepEqual(readdirSync(join(f.root, `release/macos-v${VERSION}-arm64/assets`)), [asset.name]);
  } finally { f.cleanup(); }
});

test("CLI rejects unsupported targets and missing option values", () => {
  assert.throws(() => parseArgs(["--arch", "linux"]), /--arch must/);
  assert.throws(() => parseArgs(["--out"]), /requires a value/);
  assert.throws(() => parseArgs(["--publish"]), /Unknown argument/);
});


test("a different Wails CLI version is rejected before any signing lookup", async () => {
  const f = fixture();
  try {
    const original = f.context.execute;
    f.context.execute = (label, command, args, options) => command === "/mock/wails3" && args[0] === "version"
      ? "v3.0.0-alpha.740" : original(label, command, args, options);
    await assert.rejects(main(["--check"], f.context), /must match the version/);
    assert.deepEqual(f.calls, []);
  } finally { f.cleanup(); }
});

test("development CI cannot publish unsigned artifacts as an official Release", () => {
  const workflow = readFileSync(new URL("../.github/workflows/build-macos.yml", import.meta.url), "utf8");
  assert.match(workflow, /contents: read/);
  assert.match(workflow, /unsigned-development/);
  assert.doesNotMatch(workflow, /^\s*tags:/m);
  assert.doesNotMatch(workflow, /action-gh-release|gh release (?:create|upload)/);
});


test("real CLI preflight reads a successful Wails version from stderr", () => {
  const f = fixture();
  try {
    const script = join(f.root, "script/package-release.mjs");
    mkdirSync(join(f.root, "script/lib"), { recursive: true });
    writeFileSync(script, readFileSync(new URL("./package-release.mjs", import.meta.url)));
    writeFileSync(join(f.root, "script/lib/macos-signing.mjs"), "export const loadSigningConfig = () => ({}); export const preflightSigning = (value) => value;\n");
    const wails = join(f.root, "wails-version-stderr");
    writeFileSync(wails, "#!/bin/sh\nprintf 'v3.0.0-alpha.74\\n' >&2\n");
    chmodSync(wails, 0o700);
    const output = execFileSync(process.execPath, [script, "--check", "--arch", "arm64"], {
      encoding: "utf8", env: { ...process.env, WAILS_BIN: wails },
    });
    assert.match(output, /Release preflight passed: v0\.3\.2\.1, arm64/);
  } finally { f.cleanup(); }
});


test("production build starts in an empty session BIN_DIR and leaves the developer App untouched", async () => {
  const f = fixture();
  try {
    const oldApp = join(f.root, "bin/pzssh.app");
    mkdirSync(join(oldApp, "Contents/Resources"), { recursive: true });
    mkdirSync(join(oldApp, "Contents/MacOS"), { recursive: true });
    writeFileSync(join(oldApp, "Contents/Resources/removed-resource.txt"), "must remain only in the developer App");
    writeFileSync(join(oldApp, "Contents/MacOS/pzssh"), "possibly running developer executable");
    const outDir = join(f.root, `release/macos-v${VERSION}-arm64`);
    const previous = createReleaseSession({ outDir, version: VERSION, sourceCommit: "a".repeat(40) });
    const failedBuildResource = join(previous.workDir, "build/pzssh.app/Contents/Resources/failed-build.txt");
    mkdirSync(dirname(failedBuildResource), { recursive: true });
    writeFileSync(failedBuildResource, "unfinished old build");
    previous.close();
    await main(["--arch", "arm64"], f.context);
    assert.equal(readFileSync(join(oldApp, "Contents/MacOS/pzssh"), "utf8"), "possibly running developer executable");
    assert.equal(readFileSync(join(oldApp, "Contents/Resources/removed-resource.txt"), "utf8"), "must remain only in the developer App");
    assert.equal(existsSync(failedBuildResource), false);
    assert.equal(existsSync(join(outDir, "work/pzssh.app/Contents/Resources/removed-resource.txt")), false);
  } finally { f.cleanup(); }
});

test("a clean HEAD change during the build stops before signing", async () => {
  const f = fixture({ headAfterBuild: true });
  try {
    await assert.rejects(main(["--arch", "arm64"], f.context), /Source revision changed/);
    assert.equal(f.calls.includes("sign"), false);
    assert.equal(existsSync(join(f.root, `release/macos-v${VERSION}-arm64/release-assets.json`)), false);
  } finally { f.cleanup(); }
});

test("a clean HEAD change during notarization cannot produce READY assets", async () => {
  const f = fixture({ headAfterNotary: true });
  try {
    await assert.rejects(main(["--arch", "arm64"], f.context), /Source revision changed/);
    assert.equal(existsSync(join(f.root, `release/macos-v${VERSION}-arm64/release-assets.json`)), false);
  } finally { f.cleanup(); }
});

test("the final source recheck detects a HEAD change while the archive is created", async () => {
  const f = fixture({ headAfterArchive: true });
  try {
    await assert.rejects(main(["--arch", "arm64"], f.context), /Source revision changed/);
    assert.equal(existsSync(join(f.root, `release/macos-v${VERSION}-arm64/release-assets.json`)), false);
  } finally { f.cleanup(); }
});


test("the real Darwin task graph keeps native and universal output inside the requested BIN_DIR", { skip: process.platform !== "darwin" }, (context) => {
  let wails;
  try { wails = resolveWails(); } catch { context.skip("Wails is unavailable on this machine"); return; }
  const output = join(tmpdir(), "qssh dry-run isolated output");
  const root = new URL("../", import.meta.url);
  for (const task of ["darwin:package", "darwin:package:universal"]) {
    const result = spawnSync(wails, ["task", "-dry", task, "ARCH=amd64", "DEV=false", `BIN_DIR=${output}`], { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
    const response = result.stdout + result.stderr;
    assert.equal(result.status, 0, response);
    const commands = response.split("\n").filter((line) => /go build|mkdir -p|cp |codesign --|lipo -create/.test(line));
    assert.ok(commands.length >= 7);
    for (const command of commands) assert.ok(command.includes(output), `Task escaped BIN_DIR: ${command}`);
    assert.match(response, new RegExp('go build .* -o "' + output.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + '/pzssh'));
    assert.equal(response.includes('"bin/pzssh'), false);
  }
});
