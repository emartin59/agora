package events

import (
	"context"
	"encoding/hex"
	"encoding/json"

	"github.com/golang/protobuf/proto"
	"github.com/kinecosystem/agora-common/kin"
	"github.com/kinecosystem/agora-common/taskqueue"
	"github.com/kinecosystem/agora-common/taskqueue/model/task"
	"github.com/kinecosystem/go/xdr"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/kinecosystem/agora/pkg/app"
	"github.com/kinecosystem/agora/pkg/invoice"
	"github.com/kinecosystem/agora/pkg/transaction/history/model"
	"github.com/kinecosystem/agora/pkg/webhook"
)

const (
	// IngestionQueueName is the events ingestion queue name
	IngestionQueueName = "events-ingestion-queue"
)

// Processor processes transactions as a history.Writer, and notifies
// webhooks about said transactions via a taskqueue.
type Processor struct {
	log *logrus.Entry

	submitter taskqueue.Submitter

	invoiceStore  invoice.Store
	configStore   app.ConfigStore
	webhookClient *webhook.Client
}

func NewProcessor(
	eventsQueueCtor taskqueue.ProcessorCtor,
	invoiceStore invoice.Store,
	configStore app.ConfigStore,
	webhookClient *webhook.Client,
) (p *Processor, err error) {
	p = &Processor{
		log:           logrus.StandardLogger().WithField("type", "events/Processor"),
		invoiceStore:  invoiceStore,
		configStore:   configStore,
		webhookClient: webhookClient,
	}

	p.submitter, err = eventsQueueCtor(p.queueHandler)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// Write implements history.Writer.Write.
//
// Write forwards model.Entry's to a taskqueue, where other processors
// will attempt to notify any webhooks that might be interested in the
// transaction.
func (p *Processor) Write(ctx context.Context, entry *model.Entry) error {
	b, err := proto.Marshal(entry)
	if err != nil {
		return errors.Wrap(err, "failed to marshal entry")
	}

	return p.submitter.Submit(ctx, &task.Message{
		TypeName: proto.MessageName(entry),
		RawValue: b,
	})
}

// queueHandler is the taskqueue.Handler that attempts to call any
// webhooks that might be interested.
func (p *Processor) queueHandler(ctx context.Context, task *task.Message) error {
	log := p.log.WithFields(logrus.Fields{
		"method": "handler",
	})

	entry := &model.Entry{}

	if task.TypeName != proto.MessageName(entry) {
		log.WithField("type_name", task.TypeName).Warn("Unsupported message type")
		return errors.New("unsupported message type")
	}

	if err := proto.Unmarshal(task.RawValue, entry); err != nil {
		log.WithError(err).Warn("Failed to unmarshal entry")
		return errors.Wrap(err, "failed to unmarshal entry")
	}

	txHash, err := entry.GetTxHash()
	if err != nil {
		log.WithError(err).Warn("Failed to get tx hash from entry")
		return errors.Wrap(err, "failed to get tx hash from entry")
	}

	log = log.WithField("tx_hash", hex.EncodeToString(txHash))

	il, err := p.invoiceStore.Get(ctx, txHash)
	if err != nil && err != invoice.ErrNotFound {
		log.WithError(err).Warn("Failed to get invoice list")
		return errors.Wrapf(err, "failed to get invoice for tx: %x", txHash)
	}

	appIndex := -1
	event := Event{
		TransactionEvent: &TransactionEvent{
			KinVersion:  int(entry.Version),
			TxHash:      txHash,
			InvoiceList: il,
		},
	}

	switch t := entry.Kind.(type) {
	case *model.Entry_Stellar:
		event.TransactionEvent.StellarEvent = &StellarData{
			ResultXDR:   t.Stellar.ResultXdr,
			EnvelopeXDR: t.Stellar.EnvelopeXdr,
		}

		var envelope xdr.TransactionEnvelope
		if err := envelope.UnmarshalBinary(t.Stellar.EnvelopeXdr); err != nil {
			log.WithError(err).Warn("Failed to unmarshal envelope")
			return errors.Wrap(err, "failed to unmarshal envelope")
		}

		memo, ok := kin.MemoFromXDR(envelope.Tx.Memo, true)
		if ok {
			appIndex = int(memo.AppIndex())
		}
	default:
		log.Warn("unsupported entry type, ignoring")
		return errors.Errorf("unsupported entry type, ignoring")
	}

	if appIndex < 0 {
		log.WithField("tx_hash", hex.EncodeToString(txHash)).Trace("no app id present; dropping")
		return nil
	}

	log = log.WithField("app_index", appIndex)

	conf, err := p.configStore.Get(ctx, uint16(appIndex))
	if err == app.ErrNotFound {
		log.Trace("no app id configured; dropping")
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "failed to load config for %d", appIndex)
	}

	if conf.EventsURL == nil {
		log.Trace("app has no events url configured; dropping")
		return nil
	}

	events := []Event{event}
	body, err := json.Marshal(&events)
	if err != nil {
		log.WithError(err).Warn("Failed to marshal events body")
		return errors.Wrap(err, "failed to marshal events body")
	}

	if err := p.webhookClient.Events(ctx, *conf.EventsURL, conf.WebhookSecret, body); err != nil {
		log.WithError(err).Warn("Failed to call events webhook")
		return err
	}

	return nil
}
