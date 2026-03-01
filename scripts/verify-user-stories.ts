#!/usr/bin/env bun

import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { extname, join } from "node:path";

type UserStory = {
  category?: string;
  description: string;
  steps: string[];
  passes: boolean;
};

const rootDir = join(import.meta.dir, "..");
const storiesDir = join(rootDir, "docs", "user-stories");

let hasErrors = false;

function fail(message: string) {
  console.error(`  ${message}`);
  hasErrors = true;
}

function ok(message: string) {
  console.log(`  ${message}`);
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}

function validateStoryObject(item: unknown, index: number, fileLabel: string): item is UserStory {
  if (typeof item !== "object" || item === null || Array.isArray(item)) {
    fail(`${fileLabel} item[${index}] must be an object`);
    return false;
  }

  const row = item as Record<string, unknown>;

  if (!("description" in row) || !isNonEmptyString(row.description)) {
    fail(`${fileLabel} item[${index}].description must be a non-empty string`);
    return false;
  }

  if (!("steps" in row) || !Array.isArray(row.steps) || row.steps.length === 0) {
    fail(`${fileLabel} item[${index}].steps must be a non-empty array`);
    return false;
  }

  const invalidStep = row.steps.findIndex((step) => !isNonEmptyString(step));
  if (invalidStep >= 0) {
    fail(`${fileLabel} item[${index}].steps[${invalidStep}] must be a non-empty string`);
    return false;
  }

  if (!("passes" in row) || typeof row.passes !== "boolean") {
    fail(`${fileLabel} item[${index}].passes must be a boolean`);
    return false;
  }

  if ("category" in row && row.category !== undefined && !isNonEmptyString(row.category)) {
    fail(`${fileLabel} item[${index}].category must be a non-empty string when provided`);
    return false;
  }

  return true;
}

function validateFile(filePath: string, label: string) {
  let parsed: unknown;
  try {
    parsed = JSON.parse(readFileSync(filePath, "utf8"));
  } catch (error) {
    fail(`${label} invalid JSON: ${String(error)}`);
    return;
  }

  if (!Array.isArray(parsed) || parsed.length === 0) {
    fail(`${label} must be a non-empty JSON array`);
    return;
  }

  let validCount = 0;
  let passCount = 0;

  parsed.forEach((item, index) => {
    if (validateStoryObject(item, index, label)) {
      validCount += 1;
      if ((item as UserStory).passes) {
        passCount += 1;
      }
    }
  });

  if (validCount === parsed.length) {
    ok(`${label} (${passCount}/${parsed.length} passing)`);
  }
}

function walk(dir: string, prefix = "") {
  const entries = readdirSync(dir);

  entries.forEach((entry) => {
    const fullPath = join(dir, entry);
    const stat = statSync(fullPath);

    if (stat.isDirectory()) {
      console.log(`${prefix}${entry}/`);
      walk(fullPath, `${prefix}  `);
      return;
    }

    if (extname(entry) !== ".json") {
      fail(`${prefix}${entry} is not a .json file`);
      return;
    }

    validateFile(fullPath, `${prefix}${entry}`);
  });
}

console.log("\nVerifying user stories...\n");

if (!existsSync(storiesDir)) {
  console.log("No docs/user-stories directory found\n");
  process.exit(0);
}

walk(storiesDir);

console.log("");

if (hasErrors) {
  console.log("Verification failed\n");
  process.exit(1);
}

console.log("All user stories valid\n");
