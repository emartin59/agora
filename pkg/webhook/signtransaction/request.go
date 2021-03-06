package signtransaction

import (
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	transactionpb "github.com/kinecosystem/agora-api/genproto/transaction/v3"
)

// RequestBody contains the body of a sign transaction request.
type RequestBody struct {
	// EnvelopeXDR is a base64-encoded transaction envelope XDR
	EnvelopeXDR []byte `json:"envelope_xdr"`
	// InvoiceList is a base64-encoded protobuf InvoiceList
	InvoiceList []byte `json:"invoice_list,omitempty"`
}

func RequestBodyFromProto(req *transactionpb.SubmitTransactionRequest) (*RequestBody, error) {
	reqBody := &RequestBody{
		EnvelopeXDR: req.EnvelopeXdr,
	}

	if req.InvoiceList != nil {
		b, err := proto.Marshal(req.InvoiceList)
		if err != nil {
			return nil, errors.New("failed to marshal invoice list")
		}
		reqBody.InvoiceList = b
	}

	return reqBody, nil
}
