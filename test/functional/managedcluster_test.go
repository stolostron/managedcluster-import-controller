// Copyright (c) 2020 Red Hat, Inc.

// +build functional

package functional

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/rand"
	"net"
	"os"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	libgoapplier "github.com/open-cluster-management/library-go/pkg/applier"
	libgoclient "github.com/open-cluster-management/library-go/pkg/client"
	libgoconfig "github.com/open-cluster-management/library-go/pkg/config"
	libgounstructured "github.com/open-cluster-management/library-go/pkg/unstructured"

	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const (
	clusterRolePrefix                  = "system:open-cluster-management:managedcluster:bootstrap:"
	clusterRoleBindingPrefix           = "system:open-cluster-management:managedcluster:bootstrap:"
	bootstrapServiceAccountNamePostfix = "-bootstrap-sa"
	manifestWorkNamePostfix            = "-klusterlet"
	syncsetNamePostfix                 = "-klusterlet"
	importSecretNamePostfix            = "-import"
	klusterletImageName                = "KLUSTERLET_OPERATOR_IMAGE"
)

var _ = Describe("Managedcluster", func() {
	myTestNameSpace := "managedcluster-test"
	BeforeEach(func() {
		SetDefaultEventuallyTimeout(10 * time.Second)
		SetDefaultEventuallyPollingInterval(1 * time.Second)
		os.Setenv(klusterletImageName, "quay.io/open-cluster-management/nucleus:latest")
		// clean(clientHubDynamic, clientHub, myTestNameSpace)
	})

	AfterEach(func() {
		clientHub.CoreV1().Namespaces().Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
		Eventually(func() error {
			klog.V(1).Info("Wait namespace deleted")
			ns := clientHub.CoreV1().Namespaces()
			_, err := ns.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())
	})

	Context("Without creating cluster namespace", func() {
		By("Creating the ManagedCluster", func() {
			It("Should not create the bootstrap service account", func() {
				managedCluster := newManagedcluster(myTestNameSpace)
				createNewUnstructuredClusterScoped(clientHubDynamic, gvrManagedcluster, managedCluster, myTestNameSpace)
				klog.V(1).Infof("Check for bootstrap service account not exists")
				ns := clientHubDynamic.Resource(gvrServiceaccount)
				Consistently(func() error {
					_, err := ns.Namespace(myTestNameSpace).Get(context.TODO(), myTestNameSpace+bootstrapServiceAccountNamePostfix, metav1.GetOptions{})
					return err
				}, 4, 20).ShouldNot(BeNil())

				clean(clientHubDynamic, clientHub, myTestNameSpace)
			})
		})
	})

	Context("With the creation cluster namespace", func() {
		It("Should create with manifest (import-managedcluster/with-manifestwork)", func() {
			By("Creating namespace", func() {
				ns := clientHub.CoreV1().Namespaces()
				klog.V(5).Infof("Create the namespace %s", myTestNameSpace)
				Expect(ns.Create(context.TODO(), &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: myTestNameSpace,
					},
				}, metav1.CreateOptions{})).NotTo(BeNil())
			})
			By("Creating the ManagedCluster", func() {
				By("Creating managedCluster")
				managedCluster := newManagedcluster(myTestNameSpace)
				createNewUnstructuredClusterScoped(clientHubDynamic, gvrManagedcluster, managedCluster, myTestNameSpace)

				By("checking ManagedCluster Creation")
				checkManagedClusterCreation(clientHubDynamic, clientHub, myTestNameSpace, myTestNameSpace, gvrManagedcluster, gvrServiceaccount, gvrSecret)
				Consistently(func() error {
					klog.V(1).Infof("Make sure ManifestWork %s is not created", myTestNameSpace+manifestWorkNamePostfix+"-crds")
					ns := clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix+"-crds", metav1.GetOptions{})
					return err
				}, 4, 20).ShouldNot(BeNil())
				Consistently(func() error {
					klog.V(1).Infof("Make sure ManifestWork %s is not created", myTestNameSpace+manifestWorkNamePostfix)
					ns := clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix, metav1.GetOptions{})
					return err
				}, 4, 20).ShouldNot(BeNil())
				status := `{"status":` +
					`{"conditions":[` +
					`{"type":"ManagedClusterConditionAvailable","lastTransitionTime":"2020-01-01T01:01:01Z","message":"Managed cluster joined","status":"True","reason":"ManagedClusterJoined"}` +
					`]}}`
				By("Set status ManagedClusterConditionAvailable to True")
				ns := clientHubDynamic.Resource(gvrManagedcluster)
				managedCluster, err := ns.Patch(context.TODO(), myTestNameSpace, types.MergePatchType, []byte(status), metav1.PatchOptions{}, "status")
				Expect(err).Should(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait ManifestWork %s", myTestNameSpace+manifestWorkNamePostfix+"-crds")
					ns := clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix+"-crds", metav1.GetOptions{})
					return err
				}).Should(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait ManifestWork %s", myTestNameSpace+manifestWorkNamePostfix)
					ns := clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix, metav1.GetOptions{})
					return err
				}).Should(BeNil())
				Consistently(func() error {
					klog.V(1).Infof("Make sure SyncSet %s doesn't exist", myTestNameSpace+syncsetNamePostfix+"-crds")
					ns := clientHubDynamic.Resource(gvrSyncset).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix+"-crds", metav1.GetOptions{})
					return err
				}, 4, 20).ShouldNot(BeNil())
				Consistently(func() error {
					klog.V(1).Infof("Make sure SyncSet %s doesn't exist", myTestNameSpace+syncsetNamePostfix)
					ns := clientHubDynamic.Resource(gvrSyncset).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix, metav1.GetOptions{})
					return err
				}, 4, 20).ShouldNot(BeNil())
			})
			By("Updating ServiceAccount", func() {
				ns := clientHub.CoreV1().ServiceAccounts(myTestNameSpace)
				obj, err := ns.Get(context.TODO(), myTestNameSpace+bootstrapServiceAccountNamePostfix, metav1.GetOptions{})
				Expect(err).To(BeNil())
				obj.Secrets = []corev1.ObjectReference{}
				_, err = ns.Update(context.TODO(), obj, metav1.UpdateOptions{})
				Expect(err).To(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait ServiceAccount %s", myTestNameSpace+bootstrapServiceAccountNamePostfix)
					objGet, err := ns.Get(context.TODO(), myTestNameSpace+bootstrapServiceAccountNamePostfix, metav1.GetOptions{})
					if err != nil {
						return err
					}
					klog.V(5).Infof("ServiceAccount %v", objGet)

					if len(objGet.Secrets) == 0 {
						return fmt.Errorf("ServiceAccount has no secret set %s", objGet.GetName())
					}
					return nil
				}).Should(BeNil())
			})

			By("Updating ClusterRole", func() {
				ns := clientHub.RbacV1().ClusterRoles()
				obj, err := ns.Get(context.TODO(), clusterRolePrefix+myTestNameSpace, metav1.GetOptions{})
				Expect(err).To(BeNil())
				objOri := obj.DeepCopy()
				obj.Rules = nil
				_, err = ns.Update(context.TODO(), obj, metav1.UpdateOptions{})
				Expect(err).To(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait ClusterRole %s", clusterRolePrefix+myTestNameSpace)
					objGet, err := ns.Get(context.TODO(), clusterRolePrefix+myTestNameSpace, metav1.GetOptions{})
					if err != nil {
						return err
					}
					if !reflect.DeepEqual(objOri.Rules, objGet.Rules) {
						return fmt.Errorf("ClusterRole.Rules expect\n%v\ngot\n%v\n", objOri.Rules, objGet.Rules)
					}
					return nil
				}).Should(BeNil())
			})

			By("Updating ClusterRoleBinding", func() {
				ns := clientHub.RbacV1().ClusterRoleBindings()
				obj, err := ns.Get(context.TODO(), clusterRoleBindingPrefix+myTestNameSpace, metav1.GetOptions{})
				Expect(err).To(BeNil())
				objOri := obj.DeepCopy()
				obj.Subjects = nil
				_, err = ns.Update(context.TODO(), obj, metav1.UpdateOptions{})
				Expect(err).To(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait ClusterRoleBinding %s", clusterRoleBindingPrefix+myTestNameSpace)
					objGet, err := ns.Get(context.TODO(), clusterRoleBindingPrefix+myTestNameSpace, metav1.GetOptions{})
					if err != nil {
						return err
					}
					if !reflect.DeepEqual(objOri.Subjects, objGet.Subjects) {
						return fmt.Errorf("ClusterRoleBinding.Subject expect\n%v\ngot\n%v\n", objOri.Subjects, objGet.Subjects)
					}
					return nil
				}).Should(BeNil())
			})

			By("Updating ManifestWorks", func() {
				ns := clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
				obj, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix, metav1.GetOptions{})
				Expect(err).To(BeNil())
				delete(obj.Object, "spec")
				_, err = ns.Update(context.TODO(), obj, metav1.UpdateOptions{})
				Expect(err).To(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait ManifestWork %s", myTestNameSpace+manifestWorkNamePostfix)
					objGet, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix, metav1.GetOptions{})
					if err != nil {
						return err
					}

					if _, ok := objGet.Object["spec"]; !ok {
						return fmt.Errorf("ManifestWork spec empty %s", objGet.GetName())
					}
					return nil
				}).Should(BeNil())
			})

			By("Deleting the ManagedCluster", func() {
				status := `{"status":` +
					`{"conditions":[` +
					`{"type":"ManagedClusterConditionAvailable","lastTransitionTime":"2020-01-01T01:01:01Z","message":"Managed cluster not available","status":"False","reason":"ManagedClusterNotAvailable"}` +
					`]}}`
				By("Set status ManagedClusterConditionAvailable to False")
				ns := clientHubDynamic.Resource(gvrManagedcluster)
				_, err := ns.Patch(context.TODO(), myTestNameSpace, types.MergePatchType, []byte(status), metav1.PatchOptions{}, "status")
				Expect(err).Should(BeNil())
				Expect(clientHubDynamic.Resource(gvrManagedcluster).Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})).Should(BeNil())
				checkManagedClusterDeletion(clientHubDynamic, clientHub, myTestNameSpace, myTestNameSpace, gvrManagedcluster)
			})

			By("Check if manifestWork deleted", func() {
				Eventually(func() error {
					klog.V(1).Infof("Wait delete ManifestWork CRDs %s", myTestNameSpace+manifestWorkNamePostfix+"-crds")
					ns := clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix+"-crds", metav1.GetOptions{})
					return err
				}).ShouldNot(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait delete ManifestWork %s", myTestNameSpace+manifestWorkNamePostfix)
					ns := clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix, metav1.GetOptions{})
					return err
				}).ShouldNot(BeNil())
			})
		})

		It("Should create synset (import-managedcluster/with-syncset)", func() {
			By("Creating namespace", func() {
				ns := clientHub.CoreV1().Namespaces()
				klog.V(5).Infof("Create the namespace %s", myTestNameSpace)
				Expect(ns.Create(context.TODO(), &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: myTestNameSpace,
					},
				}, metav1.CreateOptions{})).NotTo(BeNil())
			})
			By("Creating the Cluster", func() {
				clusterdeployment := newClusterdeployment(myTestNameSpace)
				createNewUnstructured(clientHubDynamic, gvrClusterdeployment,
					clusterdeployment, myTestNameSpace, myTestNameSpace)
				managedCluster := newManagedcluster((myTestNameSpace))
				createNewUnstructuredClusterScoped(clientHubDynamic, gvrManagedcluster, managedCluster, myTestNameSpace)
				checkManagedClusterCreation(clientHubDynamic, clientHub, myTestNameSpace, myTestNameSpace, gvrManagedcluster, gvrServiceaccount, gvrSecret)
				Eventually(func() error {
					klog.V(1).Infof("Wait SyncSet %s", myTestNameSpace+syncsetNamePostfix+"-crds")
					ns := clientHubDynamic.Resource(gvrSyncset).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix+"-crds", metav1.GetOptions{})
					return err
				}).Should(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait SyncSet %s", myTestNameSpace+syncsetNamePostfix)
					ns := clientHubDynamic.Resource(gvrSyncset).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix, metav1.GetOptions{})
					return err
				}).Should(BeNil())
				Consistently(func() error {
					klog.V(1).Infof("Make sure ManifestWork %s is not created", myTestNameSpace+manifestWorkNamePostfix+"-crds")
					ns := clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix+"-crds", metav1.GetOptions{})
					return err
				}, 4, 20).ShouldNot(BeNil())
				Consistently(func() error {
					klog.V(1).Infof("Make sure ManifestWork %s is not created", myTestNameSpace+manifestWorkNamePostfix)
					ns := clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix, metav1.GetOptions{})
					return err
				}, 4, 20).ShouldNot(BeNil())
			})

			By("Updating SyncSet", func() {
				ns := clientHubDynamic.Resource(gvrSyncset).Namespace(myTestNameSpace)
				obj, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix, metav1.GetOptions{})
				Expect(err).To(BeNil())
				delete(obj.Object, "spec")
				_, err = ns.Update(context.TODO(), obj, metav1.UpdateOptions{})
				Expect(err).To(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait SyncSset %s", myTestNameSpace+syncsetNamePostfix)
					objGet, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix, metav1.GetOptions{})
					if err != nil {
						return err
					}

					if _, ok := objGet.Object["spec"]; !ok {
						return fmt.Errorf("SyncSet spec empty %s", objGet.GetName())
					}
					return nil
				}).Should(BeNil())
			})

			By("Updating SyncSet CRDS", func() {
				ns := clientHubDynamic.Resource(gvrSyncset).Namespace(myTestNameSpace)
				obj, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix+"-crds", metav1.GetOptions{})
				Expect(err).To(BeNil())
				delete(obj.Object, "spec")
				_, err = ns.Update(context.TODO(), obj, metav1.UpdateOptions{})
				Expect(err).To(BeNil())
				Eventually(func() error {
					klog.V(1).Infof("Wait SyncSset %s", myTestNameSpace+syncsetNamePostfix+"-crds")
					objGet, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix+"-crds", metav1.GetOptions{})
					if err != nil {
						return err
					}

					if _, ok := objGet.Object["spec"]; !ok {
						return fmt.Errorf("SyncSet CRDS spec empty %s", objGet.GetName())
					}
					return nil
				}).Should(BeNil())
			})

			By("Deleting the ManagedCluster", func() {
				Expect(clientHubDynamic.Resource(gvrManagedcluster).Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})).Should(BeNil())
				checkManagedClusterDeletion(clientHubDynamic, clientHub, myTestNameSpace, myTestNameSpace, gvrManagedcluster)
			})

			By("Check if syncset deleted", func() {
				Eventually(func() error {
					ns := clientHubDynamic.Resource(gvrSyncset).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix+"-crds", metav1.GetOptions{})
					return err
				}).ShouldNot(BeNil())
				Eventually(func() error {
					ns := clientHubDynamic.Resource(gvrSyncset).Namespace(myTestNameSpace)
					_, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix, metav1.GetOptions{})
					return err
				}).ShouldNot(BeNil())
			})

		})
	})
})

