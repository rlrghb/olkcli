#!/usr/bin/env node
// Stamp versions across the npm wrapper packages and (optionally) publish them.
//
//   node scripts/build-npm.mjs <version> [--publish]
//
// <version> may be "v0.9.2" or "0.9.2" (a leading "v" is stripped). Without
// --publish it only stamps versions (safe for local inspection / dry-run).
// With --publish it `npm publish`es each per-platform package first (so the
// main package's optionalDependencies already resolve), then the main `olk`
// package. The per-platform binaries must already be present in
// npm/olk-<os>-<arch>/bin/ (the release workflow extracts them there).

import { readFileSync, writeFileSync, existsSync, readdirSync } from "node:fs";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const npmDir = path.join(root, "npm");

const rawVersion = process.argv[2] || process.env.VERSION || process.env.GITHUB_REF_NAME;
if (!rawVersion) {
  console.error("usage: build-npm.mjs <version> [--publish]");
  process.exit(2);
}
const version = rawVersion.replace(/^v/, "");
const publish = process.argv.includes("--publish");

const platformPkgs = readdirSync(npmDir).filter((d) => d.startsWith("olk-"));

function stamp(pkgDir, mutate) {
  const p = path.join(npmDir, pkgDir, "package.json");
  const json = JSON.parse(readFileSync(p, "utf8"));
  json.version = version;
  if (mutate) mutate(json);
  writeFileSync(p, JSON.stringify(json, null, 2) + "\n");
}

for (const pkg of platformPkgs) stamp(pkg);
stamp("olk", (json) => {
  for (const dep of Object.keys(json.optionalDependencies || {})) {
    json.optionalDependencies[dep] = version;
  }
});
console.log(`stamped version ${version} across olk + ${platformPkgs.length} platform packages`);

if (!publish) {
  console.log("(stamp-only — pass --publish to npm publish)");
  process.exit(0);
}

function pkgName(pkgDir) {
  return JSON.parse(readFileSync(path.join(npmDir, pkgDir, "package.json"), "utf8")).name;
}

// Idempotent: skip a package@version that already exists on npm, so re-running
// a partially-failed release (e.g. the registry step failed) does not error on
// "cannot publish over previously published version".
function alreadyPublished(name) {
  try {
    return execFileSync("npm", ["view", `${name}@${version}`, "version"], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim() === version;
  } catch {
    return false;
  }
}

function npmPublish(pkgDir) {
  const name = pkgName(pkgDir);
  if (alreadyPublished(name)) {
    console.log(`skip ${name}@${version} (already published)`);
    return;
  }
  console.log(`publishing ${name}@${version} ...`);
  execFileSync("npm", ["publish", "--access", "public"], {
    cwd: path.join(npmDir, pkgDir),
    stdio: "inherit",
  });
}

for (const pkg of platformPkgs) {
  const exe = pkg.includes("win32") ? "olk.exe" : "olk";
  if (!existsSync(path.join(npmDir, pkg, "bin", exe))) {
    console.error(`refusing to publish ${pkg}: missing bin/${exe}`);
    process.exit(1);
  }
  npmPublish(pkg);
}
npmPublish("olk");
console.log("done");
