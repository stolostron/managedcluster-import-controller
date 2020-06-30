package e2e

// import (
// 	"fmt"

// 	"github.com/ghodss/yaml"

// 	libgoclient "github.com/open-cluster-management/library-go/pkg/client"
// 	"k8s.io/apimachinery/pkg/api/errors"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
// 	"k8s.io/apimachinery/pkg/runtime/schema"
// 	"k8s.io/client-go/dynamic"
// 	"k8s.io/client-go/kubernetes"
// 	"k8s.io/klog"

// 	. "github.com/onsi/ginkgo"
// 	. "github.com/onsi/gomega"
// 	"github.com/open-cluster-management/open-cluster-management-e2e/utils"

// 	// . "github.com/sclevine/agouti/matchers"
// 	appsv1 "k8s.io/api/apps/v1"
// 	corev1 "k8s.io/api/core/v1"
// 	//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	//	"k8s.io/client-go/dynamic"
// 	//	"k8s.io/client-go/kubernetes"
// )

// const (
// 	OCP_VERSION = "quay.io/openshift-release-dev/ocp-release:4.3.12-x86_64"
// )

// var hubClient kubernetes.Interface
// var dynClient dynamic.Interface

// var _ = Describe("Given a hub API", func() {

// 	BeforeEach(func() {
// 		klog.V(5).Infof("\n\nConnecting to the Hub with master-url: %s\n\tcontext: %s\n\tfrom kubeconfig: %s\n\n",
// 			testOptions.HubCluster.MasterURL, testOptions.HubCluster.KubeContext, testOptions.KubeConfig)

// 		if testOptions.Connection.Keys.AWS.AWSAccessID == "" ||
// 			testOptions.Connection.Keys.AWS.AWSAccessSecret == "" {
// 			Skip("Hive Provision not executed because no AWS AccessID/SecretKey was provided")
// 		}

// 		if testOptions.Connection.Keys.GCP.ProjectID == "" ||
// 			testOptions.Connection.Keys.GCP.ServiceAccountJsonKey == "" {
// 			Skip("Hive Provision not executed because no GCP ProjectID/ServiceAccountJsonKey was provided")
// 		}

// 		if testOptions.Connection.Keys.Azure.BaseDnsDomain == "" ||
// 			testOptions.Connection.Keys.Azure.ServicePrincipalJson == "" ||
// 			testOptions.Connection.Keys.Azure.BaseDomainRGN == "" ||
// 			testOptions.Connection.Keys.Azure.Region == "" {
// 			Skip("Hive Provision not executed because no Azure BaseDnsDomain/ServicePrincipalJson/BaseDomainRGN/Region was provided")
// 		}

// 		//Setup our kube connections
// 		var err error
// 		hubClient, err = libgoclient.NewKubeClient(testOptions.HubCluster.MasterURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext)
// 		Expect(err).To(BeNil())
// 		dynClient, err = libgoclient.NewKubeClientDynamic(testOptions.HubCluster.MasterURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext)
// 		Expect(err).To(BeNil())
// 	})

// 	It("should have the expected deployments in open-cluster-management namespace (install/g0)", func() {
// 		versionInfo, err := hubClient.Discovery().ServerVersion()
// 		Expect(err).NotTo(HaveOccurred())

// 		klog.V(1).Infof("Server version info: %v", versionInfo)
// 		klog.V(1).Infof("Hub namespace: %s", hubNamespace)

// 		var deployments = hubClient.AppsV1().Deployments(hubNamespace)

// 		// Expect(deployments.Get("cert-manager-webhook", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("console-header", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("etcd-operator", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("mcm-apiserver", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("mcm-controller", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("mcm-webhook", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("multicluster-operators-application", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("multicluster-operators-hub-subscription", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("multicluster-operators-standalone-subscription", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("multiclusterhub-repo", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("multiclusterhub-operator", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("rcm-controller", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("search-operator", metav1.GetOptions{})).NotTo(BeNil())
// 		// Expect(deployments.Get("mcm-controller", metav1.GetOptions{})).NotTo(BeNil())
// 		// //hive operator is now moved to this namespace since hive is pulled in via OLM
// 		// Expect(deployments.Get("hive-operator", metav1.GetOptions{})).NotTo(BeNil())

// 		// var deploymentList *appsv1.DeploymentList
// 		// deploymentList, err = deployments.List(metav1.ListOptions{})

