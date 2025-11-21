#!/usr/bin/env node
// @ts-check

/**
 * Synchronizes the Google OAuth Web Client ID across the demo UI config files
 * and the TAuth environment files so the header, auth flow, and backend all
 * share the same value. Usage:
 *
 *    node demo/scripts/set-google-client-id.js <your-client-id>
 *
 * or set GOOGLE_CLIENT_ID in the environment and omit the argument.
 */

const fs = require('node:fs');
const path = require('node:path');

const projectRoot = path.resolve(path.join(path.dirname(__filename), '..'));
const normalizedId = (process.argv[2] || process.env.GOOGLE_CLIENT_ID || '').trim();

if (!normalizedId) {
  console.error('Usage: node demo/scripts/set-google-client-id.js <google-client-id>');
  process.exit(1);
}

const configPath = path.join(projectRoot, 'config.js');
if (!fs.existsSync(configPath)) {
  console.error(`Cannot find ${configPath}; run the script from the repo root.`);
  process.exit(1);
}

const configSource = fs.readFileSync(configPath, 'utf8');
const idMatch = configSource.match(/googleClientId:\s*'([^']+)'/);
if (!idMatch) {
  console.error('Failed to locate googleClientId in demo/config.js.');
  process.exit(1);
}

const currentId = idMatch[1];
const shouldUpdateLiterals = currentId !== normalizedId;

const escapeRegex = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const literalPattern = new RegExp(escapeRegex(currentId), 'g');

const filesToUpdate = [
  path.join(projectRoot, 'config.js'),
  path.join(projectRoot, 'ui', 'config.js'),
  path.join(projectRoot, 'ui', 'config-apply.js'),
  path.join(projectRoot, 'ui', 'app.js'),
  path.join(projectRoot, 'ui', 'index.html'),
];

const envFiles = [
  path.join(projectRoot, '.env.tauth'),
  path.join(projectRoot, '.env.tauth.example'),
];

if (shouldUpdateLiterals) {
  filesToUpdate.forEach((filePath) => {
    if (!fs.existsSync(filePath)) {
      return;
    }
    const original = fs.readFileSync(filePath, 'utf8');
    const updated = original.replace(literalPattern, normalizedId);
    if (original !== updated) {
      fs.writeFileSync(filePath, updated);
      console.log(`Updated ${path.relative(projectRoot, filePath)}`);
    }
  });
} else {
  console.log(`googleClientId already set to ${normalizedId}; syncing TAuth env files only.`);
}

envFiles.forEach((filePath) => {
  if (!fs.existsSync(filePath)) {
    return;
  }
  const original = fs.readFileSync(filePath, 'utf8');
  const line = `APP_GOOGLE_WEB_CLIENT_ID=${normalizedId}`;
  const updated = /APP_GOOGLE_WEB_CLIENT_ID=/.test(original)
    ? original.replace(/APP_GOOGLE_WEB_CLIENT_ID=.*/, line)
    : `${original.trim()}\n${line}\n`;
  if (original !== updated) {
    fs.writeFileSync(filePath, updated);
    console.log(`Updated ${path.relative(projectRoot, filePath)}`);
  }
});

console.log('Google client ID synchronized. Restart TAuth and reload the UI to apply the change.');
