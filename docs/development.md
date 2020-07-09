# Development Guide

## Prerequisite

- git
- go version v1.12+

<!-- OLD IBM stuff
- Linting Tools

    | linting tool | version |
    | ------------ | ------- |
    | [hadolint](https://github.com/hadolint/hadolint#install) | [v1.17.2](https://github.com/hadolint/hadolint/releases/tag/v1.17.2) |
    | [shellcheck](https://github.com/koalaman/shellcheck#installing) | [v0.7.0](https://github.com/koalaman/shellcheck/releases/tag/v0.7.0) |
    | [yamllint](https://github.com/adrienverge/yamllint#installation) | [v1.17.0](https://github.com/adrienverge/yamllint/releases/tag/v1.17.0)
    | [helm client](https://helm.sh/docs/using_helm/#install-helm) | [v2.10.0](https://github.com/helm/helm/releases/tag/v2.10.0) |
    | [golangci-lint](https://github.com/golangci/golangci-lint#install) | [v1.18.0](https://github.com/golangci/golangci-lint/releases/tag/v1.18.0) |
    | [autopep8](https://github.com/hhatto/autopep8#installation) | [v1.4.4](https://github.com/hhatto/autopep8/releases/tag/v1.4.4) |
    | [mdl](https://github.com/markdownlint/markdownlint#installation) | [v0.5.0](https://github.com/markdownlint/markdownlint/releases/tag/v0.5.0) |
    | [awesome_bot](https://github.com/dkhamsing/awesome_bot#installation) | [1.19.1](https://github.com/dkhamsing/awesome_bot/releases/tag/1.19.1) |
    | [sass-lint](https://github.com/sasstools/sass-lint#install) | [v1.13.1](https://github.com/sasstools/sass-lint/releases/tag/v1.13.1) |
    | [tslint](https://github.com/palantir/tslint#installation--usage) | [v5.18.0](https://github.com/palantir/tslint/releases/tag/5.18.0)
    | [prototool](https://github.com/uber/prototool/blob/dev/docs/install.md) | `7df3b95` |
    | [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports) | `3792095` |
-->

## Developer quick start

- Setup Git personal access token
```bash
export GITHUB_TOKEN=
export GITHUB_USER=
```

- Configure git to pull go modules

```bash
git config --global url.git@github.com:open-cluster-management/.insteadOf https://github.com/open-cluster-management/
```

- Configure Go
```
export GOPRIVATE=github.com
```

- Build dependencies
```bash
make deps
```
*NOTE:* You may need to run this twice if it fails the first time

- Build


- Run the unit test before building the image.

```bash
make test
make build
```

- Run controller for local development.

```bash
make run
```

> **Note:** You need to login the image registry before running the command above.

- Deploy controller for local development

```bash
make deploy
```

> **Note:** If you are using a private image registry you need follow the rest of the instruction to create image pull secret 

- Create image pull secret

```bash
export DEFAULT_IMAGE_PULL_SECRET=<image pull secret name>
kubectl create secret generic $IMAGE_PULL_SECRET \
    --from-file=.dockerconfigjson=<path/to/.docker/config.json> \
    --type=kubernetes.io/dockerconfigjson
```