// 		// Expect(err).NotTo(HaveOccurred())
// 		// println(deploymentList)
// 		// for _, d := range deploymentList.Items {
// 		// 	Expect(d.Status.Replicas).To(Equal(d.Status.ReadyReplicas))
// 		// 	for _, condition := range d.Status.Conditions {
// 		// 		if condition.Reason == "MinimumReplicasAvailable" {
// 		// 			Expect(condition.Status).To(Equal(corev1.ConditionTrue))
// 		// 		}
// 		// 	}
// 		// }
// 	})

// 	It("should have the expected deployments in hive namespace (install/g0)", func() {
// 		var err error
// 		var deployment *appsv1.Deployment

// 		var deployments = hubClient.AppsV1().Deployments("hive")
// 		deployment, err = deployments.Get("hive-controllers", metav1.GetOptions{})
// 		Expect(err).NotTo(HaveOccurred())
// 		Expect(deployment).NotTo(BeNil())
// 		for _, condition := range deployment.Status.Conditions {
// 			if condition.Reason == "MinimumReplicasAvailable" {
// 				Expect(condition.Status).To(Equal(corev1.ConditionTrue))
// 			}
// 		}

// 		Expect(deployments.Get("hiveadmission", metav1.GetOptions{})).NotTo(BeNil())

// 	})

// 	/*----------------------------------------------------------------------------
// 			Amazon Web Servcies (AWS) provisioned managed cluster
// 	------------------------------------------------------------------------------*/

// 	It("should be able to provision an AWS cluster using hive (cluster/g5/hive/aws)", func() {

// 		_, err := hubClient.Discovery().ServerVersion()
// 		Expect(err).NotTo(HaveOccurred())

// 		klog.V(1).Infof("hive cluster name: %s", hiveClusterName)

// 		By("Creating a new namespace for the hive deployed cluster", func() {

// 			_, err := hubClient.CoreV1().Namespaces().Create(&corev1.Namespace{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name: hiveClusterName,
// 				},
// 			})
// 			if err != nil {
// 				fmt.Printf("error encountered during namespace creation: %s \n", err.Error())
// 			}
// 			Expect(err).NotTo(HaveOccurred())

// 		})

// 		/*----------------------------------------------------------------------------
// 				Amazon Web Servcies (AWS) provider connection
// 		------------------------------------------------------------------------------*/

// 		By("creating the provider connection secret for AWS", func() {

// 			type AWSProvConn struct {
// 				AWSAccessID          string `json:"awsAccessKeyID"`
// 				AWSSecretAccessKeyID string `json:"awsSecretAccessKeyID"`
// 				PullSecret           string `json:"pullSecret"`
// 				SSHPrivateKey        string `json:"sshPrivatekey"`
// 				SSHPublicKey         string `json:"sshPublickey"`
// 				IsOcp                bool   `json:"isOcp"`
// 			}

// 			awsProvConn := AWSProvConn{}
// 			awsProvConn.AWSAccessID = testOptions.Connection.Keys.AWS.AWSAccessID
// 			awsProvConn.AWSSecretAccessKeyID = testOptions.Connection.Keys.AWS.AWSAccessSecret
// 			awsProvConn.PullSecret = testOptions.Connection.PullSecret
// 			awsProvConn.SSHPrivateKey = testOptions.Connection.SSHPrivateKey
// 			awsProvConn.SSHPublicKey = testOptions.Connection.SSHPublicKey
// 			awsProvConn.IsOcp = true

// 			var b []byte
// 			b, _ = yaml.Marshal(awsProvConn)

// 			result, err := hubClient.CoreV1().Secrets(hiveClusterName).Create(&corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name: hiveClusterName + "-aws-conn",
// 					Labels: map[string]string{"cluster.open-cluster-management.io/cloudconnection": "",
// 						"cluster.open-cluster-management.io/provider": "aws"},
// 				},
// 				Type: corev1.SecretTypeOpaque,
// 				Data: map[string][]byte{
// 					"metadata": b,
// 				},
// 			})
// 			Expect(err).NotTo(HaveOccurred())
// 			fmt.Printf("Created provider connection for AWS %q.\n", result.GetName())
// 		})

