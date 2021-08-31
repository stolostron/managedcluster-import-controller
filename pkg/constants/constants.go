// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package constants

const YamlSperator = "\n---\n"

const (
	CreatedViaAnnotation = "open-cluster-management/created-via"
	CreatedViaAI         = "assisted-installer"
	CreatedViaHive       = "hive"
)

/* #nosec */
const AutoImportSecretName string = "auto-import-secret"

/* #nosec */
const (
	ImportSecretNameSuffix         = "import"
	ImportSecretImportYamlKey      = "import.yaml"
	ImportSecretCRDSYamlKey        = "crds.yaml"
	ImportSecretCRDSV1YamlKey      = "crdsv1.yaml"
	ImportSecretCRDSV1beta1YamlKey = "crdsv1beta1.yaml"
)
