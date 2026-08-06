#!/usr/bin/env node
// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

"use strict";

const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

const executable = process.platform === "win32" ? "vertc.exe" : "vertc";
const binary = path.join(__dirname, "bin", executable);
const stale = `${binary}.old`;
const expectedVersion = require(path.join(__dirname, "..", "package.json")).version;

function healthy(candidate, version = expectedVersion, spawn = spawnSync) {
  if (!fs.existsSync(candidate)) return false;
  const result = spawn(candidate, ["--version"], { encoding: "utf8" });
  if (result.error || result.status !== 0) return false;
  const fields = String(result.stdout || "").trim().split(/\s+/);
  return fields.length >= 2 && fields[1].replace(/^v/, "") === version.replace(/^v/, "");
}

function recover(candidate = binary, backup = stale, platform = process.platform, version = expectedVersion, spawn = spawnSync) {
  if (platform !== "win32" || !fs.existsSync(backup)) return;
  if (healthy(candidate, version, spawn)) return;
  fs.rmSync(candidate, { force: true });
  fs.renameSync(backup, candidate);
}

function main() {
  recover();
  if (!fs.existsSync(binary)) {
    console.error("vertc binary is missing; reinstall with: npm install -g @volcengine/rtc-cli");
    return 1;
  }

  const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
  if (result.error) {
    if (process.platform === "win32" && fs.existsSync(stale)) {
      fs.rmSync(binary, { force: true });
      fs.renameSync(stale, binary);
    }
    console.error(`failed to run vertc: ${result.error.message}`);
    return 1;
  }
  if (process.platform === "win32" && fs.existsSync(stale)) fs.rmSync(stale, { force: true });
  return result.status === null ? 1 : result.status;
}

if (require.main === module) process.exit(main());

module.exports = { healthy, recover };