// 		By("creating the endpointConfig for AWS", func() {
// 			endpointconfig := &unstructured.Unstructured{
// 				Object: map[string]interface{}{
// 					"apiVersion": "multicloud.ibm.com/v1alpha1",
// 					"kind":       "EndpointConfig",
// 					"metadata": map[string]interface{}{
// 						"name":      hiveClusterName,
// 						"namespace": hiveClusterName,
// 					},
// 					"spec": map[string]interface{}{
// 						"applicationManager": map[string]interface{}{
// 							"enabled": true,
// 						},
// 						"bootstrapConfig": map[string]interface{}{
// 							"hubSecret": "multicluster-endpoint/klusterlet-bootstrap",
// 						},
// 						"clusterLabels": map[string]interface{}{
// 							"cloud":  "auto-detect",
// 							"vendor": "auto-detect",
// 						},
// 						"clusterName":      hiveClusterName,
// 						"clusterNamespace": hiveClusterName,
// 						"connectionManager": map[string]interface{}{
// 							"enabledGlobalView": false,
// 						},
// 						"policyController": map[string]interface{}{
// 							"enabled": true,
// 						},
// 						"searchCollector": map[string]interface{}{
// 							"enabled": true,
// 						},
// 						"serviceRegistry": map[string]interface{}{
// 							"enabled":   true,
// 							"dnsSuffix": "mcm.svc",
// 							"plugins":   "kube-service",
// 						},
// 						"topologyCollector": map[string]interface{}{
// 							"enabled":        true,
// 							"updateInterval": 0,
// 						},
// 						"cisController": map[string]interface{}{
// 							"enabled": false,
// 						},
// 						"certPolicyController": map[string]interface{}{
// 							"enabled": true,
// 						},
// 						"iamPolicyController": map[string]interface{}{
// 							"enabled": true,
// 						},
// 						"version": "1.0.0",
// 					},
// 				},
// 			}

// 			// create EndpointConfig
// 			fmt.Println("Creating endpointConfig...")
// 			endpointgvr := schema.GroupVersionResource{Group: "multicloud.ibm.com", Version: "v1alpha1", Resource: "endpointconfigs"}
// 			result, err := dynClient.Resource(endpointgvr).Namespace(hiveClusterName).Create(endpointconfig, metav1.CreateOptions{})
// 			if err != nil {
// 				fmt.Printf("error encountered during endpointConfig creation: %s \n", err.Error())
// 			}
// 			Expect(err).NotTo(HaveOccurred())

// 			fmt.Printf("Created endpointConfig %q.\n", result.GetName())
// 		})

// 		By("creating the cred secret for AWS", func() {

// 			result, err := hubClient.CoreV1().Secrets(hiveClusterName).Create(&corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name: hiveClusterName + "-aws-creds",
// 				},
// 				Type: corev1.SecretTypeOpaque,
// 				Data: map[string][]byte{
// 					"aws_access_key_id":     []byte(testOptions.Connection.Keys.AWS.AWSAccessID),
// 					"aws_secret_access_key": []byte(testOptions.Connection.Keys.AWS.AWSAccessSecret),
// 				},
// 			})
// 			Expect(err).NotTo(HaveOccurred())
// 			fmt.Printf("Created awsCredSecret %q.\n", result.GetName())
// 		})

// 		By("creating the installConfigSecret for AWS", func() {

// 			installConfigSecret, err := hubClient.CoreV1().Secrets(hiveClusterName).Create(&corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      hiveClusterName + "-install-config",
// 					Namespace: hiveClusterName,
// 				},
// 				Type: corev1.SecretTypeOpaque,
// 				Data: map[string][]byte{
// 					"install-config.yaml": []byte(installConfigAWS),
// 				},
// 			})
// 			Expect(err).NotTo(HaveOccurred())
// 			fmt.Printf("Created installConfigSecret %q.\n", installConfigSecret.Name)
// 		})

// 		By("creating the pullSecretRef for AWS", func() {

// 			result, err := hubClient.CoreV1().Secrets(hiveClusterName).Create(&corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name: hiveClusterName + "-pull-secret",
// 				},
// 				Type: corev1.SecretTypeDockerConfigJson,
// 				StringData: map[string]string{
// 					corev1.DockerConfigJsonKey: testOptions.Connection.PullSecret,
// 				},
// 			})
// 			Expect(err).NotTo(HaveOccurred())
// 			fmt.Printf("Created pullsecretref %q.\n", result.GetName())
// 		})

// 		//create SSH private key secret
// 		By("creating the ssh private secret for AWS", func() {
// 			_, err = hubClient.CoreV1().Secrets(hiveClusterName).Create(&corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name: hiveClusterName + "-ssh-private-key",
// 				},
// 				Type: corev1.SecretTypeOpaque,
// 				Data: map[string][]byte{
// 					"ssh-privatekey": []byte(testOptions.Connection.SSHPrivateKey)},
// 			})
// 			Expect(err).NotTo(HaveOccurred())
// 		})

// 		By("creating clusterImageSet for AWS ", func() {

