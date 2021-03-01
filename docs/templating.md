[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Using templating to deploy resources

The templating uses applier ....

## Dependencies

bindata is installed in the install_dependencies.sh which does `GOSUMDB=off go get -u github.com/jteeuwen/go-bindata/...`
applier available at....

For more info see [README.md](../pkg/applier/README.md)

## Templating the yamls

The yamls are all located in the [resources](../resources) directory. The template mechanism used is standard Go [text/template](https://golang.org/pkg/text/template/)

Example:

```
apiVersion: v1
kind: Namespace
metadata:
  name: {{ .ManagedClusterName }}
```

## How the bindata.go is generated

This uses the project [https://github.com/jteeuwen/go-bindata](https://github.com/jteeuwen/go-bindata) to generate code holding files assets. The [MakeFile](../MakeFile) contains a target `gobindata` which will generate the file [bindata_generated.go](../pkg/bindata/bindata_generated.go) using all files in [resources](../resources) as input.

If you change [resources](../resources) content then you have to run `make gobindata` to update the [bindata_generated.go](../pkg/bindata/bindata_generated.go).

A check is added in the pre-commit hook to make sure the [bindata_generated.go](../pkg/bindata/bindata_generated.go) is up-to-date.
