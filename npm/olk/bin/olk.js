#!/usr/bin/env node
"use strict";

// Launcher for the `olk` CLI distributed via npm. The actual binary ships in a
// per-platform optional dependency (olk-<platform>-<arch>); npm installs only
// the one matching this host (via the package's "os"/"cpu" fields). We resolve
// that binary and exec it, forwarding args, stdio, and exit code.

const { execFileSync } = require("child_process");

function binaryPath() {
  const pkg = `olk-${process.platform}-${process.arch}`;
  const exe = process.platform === "win32" ? "olk.exe" : "olk";
  try {
    return require.resolve(`${pkg}/bin/${exe}`);
  } catch (_) {
    throw new Error(
      `olk: no prebuilt binary for ${process.platform}-${process.arch}. ` +
        `The optional dependency "${pkg}" is missing — reinstall without ` +
        `--no-optional, or build from source (https://github.com/rlrghb/olkcli).`
    );
  }
}

try {
  execFileSync(binaryPath(), process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  if (typeof err.status === "number") {
    process.exit(err.status);
  }
  if (err.message) {
    process.stderr.write(err.message + "\n");
  }
  process.exit(1);
}