func checkManagedClusterCreation(
	clientHubDynamic dynamic.Interface,
	clientHub kubernetes.Interface,
	name, namespace string, gvrManagedcluster, gvrServiceaccount, gvrSecret schema.GroupVersionResource) {
	klog.V(1).Infof("checkManagedClusterCreation with %s/%s", name, namespace)
	When("ManagedCluster created, wait finalizer", func() {
		klog.V(1).Infof("checks Finalizer")
		Eventually(func() bool {
			ns := clientHubDynamic.Resource(gvrManagedcluster)
			sc, err := ns.Get(context.TODO(), name, metav1.GetOptions{})
			Expect(err).To(BeNil())

			for _, f := range sc.GetFinalizers() {
				if f == "managedcluster-import-controller.managedcluster" {
					klog.V(5).Info("Finalizer added")
					return true
				}
			}
			klog.V(5).Info("Finalizer not yet added")
			return false
		}).Should(BeTrue())
	})
	When("ManagedCluster created, wait for Cluster role", func() {
		klog.V(1).Infof("Wait Cluster Role %s", clusterRolePrefix+name)
		Eventually(func() error {
			_, err := clientHub.RbacV1().ClusterRoles().Get(context.TODO(), clusterRolePrefix+name, metav1.GetOptions{})
			return err
		}).Should(BeNil())
	})
	When("ManagedCluster created, wait for Cluster rolebinding", func() {
		klog.V(1).Infof("Wait Cluster RoleBinding %s", clusterRoleBindingPrefix+name)
		Eventually(func() error {
			_, err := clientHub.RbacV1().ClusterRoleBindings().Get(context.TODO(), clusterRoleBindingPrefix+name, metav1.GetOptions{})
			return err
		}).Should(BeNil())
	})
	When("ManagedCluster created, wait for bootstrap service account", func() {
		klog.V(1).Infof("Wait for bootstrap service account %s", name+bootstrapServiceAccountNamePostfix)
		ns := clientHubDynamic.Resource(gvrServiceaccount)
		Eventually(func() error {
			_, err := ns.Namespace(name).Get(context.TODO(), name+bootstrapServiceAccountNamePostfix, metav1.GetOptions{})
			return err
		}).Should(BeNil())
	})
	When("ManagedCluster created, wait for import secret", func() {
		klog.V(1).Infof("Wait for import secret %s", name+importSecretNamePostfix)
		ns := clientHubDynamic.Resource(gvrSecret)
		Eventually(func() error {
			_, err := ns.Namespace(name).Get(context.TODO(), name+importSecretNamePostfix, metav1.GetOptions{})
			return err
		}).Should(BeNil())
	})
}

