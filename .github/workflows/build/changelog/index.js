let fs = require('fs');

const libraryLabel = 'lib';

const typeLabelPrefix = 'type/';
const typeLabelsMap = {
  "Features": typeLabelPrefix + "enhancement",
  "Fixes": typeLabelPrefix + "bug",
};

// Map of annotations to be added to issue per label.
const annotationLabelsMap = {
  "experimental": "experimental",
  "breaking": "breaking-change",
};

module.exports = async ({core, github, repository, version}) => {
  const milestone = version;

  console.log("looking up PRs for milestone", milestone, "in repo", repository);

  return await github.paginate("GET /search/issues", {
    q: `repo:${repository} is:pr state:closed milestone:${milestone}`,
  }).then((items) => {

    let cliIssues = [];
    let libraryIssues = [];

    items.forEach(issue => {
      if (issue.labels.filter(label => label.name === libraryLabel).length === 0) {
        cliIssues.push(issue);
      } else {
        libraryIssues.push(issue);
      }
    });

    // group issues by type label
    return {"cliIssues": cliIssues, "libIssues": libraryIssues};
  }).then(({cliIssues, libIssues}) => {
    console.log("CLI issues:", cliIssues.length);
    console.log("Library issues:", libIssues.length);
    console.log("Note: some issues may not be presented (eg. type/chore)");

    let groupedCliIssues = groupByType(cliIssues);
    let groupedLibIssues = groupByType(libIssues);
    let output = "";

    // issues
    for (let key in typeLabelsMap) {
      let issues = (groupedCliIssues[typeLabelsMap[key]] || []);
      if (issues.length > 0) {
        output += `### ${key}\n\n`;
        issues.forEach(issue => {
          output += createIssueEntry(issue);
        });
        output += "\n";
      }
    }

    // library issues
    if (Object.keys(groupedLibIssues).length > 0) {
      output += "### Library\n\n";
      output += "<details><summary>Changes that only affect `pack` as a library usage...</summary><p>\n\n";
      for (let key in typeLabelsMap) {
        let issues = (groupedLibIssues[typeLabelsMap[key]] || []);
        if (issues.length > 0) {
          output += `#### ${key}\n\n`;
          issues.forEach(issue => {
            output += createIssueEntry(issue);
          });
          output += "\n";
        }
      }
      output += "</p></details>";
    }

    output = output.trim();

    fs.writeFile("changelog.md", output, function (err) {
      if (err) {
        console.log(err);
      } else {
        console.log("The file was saved!");
      }
    });

    console.log(`CHANGELOG:\n${output}`);

    core.setOutput('contents', output);
    core.setOutput('file', 'changelog.md');
  });
};

function createIssueEntry(issue) {
  let annotations = [];

  issue.labels.forEach(label => {
    for (const annotation in annotationLabelsMap) {
      if (annotationLabelsMap[annotation] === label.name) {
        annotations.push(annotation);
      }
    }
  });

  let line = `* ${issue.title}`;
  if (annotations.length !== 0) {
    line += ` [${annotations.join(", ")}]`;
  }
  line += ` (#${issue.number} by @${issue.user.login})\n`;

  return line;
}

function groupByType(issues) {
  return issues.reduce((groupedMap, issue) => {
    let typeLabels = issue.labels.filter(label => {
      return label.name.startsWith(typeLabelPrefix);
    }).map(label => label.name);

    if (typeLabels.length > 1) {
      console.warn("issue", issue.number, "has more than one label types: ", typeLabels);
    } else if (typeLabels.length === 0) {
      console.warn("issue", issue.number, "doesn't have a 'type/' label.");
    } else {
      let key = typeLabels[0];
      (groupedMap[key] = groupedMap[key] || []).push(issue);
    }

    return groupedMap;
  }, {})
}