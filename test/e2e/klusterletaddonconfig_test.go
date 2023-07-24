// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package e2e

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/stolostron/managedcluster-import-controller/pkg/controller/agentregistration"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = ginkgo.Describe("Create KlusterletAddonConfig for managed clusters", ginkgo.Serial, func() {
	var managedClusterName string

	ginkgo.AfterEach(func() {
		assertManagedClusterDeleted(managedClusterName)
	})

	ginkgo.It("Should create the KlusterletAddonConfig for a managed cluster with label ", func() {
		managedClusterName = fmt.Sprintf("cluster-test-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedClusterWithAnnotations(hubClusterClient, managedClusterName, map[string]string{
				agentregistration.AnnotationCreateWithDefaultKlusterletAddonConfig: "",
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertKlusterletAddonConfigCreated(managedClusterName)
	})

	ginkgo.It("Should not create the KlusterletAddonConfig for managed cluster without the label", func() {
		managedClusterName = fmt.Sprintf("cluster-test-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			_, err := util.CreateManagedCluster(hubClusterClient, managedClusterName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		assertKlusterletAddonConfigNotCreated(managedClusterName)
	})
})
