[![License][license-badge]][license-link]
[![Actions][github-actions-badge]][github-actions-link]
[![Releases][github-release-badge]][github-release-link]

# Drone Github Comment Plugin

💬 Drone plugin which takes the output of a step and comments on a Github pull request

![pull request comment](https://user-images.githubusercontent.com/307183/162646831-7766bbb9-f815-4f32-87a3-a75796401411.png)

## Motivations

This DroneCI plugin enables you to take the output from a target step and comment that output as a GitHub pull request comment.
This enables you to write single purpose steps that lint files, run tests, etc, that don't require any knowledge of the underlying pull request or the GitHub API.

## Usage

This plugin can be added to your .drone.yml as a new step within an existing pipeline.
Secrets for a `DRONE_TOKEN` as well as a `GITHUB_TOKEN` must be configured.

```yaml
steps:
- name: drone-github-comment
  image: ghcr.io/joshdk/drone-github-comment:v0.1.0
  environment:
    DRONE_TOKEN:
      from_secret: DRONE_TOKEN
    GITHUB_TOKEN:
      from_secret: GITHUB_TOKEN
```

You then need to configure a target `step` name that refers to existing pipeline step.
You can either specify both the stage/step name, like `build-pull-request/lint-code` in the example below, or just the step name for convenience.

```yaml
kind: pipeline
name: build-pull-request
steps:
- name: lint-code
  commands:
  - "golangci-lint run ."

- name: drone-github-comment
  image: ghcr.io/joshdk/drone-github-comment:v0.1.0
  settings:
    step: build-pull-request/lint-code
    #step: lint-code
```

You must also configure the `depends_on` values, since this plugin must be run after the target step finishes.

```yaml
steps:
- name: drone-github-comment
  image: ghcr.io/joshdk/drone-github-comment:v0.1.0
  depends_on:
  - lint-code
```

You should also configure the `when` values, since this plugin might be run after the target step fails.

```yaml
steps:
- name: drone-github-comment
  image: ghcr.io/joshdk/drone-github-comment:v0.1.0
  when:
    status:
      - failure
```

Comments posted by previous runs are considered out of date, and are automatically deleted on subsequent runs.
To avoid the deletion of out of date comments, you can set `keep` to `true`.

```yaml
steps:
- name: drone-github-comment
  image: ghcr.io/joshdk/drone-github-comment:v0.1.0
  settings:
    keep: true
```

You may want to only post a comment when the target step succeeds or fails.
You can use set `when` to `success`, `failure`, or `always`.
This setting defaults to `always`.
Note that the `when` setting is different to the `when` step property.

```yaml
steps:
- name: drone-github-comment
  image: ghcr.io/joshdk/drone-github-comment:v0.1.0
  settings:
    when: failure
```

Target steps often run a series of shell `commands`, which print each command run using `set -e`.
These lines, which start with a `+` symbol, are automatically ignored.
To include such lines, you can set `verbatim` to `true`.

```yaml
steps:
- name: drone-github-comment
  image: ghcr.io/joshdk/drone-github-comment:v0.1.0
  settings:
    verbatim: true
```

## License

This code is distributed under the [MIT License][license-link], see [LICENSE.txt][license-file] for more information.

[github-actions-badge]:  https://github.com/joshdk/drone-github-comment/workflows/Build/badge.svg
[github-actions-link]:   https://github.com/joshdk/drone-github-comment/actions
[github-release-badge]:  https://img.shields.io/github/release/joshdk/drone-github-comment/all.svg
[github-release-link]:   https://github.com/joshdk/drone-github-comment/releases
[license-badge]:         https://img.shields.io/badge/license-MIT-green.svg
[license-file]:          https://github.com/joshdk/drone-github-comment/blob/master/LICENSE.txt
[license-link]:          https://opensource.org/licenses/MIT
