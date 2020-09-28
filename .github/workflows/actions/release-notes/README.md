## Changelog

A simple script that generates the changelog for pack based on a pack version (aka milestone).

### Usage

> **NOTICE:** This is not an action _yet_. Mostly due to the complexities of manually testing it via `act` because of: 

#### Github Action

```yaml
# TODO: UPDATE
  - name: Generate changelog
```

#### Local

To run/test locally:

```shell script
# install deps
npm install

# set required info
export GITHUB_TOKEN="<GITHUB_PAT_TOKEN>"

# run locally
npm run local -- <milestone> <config-path>
```

Notice that a file `changelog.md` is created as well for further inspection.


### Updating

This action is packaged for distribution without vendoring `npm_modules` with use of [ncc](https://github.com/vercel/ncc).

When making changes to the action, compile it and commit the changes.

```shell script
npm build
```