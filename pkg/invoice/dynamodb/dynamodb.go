package dynamodb

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/awserr"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/dynamodbiface"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"

	commonpb "github.com/kinecosystem/kin-api/genproto/common/v3"

	"github.com/kinecosystem/agora-transaction-services/pkg/invoice"
)

type db struct {
	db dynamodbiface.ClientAPI
}

// New returns a dynamo-backed invoice.Store
func New(client dynamodbiface.ClientAPI) invoice.Store {
	return &db{
		db: client,
	}
}

// Add implements invoice.Store.Add.
func (d *db) Add(ctx context.Context, inv *commonpb.Invoice, txHash []byte) error {
	item, err := toItem(inv, txHash)
	if err != nil {
		return err
	}

	_, err = d.db.PutItemRequest(&dynamodb.PutItemInput{
		TableName:           tableNameStr,
		Item:                item,
		ConditionExpression: putConditionStr,
	}).Send(ctx)
	if err != nil {
		if aErr, ok := err.(awserr.Error); ok {
			switch aErr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				return invoice.ErrExists
			}
		}

		return errors.Wrapf(err, "failed to store invoice")
	}

	return nil
}

// Get implements invoice.Store.Get.
func (d *db) Get(ctx context.Context, prefix []byte, txHash []byte) (*commonpb.Invoice, error) {
	if len(prefix) != 29 {
		return nil, errors.Errorf("invalid invoice hash prefix len: %d", len(prefix))
	}

	if len(txHash) != 32 {
		return nil, errors.Errorf("invalid transaction hash len: %d", len(txHash))
	}

	resp, err := d.db.GetItemRequest(&dynamodb.GetItemInput{
		TableName: tableNameStr,
		Key: map[string]dynamodb.AttributeValue{
			tableHashKey: {
				B: prefix,
			},
			tableRangeKey: {
				B: txHash,
			},
		},
	}).Send(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get invoice")
	}

	if len(resp.Item) == 0 {
		return nil, invoice.ErrNotFound
	}
	return fromItem(resp.Item)
}

func (d *db) DoesNotExist(ctx context.Context, inv *commonpb.Invoice) error {
	prefix, err := invoice.GetHashPrefix(inv)
	if err != nil {
		return errors.Wrap(err, "failed to get invoice prefix")
	}

	input := &dynamodb.QueryInput{
		TableName:              tableNameStr,
		KeyConditionExpression: aws.String("prefix = :prefix"),
		ExpressionAttributeValues: map[string]dynamodb.AttributeValue{
			":prefix": {B: prefix},
		},
	}

	for {
		resp, err := d.db.QueryRequest(input).Send(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to query invoices")
		}

		if len(resp.Items) == 0 {
			return nil
		}

		for _, item := range resp.Items {
			storedInv, err := fromItem(item)
			if err != nil {
				return err
			}
			if proto.Equal(storedInv, inv) {
				return invoice.ErrExists
			}
		}

		if resp.LastEvaluatedKey != nil {
			input.ExclusiveStartKey = resp.LastEvaluatedKey
		} else {
			break
		}
	}
	return nil
}