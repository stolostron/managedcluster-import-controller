// Copyright (c) 2020 Red Hat, Inc.

// +build e2e

package e2e

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
	"k8s.io/klog"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	libe2egoapis "github.com/open-cluster-management/library-e2e-go/pkg/apis"
	libgooptions "github.com/open-cluster-management/library-e2e-go/pkg/options"
	libgoproviders "github.com/open-cluster-management/library-e2e-go/pkg/providers"
	"github.com/sclevine/agouti"
)

var kubeadminUser string
var kubeadminCredential string
var reportFile string

var registry string
var registryUser string
var registryPassword string

var optionsFile, clusterDeployFile, installConfigFile string
var clusterDeploy libe2egoapis.ClusterDeploy
var installConfig libe2egoapis.InstallConfig
var testOptionsContainer libgooptions.TestOptionsContainer
var testOptions libgooptions.TestOptions

var testUITimeout time.Duration
var testHeadless bool
var testIdentityProvider int

var ownerPrefix string

var hubNamespace string
var pullSecretName string
var installConfigAWS, installConfigGCP, installConfigAzure string
var hiveClusterName, hiveGCPClusterName, hiveAzureClusterName string

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func randString(length int) string {
	return StringWithCharset(length, charset)
}

func init() {
	klog.SetOutput(GinkgoWriter)
	klog.InitFlags(nil)

	flag.StringVar(&kubeadminUser, "kubeadmin-user", "kubeadmin", "Provide the kubeadmin credential for the cluster under test (e.g. -kubeadmin-user=\"xxxxx\").")
	flag.StringVar(&kubeadminCredential, "kubeadmin-credential", "", "Provide the kubeadmin credential for the cluster under test (e.g. -kubeadmin-credential=\"xxxxx-xxxxx-xxxxx-xxxxx\").")
	flag.StringVar(&reportFile, "report-file", "results.xml", "Provide the path to where the junit results will be printed.")

	flag.StringVar(&optionsFile, "options", "", "Location of an \"options.yaml\" file to provide input for various tests")

}

func TestOpenClusterManagementE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	junitReporter := reporters.NewJUnitReporter(reportFile)
	RunSpecsWithDefaultAndCustomReporters(t, "OpenClusterManagementE2E Suite", []Reporter{junitReporter})
}

var agoutiDriver *agouti.WebDriver

var _ = BeforeSuite(func() {

	initVars()

	// Choose a WebDriver:
	//agoutiDriver = agouti.PhantomJS()
	// agoutiDriver = agouti.Selenium()
	agoutiDriver = agouti.ChromeDriver()

	Expect(agoutiDriver.Start()).To(Succeed())
})

var _ = AfterSuite(func() {
	//Expect(agoutiDriver.Stop()).To(Succeed())
})

func initVars() {

	// default ginkgo test timeout 30s
	// increased from original 10s
	testUITimeout = time.Second * 30

	if optionsFile == "" {
		optionsFile = os.Getenv("OPTIONS")
		if optionsFile == "" {
			optionsFile = "resources/options.yaml"
		}
	}

	klog.V(1).Infof("options filename=%s", optionsFile)

	data, err := ioutil.ReadFile(optionsFile)
	if err != nil {
		klog.Errorf("--options error: %v", err)
	}
	Expect(err).NotTo(HaveOccurred())

	fmt.Printf("file preview: %s \n", string(optionsFile))

	err = yaml.Unmarshal([]byte(data), &testOptionsContainer)
	if err != nil {
		klog.Errorf("--options error: %v", err)
	}

	testOptions = testOptionsContainer.Options

	// clusterdeploy.yaml is optional
	var clusterDeployFile = "resources/clusterdeploy.yaml"
	cd, err := ioutil.ReadFile(clusterDeployFile)
	if err != nil {
		klog.Warningf("warning: %v", err)
	}
	err = yaml.Unmarshal([]byte(cd), &clusterDeploy)
	if err != nil {
		klog.Errorf("clusterdeploy file error: %v", err)
	}

	// install-config.yaml is optional
	var installConfigFile = "resources/install-config.yaml"
	ic, err := ioutil.ReadFile(installConfigFile)
	if err != nil {
		klog.Warningf("warning: %v", err)
	}
	err = yaml.Unmarshal([]byte(ic), &installConfig)
	if err != nil {
		klog.Errorf("installconfig file error: %v", err)
	}

	// default Headless is `true`
	// to disable, set Headless: false
	// in options file
	if testOptions.Headless == "" {
		testHeadless = true
	} else {
		if testOptions.Headless == "false" {
			testHeadless = false
		} else {
			testHeadless = true
		}
	}

	// OwnerPrefix is used to help identify who owns deployed resources
	//    If a value is not supplied, the default is OS environment variable $USER
	if testOptions.OwnerPrefix == "" {
		ownerPrefix = os.Getenv("USER")
		if ownerPrefix == "" {
			ownerPrefix = "ginkgo"
		}
	} else {
		ownerPrefix = testOptions.OwnerPrefix
	}
	klog.V(1).Infof("ownerPrefix=%s", ownerPrefix)

	// identity provider can either be 0 or 1
	// with 0 for kube:admin or `kubeadmin` and
	// 1 for any other use, ie. user defined users
	// default to `kubeadmin` logins, otherwise
	// select the second option
	testIdentityProvider = 0
	if kubeadminUser != "kubeadmin" {
		testIdentityProvider = 1
	}

	// if testOptions.ImageRegistry.Server != "" {
	// 	registry = testOptions.ImageRegistry.Server
	// 	registryUser = testOptions.ImageRegistry.User
	// 	registryPassword = testOptions.ImageRegistry.Password
	// } else {
	// 	klog.Warningf("No `imageRegistry.server` was included in the options.yaml file. Ignoring any tests that require an ImageRegistry.")
	// }

	hiveClusterName = ownerPrefix + "-aws-" + randString(4)
	hiveGCPClusterName = ownerPrefix + "-gcp-" + randString(4)
	hiveAzureClusterName = ownerPrefix + "-azure-" + randString(4)

	var installerConfigAWS = libgoproviders.InstallerConfigAWS{Name: hiveClusterName, BaseDnsDomain: testOptions.Connection.Keys.AWS.BaseDnsDomain, SSHKey: testOptions.Connection.SSHPublicKey, Region: testOptions.Connection.Keys.AWS.Region}
	installConfigAWS, err = libgoproviders.GetInstallConfigAWS(installerConfigAWS)
	Expect(err).To(BeNil())

	var installerConfigGCP = libgoproviders.InstallerConfigGCP{Name: hiveGCPClusterName, BaseDnsDomain: testOptions.Connection.Keys.GCP.BaseDnsDomain, SSHKey: testOptions.Connection.SSHPublicKey, ProjectID: testOptions.Connection.Keys.GCP.ProjectID, Region: testOptions.Connection.Keys.GCP.Region}
	installConfigGCP, err = libgoproviders.GetInstallConfigGCP(installerConfigGCP)
	Expect(err).To(BeNil())

	var installerConfigAzure = libgoproviders.InstallerConfigAzure{Name: hiveAzureClusterName, BaseDnsDomain: testOptions.Connection.Keys.Azure.BaseDnsDomain, SSHKey: testOptions.Connection.SSHPublicKey, BaseDomainRGN: testOptions.Connection.Keys.Azure.BaseDomainRGN, Region: testOptions.Connection.Keys.Azure.Region}
	installConfigAzure, err = libgoproviders.GetInstallConfigAzure(installerConfigAzure)
	Expect(err).To(BeNil())

}
