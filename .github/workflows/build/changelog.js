let fs = require('fs');

module.exports = async ({core, github}) => {
  const milestone = process.env.PACK_VERSION;
  const repository = process.env.GITHUB_REPOSITORY;
  console.log("looking up PRs for milestone", milestone, "in repo", repository);

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

  return await github.paginate("GET /search/issues", {
    q: `repo:${repository} is:pr state:closed milestone:${milestone}`,
  }).then((items) => {
    // group issues by type label
    return items.reduce((groupedMap, issue) => {
      let typeLabels = issue.labels.filter(label => {
        return label.name.startsWith(typeLabelPrefix);
      }).map(label => label.name);

      if (typeLabels.length > 1) {
        console.log("issue", issue.number, "has more than one label types: ", typeLabels);
      } else if (typeLabels.length === 0) {
        console.log("issue", issue.number, "doesn't have a 'type/' label.");
      } else {
        let key = typeLabels[0];
        (groupedMap[key] = groupedMap[key] || []).push(issue);
      }

      return groupedMap;
    }, {});
  }).then(groupedIssues => {
    let output = "";

    for (let key in typeLabelsMap) {
      output += `## ${key}\n\n`;
      (groupedIssues[typeLabelsMap[key]] || []).forEach(issue => {
        let annotations = [];
        issue.labels.forEach(label => {
          for (const annotation in annotationLabelsMap) {
            if (annotationLabelsMap[annotation] === label.name) {
              annotations.push(annotation);
            }
          }
        });

        if (annotations.length !== 0) {
          output += `* ${issue.title} [${annotations.join(", ")}] (#${issue.number})\n`;
        } else {
          output += `* ${issue.title} (#${issue.number})\n`;
        }
      });
      output += `\n`;
    }

    output = output.trim();

    fs.writeFile("changelog.md", output, function (err) {
      if (err) {
        console.log(err);
      } else {
        console.log("The file was saved!");
      }
    });

    core.setOutput('contents', output);
    core.setOutput('file', 'changelog.md');
  });
};

