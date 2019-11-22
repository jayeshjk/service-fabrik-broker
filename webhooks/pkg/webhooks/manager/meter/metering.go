package meter

import (
	// "encoding/json"
	"time"

	"github.com/cloudfoundry-incubator/service-fabrik-broker/webhooks/pkg/apis/instance/v1alpha1"
	c "github.com/cloudfoundry-incubator/service-fabrik-broker/webhooks/pkg/webhooks/manager/constants"
	"github.com/cloudfoundry-incubator/service-fabrik-broker/webhooks/pkg/webhooks/manager/resources"
	"github.com/golang/glog"
	"github.com/google/uuid"
)

// NewMetering creates a new Sfevent
func NewMetering(opt resources.GenericOptions, crd resources.GenericResource, startStop int, e c.EventType) *v1alpha1.Sfevent {
	si := v1alpha1.ServiceInfo{
		ID:   opt.ServiceID,
		Plan: opt.PlanID,
	}
	ci := v1alpha1.ConsumerInfo{
		Environment: "",
		Region:      "",
		Org:         opt.Context.OrganizationGUID,
		Space:       opt.Context.SpaceGUID,
		Instance:    crd.Name,
	}
	//Assing the environment
	switch opt.Context.Platform {
	case c.Cloudfoundry:
		ci.Environment = c.Cf
	default:
		ci.Environment = ""
	}
	im := v1alpha1.InstancesMeasure{
		ID:    c.MeasuresID,
		Value: startStop,
	}
	guid := uuid.New().String()

	mo := v1alpha1.SfeventOptions{
		ID:                guid,
		Timestamp:         time.Now().UTC().Format(c.MeteringTimestampFormat),
		ServiceInfo:       si,
		ConsumerInfo:      ci,
		InstancesMeasures: []v1alpha1.InstancesMeasure{im},
	}
	glog.Infof("New metering event for CRD: %s, Sfevent Id: %s", crd.Name, guid)
	m := &v1alpha1.Sfevent{
		Spec: v1alpha1.SfeventSpec{
			Options: mo,
		},
		Status: v1alpha1.SfeventStatus{
			State: c.ToBeMetered,
		},
	}
	m.SetName(guid)
	labels := make(map[string]string)
	labels[c.MeterStateKey] = c.ToBeMetered
	labels[c.InstanceGUIDKey] = ci.Instance
	labels[c.EventTypeKey] = string(e)
	m.SetLabels(labels)
	return m
}
