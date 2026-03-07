// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/test/e2e-new/framework"
)

// hub is the primary test context, providing all clients and assertion methods.
var hub *framework.Hub

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "End-to-end Test Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	var err error
	hub, err = framework.NewHub("")
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	createGlobalKlusterletConfig()
})

func createGlobalKlusterletConfig() {
	ginkgo.By("Create global KlusterletConfig, set work status sync interval", func() {
		_, err := hub.KlusterletCfgClient.ConfigV1alpha1().KlusterletConfigs().Create(context.TODO(),
			&klusterletconfigv1alpha1.KlusterletConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.GlobalKlusterletConfigName,
				},
				Spec: klusterletconfigv1alpha1.KlusterletConfigSpec{
					WorkStatusSyncInterval: &metav1.Duration{Duration: 5 * time.Second},
				},
			}, metav1.CreateOptions{})
		if !errors.IsAlreadyExists(err) {
			gomega.Expect(err).Should(gomega.Succeed())
		}
	})
}