// 			clusterImageSetRes := schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "clusterimagesets"}
// 			clusterImageSet := &unstructured.Unstructured{
// 				Object: map[string]interface{}{
// 					"metadata": map[string]interface{}{
// 						"name": hiveClusterName + "-clusterimageset",
// 					},
// 					"apiVersion": "hive.openshift.io/v1",
// 					"kind":       "ClusterImageSet",
// 					"spec": map[string]interface{}{
// 						"releaseImage": OCP_VERSION,
// 					},
// 				},
// 			}
// 			fmt.Println("Creating clusterImageSet...")
// 			result, err := dynClient.Resource(clusterImageSetRes).Create(clusterImageSet, metav1.CreateOptions{})
// 			if err != nil {
// 				fmt.Printf("error encountered during clusterImageSet creation: %s \n", err.Error())
// 				panic(err)
// 			}
// 			fmt.Printf("Created clusterImageSet %q.\n", result.GetName())

// 			Expect(err).NotTo(HaveOccurred())
// 		})

// 		By("creating the clusterDeployment for AWS", func() {
// 			// clusterDeployment def
// 			deploymentRes := schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments"}
// 			deployment := &unstructured.Unstructured{
// 				Object: map[string]interface{}{
// 					"apiVersion": "hive.openshift.io/v1",
// 					"kind":       "ClusterDeployment",
// 					"metadata": map[string]interface{}{
// 						"labels": map[string]interface{}{
// 							"cloud":       "AWS",
// 							"region":      testOptions.Connection.Keys.AWS.Region,
// 							"environment": "dev",
// 							"vendor":      "OCP",
// 						},
// 						"name":      hiveClusterName,
// 						"namespace": hiveClusterName,
// 					},
// 					"spec": map[string]interface{}{
// 						"baseDomain":  testOptions.Connection.Keys.AWS.BaseDnsDomain,
// 						"clusterName": hiveClusterName,
// 						"controlPlaneConfig": map[string]interface{}{
// 							"servingCertificates": map[string]interface{}{},
// 						},
// 						"installed": false,
// 						"platform": map[string]interface{}{
// 							"aws": map[string]interface{}{
// 								"credentialsSecretRef": corev1.LocalObjectReference{
// 									Name: hiveClusterName + "-aws-creds",
// 								},
// 								"region": testOptions.Connection.Keys.AWS.Region,
// 							},
// 						},
// 						"provisioning": map[string]interface{}{
// 							"installConfigSecretRef": corev1.LocalObjectReference{
// 								Name: hiveClusterName + "-install-config",
// 							},
// 							"sshPrivateKeySecretRef": map[string]interface{}{
// 								"name": hiveClusterName + "-ssh-private-key",
// 							},
// 							"imageSetRef": map[string]interface{}{
// 								"name": hiveClusterName + "-clusterimageset",
// 							},
// 						},
// 						"pullSecretRef": map[string]interface{}{
// 							"name": hiveClusterName + "-pull-secret",
// 						},
// 					},
// 				},
// 			}

// 			// create ClusterDeployment
// 			klog.V(1).Info("Creating deployment...")
// 			result, err := dynClient.Resource(deploymentRes).Namespace(hiveClusterName).Create(deployment, metav1.CreateOptions{})
// 			if err != nil {
// 				fmt.Printf("error encountered during deployment creation: %s \n", err.Error())
// 				panic(err)
// 			}
// 			fmt.Printf("Created deployment %q.\n", result.GetName())

// 		})

// 		By("creating the Cluster for AWS", func() {
// 			clusterRes := schema.GroupVersionResource{Group: "clusterregistry.k8s.io", Version: "v1alpha1", Resource: "clusters"}
// 			cluster := &unstructured.Unstructured{
// 				Object: map[string]interface{}{
// 					"apiVersion": "clusterregistry.k8s.io/v1alpha1",
// 					"kind":       "Cluster",
// 					"metadata": map[string]interface{}{
// 						"labels": map[string]interface{}{
// 							"cloud":  "auto-detect",
// 							"name":   hiveClusterName,
// 							"vendor": "auto-detect",
// 						},
// 						"name":      hiveClusterName,
// 						"namespace": hiveClusterName,
// 					},
// 					"spec": map[string]interface{}{},
// 				},
// 			}

// 			// create Cluster
// 			fmt.Println("Creating cluster...")
// 			result, err := dynClient.Resource(clusterRes).Namespace(hiveClusterName).Create(cluster, metav1.CreateOptions{})
// 			if err != nil {
// 				fmt.Printf("error encountered during cluster creation: %s \n", err.Error())
// 				panic(err)
// 			}
// 			fmt.Printf("Created cluster %q.\n", result.GetName())

