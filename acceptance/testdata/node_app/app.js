var fs = require('fs');
var express = require('express');
var app = express();
const PORT = process.env.PORT || 3000;

app.get('/', function (req, res) {
    fs.stat('./', function(err, stat) {
        if (err) {
            res.send('Failed to stat ./');
            return
        }
        fs.writeFile('./test-permission.txt', 'This file is used to test for write permission.', 'utf8', function (err) {
            if (err) {
                res.send('Failed to write ./test-permission.txt!');
                return
            }
            res.send(`Buildpacks Worked! - ${stat.uid}:${stat.gid}`);
        });
    });
});

app.listen(PORT, function () {
    console.log(`Example app listening on port ${PORT}!`);
});