func checkManagedClusterDeletion(
	clientHubDynamic dynamic.Interface,
	clientHub kubernetes.Interface,
	name, namespace string, gvrManagedcluster schema.GroupVersionResource) {
	ns := clientHubDynamic.Resource(gvrManagedcluster)
	When("ManagedCluster deletion is requested, wait for all other finalizer to be removed", func() {
		klog.V(1).Info("Wait other finalizers removal")
		Eventually(func() bool {
			sc, err := ns.Get(context.TODO(), name, metav1.GetOptions{})
			if err == nil {
				if len(sc.GetFinalizers()) != 1 {
					klog.V(5).Infof("Waiting other finalizers to be removed %v ", sc.GetFinalizers())
					return false
				}
				return sc.GetFinalizers()[0] == "rcm-controller.managedcluster"
			}
			return errors.IsNotFound(err)
		}).Should(BeTrue())
	})

	When("All other finalizers are removed, wait the managedCluster finalizer to be removed", func() {
		klog.V(1).Info("Wait finalizer to be removed")
		Eventually(func() bool {
			sc, err := ns.Get(context.TODO(), name, metav1.GetOptions{})
			if err == nil {
				return len(sc.GetFinalizers()) == 0
			} else {
				return true
			}
		}).Should(BeTrue())
	})

	When("ManagedCluster finalizer is removed, wait the managedCluster deletion", func() {
		klog.V(1).Info("Wait managedcluster deleted")
		Eventually(func() error {
			_, err := ns.Get(context.TODO(), name, metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())
	})

}

func clean(clientHubDynamic dynamic.Interface,
	clientHub kubernetes.Interface,
	myTestNameSpace string,
) {
	klog.Infof("---------------------Cleaning environment--------------------")
	err := clientHub.CoreV1().Namespaces().Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
	if err == nil {
		ns := clientHubDynamic.Resource(gvrSyncset).Namespace(myTestNameSpace)
		ns.Delete(context.TODO(), myTestNameSpace+syncsetNamePostfix+"-crds", metav1.DeleteOptions{})
		Eventually("Wait deletion of syncset crds", func() error {
			_, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix+"-crds", metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())

		ns.Delete(context.TODO(), myTestNameSpace+syncsetNamePostfix, metav1.DeleteOptions{})
		Eventually("Wait deletion of syncset", func() error {
			_, err := ns.Get(context.TODO(), myTestNameSpace+syncsetNamePostfix, metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())

		ns = clientHubDynamic.Resource(gvrManifestwork).Namespace(myTestNameSpace)
		ns.Delete(context.TODO(), myTestNameSpace+manifestWorkNamePostfix+"-crds", metav1.DeleteOptions{})
		Eventually("Wait deletion of manifest crds", func() error {
			_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix+"-crds", metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())

		ns.Delete(context.TODO(), myTestNameSpace+manifestWorkNamePostfix, metav1.DeleteOptions{})
		Eventually("Wait deletion of manifest", func() error {
			_, err := ns.Get(context.TODO(), myTestNameSpace+manifestWorkNamePostfix, metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())

		s := clientHub.CoreV1().Secrets(myTestNameSpace)
		s.Delete(context.TODO(), myTestNameSpace+importSecretNamePostfix, metav1.DeleteOptions{})
		Eventually("Wait deletion of Secret import", func() error {
			_, err := s.Get(context.TODO(), myTestNameSpace+importSecretNamePostfix, metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())

		sa := clientHub.CoreV1().ServiceAccounts(myTestNameSpace)
		sa.Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
		Eventually("Wait deletion of ServiceAccount", func() error {
			_, err := sa.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())

		crb := clientHub.RbacV1().ClusterRoleBindings()
		crb.Delete(context.TODO(), clusterRoleBindingPrefix+myTestNameSpace, metav1.DeleteOptions{})
		Eventually("Wait deletion of ClusterRoleBinding", func() error {
			_, err := crb.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())

		cr := clientHub.RbacV1().ClusterRoles()
		cr.Delete(context.TODO(), clusterRolePrefix+myTestNameSpace, metav1.DeleteOptions{})
		Eventually("Wait deletion of ClusterRole", func() error {
			_, err := cr.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())

		clientHubDynamic.Resource(gvrClusterdeployment).Namespace(myTestNameSpace).Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
		sc, err := clientHubDynamic.Resource(gvrManagedcluster).Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
		if err == nil {
			sc.SetFinalizers([]string{})
			clientHubDynamic.Resource(gvrManagedcluster).Update(context.TODO(), sc, metav1.UpdateOptions{})
		}
		err = clientHub.CoreV1().Namespaces().Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
		Eventually(func() error {
			klog.V(1).Info("Wait namespace deleted")
			ns := clientHub.CoreV1().Namespaces()
			_, err := ns.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
			return err
		}).ShouldNot(BeNil())
	}
	err = clientHubDynamic.Resource(gvrManagedcluster).Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
	if err != nil {
		klog.V(5).Infof("ManagedCluster: %s", err.Error())
	}
	Eventually(func() error {
		klog.V(1).Info("Wait managedcluster deleted")
		ns := clientHubDynamic.Resource(gvrManagedcluster)
		_, err := ns.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
		return err
	}).ShouldNot(BeNil())

}

var _ = Describe("CSR Approval", func() {
	myTestNameSpace := "csrapproval-test"
	BeforeEach(func() {
		SetDefaultEventuallyTimeout(10 * time.Second)
		SetDefaultEventuallyPollingInterval(1 * time.Second)
		By("Creating managedCluster", func() {
			managedCluster := newManagedcluster(myTestNameSpace)
			createNewUnstructuredClusterScoped(clientHubDynamic, gvrManagedcluster, managedCluster, myTestNameSpace)
		})
		By("Create ClusterRole and ClusterRoleBinding", func() {
			var clientHubApplier *libgoapplier.Applier
			yamlReader := libgoapplier.NewYamlFileReader("resources")
			templateProcessor, err := libgoapplier.NewTemplateProcessor(yamlReader, &libgoapplier.Options{})
			Expect(err).To(BeNil())
			hubClientClient, err := libgoclient.NewDefaultClient("", client.Options{})
			Expect(err).To(BeNil())
			clientHubApplier, err = libgoapplier.NewApplier(templateProcessor, hubClientClient, nil, nil, nil)
			Expect(err).To(BeNil())
			values := struct {
				ClusterName string
			}{
				ClusterName: myTestNameSpace,
			}
			Expect(clientHubApplier.CreateOrUpdateInPath("csr", nil, false, values)).To(BeNil())
		})
	})

	AfterEach(func() {
		cleanCSR(myTestNameSpace)
	})

	Context("With valid CSR", func() {
		It("Should approve the CSR (approve-csr/valid)", func() {
			By("Creating the CSR", func() {
				config, err := libgoconfig.LoadConfig("", "", "")
				Expect(err).To(BeNil())
				config.Impersonate.UserName = fmt.Sprintf("system:serviceaccount:%s:%s-bootstrap-sa", myTestNameSpace, myTestNameSpace)
				clientset, err := kubernetes.NewForConfig(config)
				signerName := certificatesv1beta1.KubeAPIServerClientSignerName
				csr, err := newCSR(myTestNameSpace,
					map[string]string{"open-cluster-management.io/cluster-name": myTestNameSpace},
					&signerName,
					"redhat",
					[]string{"RedHat"},
					"",
					"CERTIFICATE REQUEST",
				)
				Expect(err).To(BeNil())
				signingRequest := clientset.CertificatesV1beta1().CertificateSigningRequests()
				_, err = signingRequest.Create(context.TODO(), csr, metav1.CreateOptions{})
				Expect(err).To(BeNil())
			})

			When("CSR created, waiting approval", func() {
				Eventually(func() error {
					klog.V(1).Infof("Wait approval of CSR %s", myTestNameSpace)
					ns := clientHubDynamic.Resource(gvrCSR)
					csr, err := ns.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
					Expect(err).To(BeNil())
					_, err = libgounstructured.GetCondition(csr, string(certificatesv1beta1.CertificateApproved))
					return err
				}).Should(BeNil())
			})
		})
	})

	Context("With CSR having wrong labels", func() {
		It("Should not approve the CSR (approve-csr/wrong-label)", func() {
			By("Creating the CSR", func() {
				config, err := libgoconfig.LoadConfig("", "", "")
				Expect(err).To(BeNil())
				config.Impersonate.UserName = fmt.Sprintf("system:serviceaccount:%s:%s-bootstrap-sa", myTestNameSpace, myTestNameSpace)
				clientset, err := kubernetes.NewForConfig(config)
				signerName := certificatesv1beta1.KubeAPIServerClientSignerName
				csr, err := newCSR(myTestNameSpace,
					map[string]string{"wronglabel": myTestNameSpace},
					&signerName,
					"redhat",
					[]string{"RedHat"},
					"",
					"CERTIFICATE REQUEST",
				)
				Expect(err).To(BeNil())
				signingRequest := clientset.CertificatesV1beta1().CertificateSigningRequests()
				_, err = signingRequest.Create(context.TODO(), csr, metav1.CreateOptions{})
				Expect(err).To(BeNil())
			})

			When("CSR created, waiting if get approved", func() {
				Consistently(func() error {
					klog.V(1).Infof("Wait approval of CSR %s", myTestNameSpace)
					ns := clientHubDynamic.Resource(gvrCSR)
					csr, err := ns.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
					Expect(err).To(BeNil())
					_, err = libgounstructured.GetCondition(csr, string(certificatesv1beta1.CertificateApproved))
					return err
				}).ShouldNot(BeNil())
			})
		})
	})

})

func newCSR(name string, labels map[string]string, signerName *string, cn string, orgs []string, username string, reqBlockType string) (*certificatesv1beta1.CertificateSigningRequest, error) {
	insecureRand := rand.New(rand.NewSource(0))
	pk, err := ecdsa.GenerateKey(elliptic.P256(), insecureRand)
	if err != nil {
		return nil, err
	}
	csrb, err := x509.CreateCertificateRequest(insecureRand, &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: orgs,
		},
		DNSNames:       []string{},
		EmailAddresses: []string{},
		IPAddresses:    []net.IP{},
	}, pk)
	if err != nil {
		return nil, err
	}
	return &certificatesv1beta1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: certificatesv1beta1.CertificateSigningRequestSpec{
			Username:   username,
			Usages:     []certificatesv1beta1.KeyUsage{},
			SignerName: signerName,
			Request:    pem.EncodeToMemory(&pem.Block{Type: reqBlockType, Bytes: csrb}),
		},
	}, nil
}

func cleanCSR(myTestNameSpace string) {
	clientHubDynamic.Resource(gvrCSR).Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
	Eventually(func() error {
		klog.V(1).Info("Wait CSR deleted")
		ns := clientHubDynamic.Resource(gvrCSR)
		_, err := ns.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
		return err
	}).ShouldNot(BeNil())
	crb := clientHub.RbacV1().ClusterRoleBindings()
	crb.Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
	Eventually(func() error {
		klog.V(1).Info("Wait clusterrolebinding deleted")
		_, err := crb.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
		return err
	}).ShouldNot(BeNil())

	cr := clientHub.RbacV1().ClusterRoles()
	cr.Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
	Eventually(func() error {
		klog.V(1).Info("Wait clusterroleb deleted")
		_, err := cr.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
		return err
	}).ShouldNot(BeNil())
	err := clientHubDynamic.Resource(gvrManagedcluster).Delete(context.TODO(), myTestNameSpace, metav1.DeleteOptions{})
	if err != nil {
		klog.V(5).Infof("ManagedCluster: %s", err.Error())
	}
	Eventually(func() error {
		klog.V(1).Info("Wait managedcluster deleted")
		ns := clientHubDynamic.Resource(gvrManagedcluster)
		_, err := ns.Get(context.TODO(), myTestNameSpace, metav1.GetOptions{})
		return err
	}).ShouldNot(BeNil())
}