// 		})
// 		//Create MachinePool
// 		By("creating the MachinePool for AWS", func() {
// 			machinePool := &unstructured.Unstructured{
// 				Object: map[string]interface{}{
// 					"apiVersion": "hive.openshift.io/v1",
// 					"kind":       "MachinePool",
// 					"metadata": map[string]interface{}{
// 						"name":      hiveClusterName + "-worker",
// 						"namespace": hiveClusterName,
// 					},
// 					"spec": map[string]interface{}{
// 						"clusterDeploymentRef": map[string]string{
// 							"name": hiveClusterName,
// 						},
// 						"name": "worker",
// 						"platform": map[string]interface{}{
// 							"aws": map[string]interface{}{
// 								"rootVolume": map[string]interface{}{
// 									"iops": 100,
// 									"size": 500,
// 									"type": "gp2",
// 								},
// 								"type": "m4-large",
// 							},
// 						},
// 						"replicas": 3,
// 					},
// 				},
// 			}
// 			gvr := schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "machinepools"}
// 			_, err = dynClient.Resource(gvr).Namespace(hiveClusterName).Create(machinePool, metav1.CreateOptions{})
// 			Expect(err).NotTo(HaveOccurred())
// 		})

// 		When("Import launched, wait for cluster ready for AWS", func() {
// 			fmt.Printf("Checking for cluster ready status")
// 			gvr := schema.GroupVersionResource{Group: "clusterregistry.k8s.io", Version: "v1alpha1", Resource: "clusters"}
// 			Eventually(func() bool {
// 				klog.V(1).Info("Wait cluster ready...")
// 				cluster, err := dynClient.Resource(gvr).Namespace(hiveClusterName).Get(hiveClusterName, metav1.GetOptions{})
// 				if err == nil {
// 					return utils.StatusContainsTypeEqualTo(cluster, "OK")
// 				}
// 				return false
// 			}, "65m", "2m").Should(BeTrue())
// 			klog.V(1).Info("Cluster imported")
// 		})

// 		// delete clusterdeployment
// 		By("deleting the clusterdeployment for AWS", func() {
// 			gvr := schema.GroupVersionResource{Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments"}
// 			Expect(dynClient.Resource(gvr).Namespace(hiveClusterName).Delete(hiveClusterName, &metav1.DeleteOptions{})).NotTo(HaveOccurred())

// 			Eventually(func() bool {
// 				klog.V(1).Infof("Wait clusterdeployment %s deletion...", hiveClusterName)
// 				_, err := dynClient.Resource(gvr).Namespace(hiveClusterName).Get(hiveClusterName, metav1.GetOptions{})
// 				if err != nil {
// 					klog.V(1).Info(err)
// 					return errors.IsNotFound(err)
// 				}
// 				return false
// 			}, "20m", "1m").Should(BeTrue())
// 			klog.V(1).Infof("clusterdeployment %s deleted", hiveClusterName)
// 		})

// 		// delete clusterregistry.cluster
// 		By("deleting the clusterregistry.cluster for AWS", func() {
// 			gvr := schema.GroupVersionResource{Group: "clusterregistry.k8s.io", Version: "v1alpha1", Resource: "clusters"}
// 			Expect(dynClient.Resource(gvr).Namespace(hiveClusterName).Delete(hiveClusterName, &metav1.DeleteOptions{})).NotTo(HaveOccurred())

// 			Eventually(func() bool {
// 				klog.V(1).Infof("Wait clusterregistry %s deletion...", hiveClusterName)
// 				_, err := dynClient.Resource(gvr).Namespace(hiveClusterName).Get(hiveClusterName, metav1.GetOptions{})
// 				if err != nil {
// 					klog.V(1).Info(err)
// 					return errors.IsNotFound(err)
// 				}
// 				return false
// 			}, "10m", "1m").Should(BeTrue())
// 			klog.V(1).Infof("clusterregistry %s  deleted", hiveClusterName)

// 		})

// 		By("deleting namespace for the hive deployed AWS cluster", func() {

// 			err := hubClient.CoreV1().Namespaces().Delete(hiveClusterName, &metav1.DeleteOptions{})
// 			if err != nil {
// 				fmt.Printf("error encountered during namespace creation: %s \n", err.Error())
// 			}
// 			Expect(err).NotTo(HaveOccurred())

// 			Eventually(func() bool {
// 				klog.V(1).Infof("Wait namespace %s deletion...", hiveClusterName)
// 				_, err = hubClient.CoreV1().Namespaces().Get(hiveClusterName, metav1.GetOptions{})
// 				if err != nil {
// 					return errors.IsNotFound(err)
// 				}
// 				return false
// 			}, "10m", "1m").Should(BeTrue())
// 			klog.V(1).Infof("namespace %s deleted", hiveClusterName)
// 		})
// 	})

// })
