[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Development Guide

## Prerequisite

- git
- go version v1.12+
- [kind](https://kind.sigs.k8s.io/), used for functional-test
- kubectl, used for functional-test

## Developer quick start
- Run the unit test before building the binary.

```bash
make test
make build
```

- Run controller for local development.

```bash
make run
```

- Run functional-test
[here](functional_test.md)
