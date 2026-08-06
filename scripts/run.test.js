"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const { healthy, recover } = require("./run.js");

test("healthy requires a launchable binary with the expected version", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "vertc-run-test-"));
  const binary = path.join(dir, "vertc.exe");
  fs.writeFileSync(binary, "candidate");
  assert.equal(healthy(binary, "2.0.0", () => ({ status: 0, stdout: "vertc 2.0.0\n" })), true);
  assert.equal(healthy(binary, "2.0.0", () => ({ status: 0, stdout: "vertc 1.0.0\n" })), false);
  assert.equal(healthy(binary, "2.0.0", () => ({ error: new Error("bad image") })), false);
  fs.rmSync(dir, { recursive: true, force: true });
});

test("recover restores backup for a corrupt Windows candidate", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "vertc-run-test-"));
  const binary = path.join(dir, "vertc.exe");
  const backup = `${binary}.old`;
  fs.writeFileSync(binary, "broken");
  fs.writeFileSync(backup, "old");
  recover(binary, backup, "win32", "2.0.0", () => ({ status: 1 }));
  assert.equal(fs.readFileSync(binary, "utf8"), "old");
  assert.equal(fs.existsSync(backup), false);
  fs.rmSync(dir, { recursive: true, force: true });
});

test("recover preserves a verified Windows candidate and its backup", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "vertc-run-test-"));
  const binary = path.join(dir, "vertc.exe");
  const backup = `${binary}.old`;
  fs.writeFileSync(binary, "new");
  fs.writeFileSync(backup, "old");
  recover(binary, backup, "win32", "2.0.0", () => ({ status: 0, stdout: "vertc 2.0.0\n" }));
  assert.equal(fs.readFileSync(binary, "utf8"), "new");
  assert.equal(fs.readFileSync(backup, "utf8"), "old");
  fs.rmSync(dir, { recursive: true, force: true });
});
