module github.com/open-cluster-management/rcm-controller

go 1.13

require (
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/open-cluster-management/api v0.0.0-20200602195039-a516cac2e038
	github.com/open-cluster-management/library-e2e-go v0.0.0-20200620112055-c80fc3c14997
	github.com/open-cluster-management/library-go v0.0.0-20200701182249-c38b9f4da215
	github.com/openshift/api v3.9.1-0.20191112184635-86def77f6f90+incompatible
	github.com/openshift/hive v0.0.0-20200318152403-0c1ea8babb4e
	github.com/operator-framework/operator-sdk v0.18.1
	github.com/sclevine/agouti v3.0.0+incompatible
	github.com/spf13/pflag v1.0.5
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.18.4
	k8s.io/apiextensions-apiserver v0.18.3
	k8s.io/apimachinery v0.18.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.13.0
	sigs.k8s.io/controller-runtime v0.6.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	github.com/mattn/go-sqlite3 => github.com/mattn/go-sqlite3 v1.10.0
	k8s.io/client-go => k8s.io/client-go v0.18.2
)
