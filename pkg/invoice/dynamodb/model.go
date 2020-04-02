package dynamodb

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/dynamodbattribute"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	commonpb "github.com/kinecosystem/kin-api/genproto/common/v3"

	"github.com/kinecosystem/agora-transaction-services/pkg/invoice"
)

const (
	tableName    = "invoices"
	putCondition = "attribute_not_exists(prefix) AND attribute_not_exists(tx_hash)"

	tableHashKey  = "prefix"
	tableRangeKey = "tx_hash"
)

var (
	tableNameStr    = aws.String(tableName)
	putConditionStr = aws.String(putCondition)
)

type invoiceItem struct {
	Prefix   []byte `dynamodbav:"prefix"`
	TXHash   []byte `dynamodbav:"tx_hash"`
	Contents []byte `dynamodbav:"contents"`
}

func toItem(inv *commonpb.Invoice, txHash []byte) (map[string]dynamodb.AttributeValue, error) {
	if len(txHash) != 32 {
		return nil, errors.New("transaction hash must be 32 bytes")
	}

	prefix, err := invoice.GetHashPrefix(inv)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get invoice prefix")
	}

	b, err := proto.Marshal(inv)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal invoice")
	}

	return dynamodbattribute.MarshalMap(&invoiceItem{
		Prefix:   prefix,
		TXHash:   txHash,
		Contents: b,
	})
}

func fromItem(item map[string]dynamodb.AttributeValue) (*commonpb.Invoice, error) {
	var invoiceItem invoiceItem
	if err := dynamodbattribute.UnmarshalMap(item, &invoiceItem); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal invoice item")
	}

	inv := &commonpb.Invoice{}
	if err := proto.Unmarshal(invoiceItem.Contents, inv); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal invoice")
	}

	return inv, nil
}