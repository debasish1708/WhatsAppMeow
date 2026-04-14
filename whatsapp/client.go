package whatsapp

import (
	"context"

	_ "modernc.org/sqlite"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type WhatsAppAdapter struct {
	Client     *whatsmeow.Client
	Container  *sqlstore.Container
	Dispatcher *EventDispatcher
}

func NewWhatsAppAdapter(dsn string) (*WhatsAppAdapter, error) {
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New(context.Background(), "sqlite", dsn, dbLog)
	if err != nil {
		return nil, err
	}

	adapter := &WhatsAppAdapter{
		Container: container,
	}

	adapter.InitClient()
	return adapter, nil
}

func (wa *WhatsAppAdapter) InitClient() {
	deviceStore, err := wa.Container.GetFirstDevice(context.Background())
	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("Client", "INFO", true)
	wa.Client = whatsmeow.NewClient(deviceStore, clientLog)
	
	if wa.Dispatcher != nil {
		wa.Client.AddEventHandler(wa.Dispatcher.HandleEvent)
	}
}

func (wa *WhatsAppAdapter) Connect() error {
	if wa.Client.Store.ID != nil {
		return wa.Client.Connect()
	}
	return nil
}

func (wa *WhatsAppAdapter) Disconnect() {
	wa.Client.Disconnect()
}