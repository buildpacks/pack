const path = require('path');
const scriptPath = path.resolve('index.js');
const {Octokit} = require("@octokit/rest");

const core = require('@actions/core');
const github = new Octokit({auth: mustGetEnvVar('GITHUB_TOKEN')});
const context = {
  repository: "buildpacks/pack"
};

require(scriptPath)({
  core,
  github,
  context,
  version: mustGetEnvVar('PACK_VERSION'),
});


function mustGetEnvVar(envVar) {
  let value = process.env[envVar];
  if (!value) {
    console.error(`'${envVar}' env var must be set.`);
    process.exit(1);
  }
  return value;
}