// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	/* #nosec */
	kubeConfigFileBasic = "../../../test/unit/tmp/envtest/kubeconfig/basic"
	kubeConfigFileT     = "../../../test/unit/tmp/envtest/kubeconfig/token"
	kubeConfigFileCerts = "../../../test/unit/tmp/envtest/kubeconfig/certs"
)

var envTests map[string]*envtest.Environment

func TestMain(m *testing.M) {
	exitCode := m.Run()
	tearDownEnvTests()
	os.Exit(exitCode)
}

func setupEnvTest(t *testing.T) (envTest *envtest.Environment,
	kubeConfigBasic string,
	kubeConfigToken string,
	kubeConfigCerts string) {
	return setupEnvTestByName(t.Name())
}

func setupEnvTestByName(name string) (envTest *envtest.Environment,
	kubeConfigBasic string,
	kubeConfigToken string,
	kubeConfigCerts string) {
	kubeConfigNameExtension := "-" + name + ".yaml"
	if envTests == nil {
		envTests = make(map[string]*envtest.Environment)
	}
	if e, ok := envTests[name]; ok {
		return e,
			filepath.Clean(kubeConfigFileBasic + kubeConfigNameExtension),
			filepath.Clean(kubeConfigFileT + kubeConfigNameExtension),
			filepath.Clean(kubeConfigFileCerts + kubeConfigNameExtension)
	}
	//Create an envTest
	envTest = &envtest.Environment{}
	envTests[name] = envTest
	envConfig, err := envTest.Start()
	if err != nil {
		panic(err)
	}
	serverTest := envTest.ControlPlane.APIURL().String()
	//Create basic kubeConfig
	kubeConfigBasic = kubeConfigFileBasic + kubeConfigNameExtension
	apiConfigBasic := kubeconfig.CreateBasic(serverTest,
		"test",
		envConfig.Username,
		envConfig.CAData)
	err = clientcmd.WriteToFile(*apiConfigBasic, kubeConfigBasic)
	if err != nil {
		panic(err)
	}

	//Create basic kubeConfig
	kubeConfigToken = kubeConfigFileT + kubeConfigNameExtension
	apiConfigToken := kubeconfig.CreateWithToken(serverTest,
		"test",
		envConfig.Username,
		envConfig.CAData,
		envConfig.BearerToken)
	err = clientcmd.WriteToFile(*apiConfigToken, kubeConfigToken)
	if err != nil {
		panic(err)
	}

	//Create basic kubeConfig
	kubeConfigCerts = kubeConfigFileCerts + kubeConfigNameExtension
	apiConfigCerts := kubeconfig.CreateWithCerts(serverTest,
		"test",
		envConfig.Username,
		envConfig.CAData,
		envConfig.KeyData,
		envConfig.CertData)
	err = clientcmd.WriteToFile(*apiConfigCerts, kubeConfigCerts)
	if err != nil {
		panic(err)
	}
	return envTest, kubeConfigBasic, kubeConfigToken, kubeConfigCerts

}

func tearDownEnvTests() {
	for name := range envTests {
		tearDownByName(name)
	}
}

func tearDownByName(name string) {
	if e, ok := envTests[name]; ok {
		e.Stop()
	}
}
