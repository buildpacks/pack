var express = require('express');
var app = express();
const PORT = process.env.PORT || 3000;

app.get('/', function (req, res) {
  res.send('Buildpacks Worked!');
});

app.listen(PORT, function () {
  console.log(`Example app listening on port ${PORT}!`);
});
