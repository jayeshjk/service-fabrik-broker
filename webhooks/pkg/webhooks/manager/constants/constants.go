package constants

const (
	// MeterStart signals the creation of an instance
	MeterStart = 1
	// MeterStop signals the deletion of an instance
	MeterStop = 0
	// ToBeMetered is the default state of metering resource
	ToBeMetered = "TO_BE_METERED"
	// Cloudfoundry : string representing cloudfoundry platform
	// in last operation
	Cloudfoundry = "cloudfoundry"
	// Cf : string representing cloudfoundry platform in metering document
	Cf = "CF"
	// MeasuresID : the name of value being measured in metering doc
	MeasuresID = "instances"
	// MeteringTimestampFormat time format expected by MaaS
	// Maas expects timestamp in 'yyyy-MM-dd'T'HH:mm:ss.SSS'
	// Go has wierd time formating rules !!
	// https://golang.org/src/time/format.go
	MeteringTimestampFormat = "2006-01-02T15:04:05.000"
	// MeterStateKey : key used to store meter state
	MeterStateKey = "state"
	// InstanceAPIVersion : Api version of instance CRD
	InstanceAPIVersion = "instance.servicefabrik.io/v1alpha1"
	// DefaultNamespace : the default namespace used by the Apiserver
	DefaultNamespace = "default"
	// InstanceGUIDKey : The key used to store instance guid in CRD labels
	InstanceGUIDKey = "instance_guid"
	// EventTypeKey :
	EventTypeKey = "event_type"
)

// EventType denotes the types of metering events
type EventType string

const (
	//UpdateEvent signals the update of an instance
	UpdateEvent EventType = "update"
	//CreateEvent signals the create of an instance
	CreateEvent EventType = "create"
	//DeleteEvent signals the delete of an instance
	DeleteEvent EventType = "delete"
	//InvalidEvent is not yet supported
	InvalidEvent EventType = "default"
)
