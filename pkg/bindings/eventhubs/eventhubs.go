package eventhubs

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/gob"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"

	eventhub "github.com/Azure/azure-event-hubs-go"
	log "github.com/Sirupsen/logrus"
	"github.com/actionscore/actions/pkg/components/bindings"
)

// AzureEventHubs allows sending/receiving Azure Event Hubs events
type AzureEventHubs struct {
	Spec bindings.Metadata
}

// AzureEventHubsMetadata is Azure Event Hubs connection metadata
type AzureEventHubsMetadata struct {
	ConnectionString string `json:"connectionString"`
}

// NewAzureEventHubs returns a new Azure Event hubs instance
func NewAzureEventHubs() *AzureEventHubs {
	return &AzureEventHubs{}
}

// Init performs metadata init
func (a *AzureEventHubs) Init(metadata bindings.Metadata) error {
	a.Spec = metadata
	return nil
}

// GetBytes turns an interface{} to a byte array representation
func GetBytes(key interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(key)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Write posts an event hubs message
func (a *AzureEventHubs) Write(req *bindings.WriteRequest) error {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	b, err := json.Marshal(a.Spec.ConnectionInfo)
	if err != nil {
		return err
	}

	var connInfo AzureEventHubsMetadata
	err = json.Unmarshal(b, &connInfo)
	if err != nil {
		return err
	}

	connStr := connInfo.ConnectionString

	hub, err := eventhub.NewHubFromConnectionString(connStr)
	if err != nil {
		return err
	}

	err = hub.Send(context.Background(), &eventhub.Event{
		Data: req.Data,
	})
	if err != nil {
		return err
	}

	log.Info("EventHubs event sent successfully")
	return nil
}

// Read reads from eventhubs in a non-blocking fashion
func (a *AzureEventHubs) Read(handler func(*bindings.ReadResponse) error) error {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	b, err := json.Marshal(a.Spec.ConnectionInfo)
	if err != nil {
		return err
	}

	var connInfo AzureEventHubsMetadata
	err = json.Unmarshal(b, &connInfo)
	if err != nil {
		return err
	}

	connStr := connInfo.ConnectionString

	hub, err := eventhub.NewHubFromConnectionString(connStr)
	if err != nil {
		return err
	}

	callback := func(c context.Context, event *eventhub.Event) error {
		if event != nil {
			handler(&bindings.ReadResponse{
				Data: event.Data,
			})
		}

		return nil
	}

	ctx := context.Background()
	runtimeInfo, err := hub.GetRuntimeInformation(ctx)
	if err != nil {
		return err
	}

	for _, partitionID := range runtimeInfo.PartitionIDs {
		_, err := hub.Receive(ctx, partitionID, callback, eventhub.ReceiveWithLatestOffset())
		if err != nil {
			return err
		}
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill)
	<-signalChan

	hub.Close(context.Background())

	return nil
}