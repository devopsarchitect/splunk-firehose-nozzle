package eventrouter

import (
	"fmt"

	"github.com/cloudfoundry-community/splunk-firehose-nozzle/cache"
	fevents "github.com/cloudfoundry-community/splunk-firehose-nozzle/events"
	"github.com/cloudfoundry-community/splunk-firehose-nozzle/eventsink"
	"github.com/cloudfoundry/sonde-go/events"
)

type Config struct {
	SelectedEvents string
	ExtraFields    string
}

type router struct {
	appCache       cache.Cache
	sink           eventsink.Sink
	selectedEvents map[string]bool
	extraFields    map[string]string
}

func New(appCache cache.Cache, sink eventsink.Sink, config *Config) (Router, error) {
	selectedEvents, err := fevents.ParseSelectedEvents(config.SelectedEvents)
	if err != nil {
		return nil, err
	}

	extraFields, err := fevents.ParseExtraFields(config.ExtraFields)
	if err != nil {
		return nil, err
	}

	return &router{
		appCache:       appCache,
		sink:           sink,
		selectedEvents: selectedEvents,
		extraFields:    extraFields,
	}, nil
}

func (r *router) Route(msg *events.Envelope) error {
	eventType := msg.GetEventType()

	if _, ok := r.selectedEvents[eventType.String()]; !ok {
		// Ignore this event since we are not interested
		return nil
	}

	var event *fevents.Event
	switch eventType {
	case events.Envelope_HttpStartStop:
		event = fevents.HttpStartStop(msg)
	case events.Envelope_LogMessage:
		event = fevents.LogMessage(msg)
	case events.Envelope_ValueMetric:
		event = fevents.ValueMetric(msg)
	case events.Envelope_CounterEvent:
		event = fevents.CounterEvent(msg)
	case events.Envelope_Error:
		event = fevents.ErrorEvent(msg)
	case events.Envelope_ContainerMetric:
		event = fevents.ContainerMetric(msg)
	}

	event.AnnotateWithEnveloppeData(msg)
	event.AnnotateWithMetaData(r.extraFields)

	if _, hasAppId := event.Fields["cf_app_id"]; hasAppId {
		event.AnnotateWithAppData(r.appCache)
	}

	if ignored, ok := event.Fields["cf_ignored_app"]; ok {
		if ignoreApp, ok := ignored.(bool); ok && ignoreApp {
			// Ignore events from this app since end user tag to ignore this app
			return nil
		}
	}

	err := r.sink.Write(event.Fields, event.Msg)
	if err != nil {
		fields := map[string]interface{}{"err": fmt.Sprintf("%s", err)}
		r.sink.Write(fields, "Failed to write events")
	}
	return err
}
