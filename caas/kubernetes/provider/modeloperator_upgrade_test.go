// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/tc"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
)

type dummyUpgradeCAASModel struct {
	client *fake.Clientset
}

type modelUpgraderSuite struct {
	broker *dummyUpgradeCAASModel
}

var _ = tc.Suite(&modelUpgraderSuite{})

func (d *dummyUpgradeCAASModel) Client() kubernetes.Interface {
	return d.client
}

func (d *dummyUpgradeCAASModel) LabelVersion() constants.LabelVersion {
	return constants.LabelVersion1
}

func (d *dummyUpgradeCAASModel) Namespace() string {
	return "test"
}

func (s *modelUpgraderSuite) SetUpTest(c *tc.C) {
	s.broker = &dummyUpgradeCAASModel{
		client: fake.NewSimpleClientset(),
	}
}

func (s *modelUpgraderSuite) TestModelOperatorUpgrade(c *tc.C) {
	var (
		operatorName = modelOperatorName
		oldImagePath = fmt.Sprintf("%s/%s:9.9.8", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
		newImagePath = fmt.Sprintf("%s/%s:9.9.9", podcfg.JujudOCINamespace, podcfg.JujudOCIName)
	)

	_, err := s.broker.Client().AppsV1().Deployments(s.broker.Namespace()).Create(context.Background(),
		&apps.Deployment{
			ObjectMeta: meta.ObjectMeta{
				Name: operatorName,
			},
			Spec: apps.DeploymentSpec{
				Selector: &meta.LabelSelector{
					MatchLabels: map[string]string{
						"match-label": "true",
					},
				},
				Template: core.PodTemplateSpec{
					Spec: core.PodSpec{
						Containers: []core.Container{
							{
								Name:  "jujud",
								Image: oldImagePath,
							},
						},
					},
				},
			},
		}, meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(modelOperatorUpgrade(context.Background(), operatorName, semversion.MustParse("9.9.9"), s.broker), tc.ErrorIsNil)
	de, err := s.broker.Client().AppsV1().Deployments(s.broker.Namespace()).
		Get(context.Background(), operatorName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(de.Spec.Template.Spec.Containers[0].Image, tc.Equals, newImagePath)

	c.Assert(de.Annotations[utils.AnnotationVersionKey(1)], tc.Equals, semversion.MustParse("9.9.9").String())
	c.Assert(de.Spec.Template.Annotations[utils.AnnotationVersionKey(1)], tc.Equals, semversion.MustParse("9.9.9").String())
}
