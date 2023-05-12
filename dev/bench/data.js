window.BENCHMARK_DATA = {
  "lastUpdate": 1683864750426,
  "repoUrl": "https://github.com/buildpacks/pack",
  "entries": {
    "Go Benchmark": [
      {
        "commit": {
          "author": {
            "email": "freilich.david@gmail.com",
            "name": "David Freilich",
            "username": "dfreilich"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "e325cc5a659468cfbb4c9dab57b6fe5974db4a88",
          "message": "Merge pull request #1745 from dmikusa/paketo-jammy\n\nUpdate Paketo stack & builder references to Jammy\r\nSigned-off-by: David Freilich <freilich.david@gmail.com>",
          "timestamp": "2023-05-04T14:45:29+03:00",
          "tree_id": "191edad4ea686305d17ce5d72609e2c6b2e69661",
          "url": "https://github.com/buildpacks/pack/commit/e325cc5a659468cfbb4c9dab57b6fe5974db4a88"
        },
        "date": 1683200816831,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkBuild/with_Untrusted_Builder",
            "value": 4836875142,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Trusted_Builder",
            "value": 1345662281,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Addtional_Buildpack",
            "value": 27957377533,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "freilich.david@gmail.com",
            "name": "David Freilich",
            "username": "dfreilich"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "639d2a843831d83317093f36273928ae60ffefb2",
          "message": "Merge pull request #1741 from inspirit941/fix-1709\n\nExtract internal/cache package to public\r\nSigned-off-by: David Freilich <freilich.david@gmail.com>",
          "timestamp": "2023-05-04T15:33:50+03:00",
          "tree_id": "28db4d94a0cb91165d3bfe0b087aa58fcb5ac61e",
          "url": "https://github.com/buildpacks/pack/commit/639d2a843831d83317093f36273928ae60ffefb2"
        },
        "date": 1683203763664,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkBuild/with_Untrusted_Builder",
            "value": 8745871142,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Trusted_Builder",
            "value": 2673230163,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Addtional_Buildpack",
            "value": 41734972476,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "freilich.david@gmail.com",
            "name": "David Freilich",
            "username": "dfreilich"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "a551896ca450f102eaebaf5748936ef26051142d",
          "message": "Merge pull request #1739 from buildpacks/dependabot/go_modules/github.com/docker/docker-23.0.5incompatible\n\nbuild(deps): bump github.com/docker/docker from 23.0.4+incompatible to 23.0.5+incompatible\r\nSigned-off-by: David Freilich <freilich.david@gmail.com>",
          "timestamp": "2023-05-04T19:14:36+03:00",
          "tree_id": "f248f20c4174d7351a703a38b7401b8f8631a356",
          "url": "https://github.com/buildpacks/pack/commit/a551896ca450f102eaebaf5748936ef26051142d"
        },
        "date": 1683217005089,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkBuild/with_Untrusted_Builder",
            "value": 6237789315,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Trusted_Builder",
            "value": 1818207036,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Addtional_Buildpack",
            "value": 29640697841,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "freilich.david@gmail.com",
            "name": "David Freilich",
            "username": "dfreilich"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "5667b07d01c961ff30a735ca1d6222e4736b2696",
          "message": "Merge pull request #1738 from buildpacks/dependabot/go_modules/github.com/docker/cli-23.0.5incompatible\n\nbuild(deps): bump github.com/docker/cli from 23.0.4+incompatible to 23.0.5+incompatible\r\nSigned-off-by: David Freilich <freilich.david@gmail.com>",
          "timestamp": "2023-05-04T21:35:29+03:00",
          "tree_id": "16ebe3abf5e90a77331a8278a9061b1bddc5e3fb",
          "url": "https://github.com/buildpacks/pack/commit/5667b07d01c961ff30a735ca1d6222e4736b2696"
        },
        "date": 1683225462511,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkBuild/with_Untrusted_Builder",
            "value": 6555696356,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Trusted_Builder",
            "value": 1858477793,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Addtional_Buildpack",
            "value": 33585789339,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "jpkutner@gmail.com",
            "name": "Joe Kutner",
            "username": "jkutner"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "8bd4c4902b328186f9817a52b8e0a56e9cd5b5d4",
          "message": "Merge pull request #1749 from buildpacks/jkutner/deps\n\nVarious dependency updates",
          "timestamp": "2023-05-05T09:46:39-05:00",
          "tree_id": "4827be80addd9da3cc7b8429a2fcbda4f8439aab",
          "url": "https://github.com/buildpacks/pack/commit/8bd4c4902b328186f9817a52b8e0a56e9cd5b5d4"
        },
        "date": 1683298130454,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkBuild/with_Untrusted_Builder",
            "value": 6273561734,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Trusted_Builder",
            "value": 1865838020,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Addtional_Buildpack",
            "value": 32321714674,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "jpkutner@gmail.com",
            "name": "Joe Kutner",
            "username": "jkutner"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "881dd55d59f74928a4754e0b21abd58793a54db1",
          "message": "Merge pull request #1735 from quantumsheep/patch-1\n\nWait for non-running state to prevent concurrency",
          "timestamp": "2023-05-11T23:10:20-05:00",
          "tree_id": "11d3980382a1e99f084608d2b40ed749baf5f543",
          "url": "https://github.com/buildpacks/pack/commit/881dd55d59f74928a4754e0b21abd58793a54db1"
        },
        "date": 1683864749465,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkBuild/with_Untrusted_Builder",
            "value": 6283298697,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Trusted_Builder",
            "value": 1748158155,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          },
          {
            "name": "BenchmarkBuild/with_Addtional_Buildpack",
            "value": 31700057682,
            "unit": "ns/op",
            "extra": "1 times\n2 procs"
          }
        ]
      }
    ]
  }
}