#!/usr/bin/env node
// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

"use strict";

const crypto = require("crypto");
const fs = require("fs");
const https = require("https");
const os = require("os");
const path = require("path");
const { spawnSync } = require("child_process");

const REPO = "volcengine/VolcEngineRTC_CLI";
const NAME = "vertc";
const MAX_REDIRECTS = 5;

function target(platform = process.platform, arch = process.arch) {
  const platforms = { darwin: "darwin", linux: "linux", win32: "windows" };
  const arches = { x64: "amd64", arm64: "arm64" };
  if (!platforms[platform] || !arches[arch]) {
    throw new Error(`unsupported platform: ${platform}/${arch}`);
  }
  return { os: platforms[platform], arch: arches[arch] };
}

function archiveName(version, platform = process.platform, arch = process.arch) {
  const t = target(platform, arch);
  const ext = platform === "win32" ? "zip" : "tar.gz";
  return `${NAME}_${version}_${t.os}_${t.arch}.${ext}`;
}

function registryHost() {
  try {
    return new URL(process.env.npm_config_registry || "https://registry.npmjs.org").hostname;
  } catch (_) {
    return "registry.npmjs.org";
  }
}

function isAllowedURL(raw) {
  let url;
  try {
    url = new URL(raw);
  } catch (_) {
    return false;
  }
  if (url.protocol !== "https:") return false;
  const allowed = new Set([
    "github.com",
    "objects.githubusercontent.com",
    "release-assets.githubusercontent.com",
    "registry.npmjs.org",
    "registry.npmmirror.com",
    registryHost(),
  ]);
  return allowed.has(url.hostname);
}

function download(raw, redirects = 0) {
  if (!isAllowedURL(raw)) return Promise.reject(new Error(`refusing unapproved download host: ${raw}`));
  if (redirects > MAX_REDIRECTS) return Promise.reject(new Error("too many download redirects"));
  return new Promise((resolve, reject) => {
	const request = https.get(raw, { headers: { "User-Agent": "@volcengine/rtc-cli installer" } }, (response) => {
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        response.resume();
        const next = new URL(response.headers.location, raw).toString();
        download(next, redirects + 1).then(resolve, reject);
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        reject(new Error(`download failed with HTTP ${response.statusCode}`));
        return;
      }
      const chunks = [];
      response.on("data", (chunk) => chunks.push(chunk));
      response.on("end", () => resolve(Buffer.concat(chunks)));
      response.on("error", reject);
	});
	request.setTimeout(30000, () => request.destroy(new Error("download timed out")));
	request.on("error", reject);
  });
}

function expectedChecksum(text, archive) {
  for (const line of text.split(/\r?\n/)) {
    const match = line.trim().match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (match && match[2] === archive) return match[1].toLowerCase();
  }
  throw new Error(`checksum not found for ${archive}`);
}

function verify(buffer, expected) {
  const actual = crypto.createHash("sha256").update(buffer).digest("hex");
  if (actual !== expected.toLowerCase()) throw new Error(`checksum mismatch: expected ${expected}, got ${actual}`);
}

function readBundledArchive(packageRoot, archive) {
  const bundled = path.join(packageRoot, "artifacts", archive);
  return fs.existsSync(bundled) ? fs.readFileSync(bundled) : null;
}

function verifyInstalledBinary(destination, version, spawn = spawnSync) {
  const result = spawn(destination, ["--version"], { encoding: "utf8" });
  if (result.error || result.status !== 0) {
    throw result.error || new Error(`installed binary exited ${result.status}: ${(result.stderr || "").trim()}`);
  }
  const fields = String(result.stdout || "").trim().split(/\s+/);
  if (fields.length < 2 || fields[1].replace(/^v/, "") !== version.replace(/^v/, "")) {
    throw new Error(`installed binary version mismatch: expected ${version}, got ${fields[1] || "unknown"}`);
  }
}

function replaceBinary(source, destination, version, platform = process.platform, spawn = spawnSync) {
  const backup = `${destination}.old`;
  const windows = platform === "win32";
  if (windows && fs.existsSync(backup)) {
    if (fs.existsSync(destination)) fs.rmSync(backup, { force: true });
    else fs.renameSync(backup, destination);
  }
  if (windows && fs.existsSync(destination)) fs.renameSync(destination, backup);
  try {
    fs.copyFileSync(source, destination);
    if (!windows) fs.chmodSync(destination, 0o755);
    verifyInstalledBinary(destination, version, spawn);
  } catch (error) {
    if (windows && fs.existsSync(backup)) {
      fs.rmSync(destination, { force: true });
      fs.renameSync(backup, destination);
    }
    throw error;
  }
}

function urls(version, archive) {
  const registry = (process.env.npm_config_registry || "https://registry.npmjs.org").replace(/\/$/, "");
  return [
    `https://github.com/${REPO}/releases/download/v${version}/${archive}`,
    `https://registry.npmmirror.com/-/binary/${NAME}/v${version}/${archive}`,
    `${registry}/-/binary/${NAME}/v${version}/${archive}`,
  ].filter((value, index, all) => all.indexOf(value) === index);
}

function extract(archivePath, destination, platform = process.platform) {
  const args = platform === "win32"
    ? ["-NoProfile", "-Command", `Expand-Archive -LiteralPath '${archivePath.replace(/'/g, "''")}' -DestinationPath '${destination.replace(/'/g, "''")}' -Force`]
    : ["-xzf", archivePath, "-C", destination];
  const command = platform === "win32" ? "powershell.exe" : "tar";
  const result = spawnSync(command, args, { stdio: "inherit" });
  if (result.error || result.status !== 0) throw result.error || new Error(`${command} exited ${result.status}`);
}

async function install() {
  if (process.env.VERTC_SKIP_POSTINSTALL) return;
  const pkg = require(path.join(__dirname, "..", "package.json"));
  const version = pkg.version;
  const archive = archiveName(version);
  const checksumPath = path.join(__dirname, "..", "checksums.txt");
  if (!fs.existsSync(checksumPath)) throw new Error("checksums.txt is missing from the npm package");
  const checksum = expectedChecksum(fs.readFileSync(checksumPath, "utf8"), archive);
  let body = readBundledArchive(path.join(__dirname, ".."), archive);
  const failures = [];
  if (!body) {
    for (const url of urls(version, archive)) {
      try {
        body = await download(url);
        break;
      } catch (error) {
        failures.push(`${url}: ${error.message}`);
      }
    }
  }
  if (!body) throw new Error(`all binary downloads failed:\n${failures.join("\n")}`);
  verify(body, checksum);
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), "vertc-install-"));
  try {
    const archivePath = path.join(temp, archive);
    fs.writeFileSync(archivePath, body, { mode: 0o600 });
    const unpacked = path.join(temp, "unpacked");
    fs.mkdirSync(unpacked);
    extract(archivePath, unpacked);
    const executable = process.platform === "win32" ? "vertc.exe" : "vertc";
    const source = path.join(unpacked, executable);
    if (!fs.existsSync(source)) throw new Error(`${executable} missing from release archive`);
    const binDir = path.join(__dirname, "bin");
    fs.mkdirSync(binDir, { recursive: true });
    const destination = path.join(binDir, executable);
    replaceBinary(source, destination, version);
  } finally {
    fs.rmSync(temp, { recursive: true, force: true });
  }
}

if (require.main === module) {
  install().catch((error) => {
    console.error(`@volcengine/rtc-cli install failed: ${error.message}`);
    process.exit(1);
  });
}

module.exports = { archiveName, expectedChecksum, isAllowedURL, readBundledArchive, replaceBinary, target, urls, verify, verifyInstalledBinary };
