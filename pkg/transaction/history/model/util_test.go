package model

import (
	"strconv"
	"testing"

	"github.com/stellar/go/network"
	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kinecosystem/agora/pkg/testutil"
)

func TestStellar(t *testing.T) {
	accounts := make([]xdr.AccountId, 6)
	for i := 0; i < len(accounts); i++ {
		_, accounts[i] = testutil.GenerateAccountID(t)
	}

	_, src := testutil.GenerateAccountID(t)
	envelope := testutil.GenerateTransactionEnvelope(
		src,
		1,
		[]xdr.Operation{
			testutil.GenerateCreateOperation(&accounts[0], accounts[1]),
			testutil.GeneratePaymentOperation(&accounts[2], accounts[3]),
			testutil.GenerateMergeOperation(&accounts[4], accounts[5]),
		},
	)

	envelopeBytes, err := envelope.MarshalBinary()
	require.NoError(t, err)

	networkPassphrase := "network phassphrase"
	expected, err := network.HashTransaction(&envelope.Tx, networkPassphrase)
	require.NoError(t, err)

	e := Entry{
		Version: KinVersion_KIN3,
		Kind: &Entry_Stellar{
			Stellar: &StellarEntry{
				Ledger:            10,
				PagingToken:       1,
				NetworkPassphrase: networkPassphrase,
				EnvelopeXdr:       envelopeBytes,
			},
		},
	}

	envelopeAccounts, err := GetAccountsFromEnvelope(envelope)
	assert.NoError(t, err)
	assert.Len(t, envelopeAccounts, 1+len(accounts))
	for _, account := range append([]xdr.AccountId{src}, accounts...) {
		_, exists := envelopeAccounts[account.Address()]
		assert.True(t, exists)
	}

	// Hash
	actual, err := e.GetTxHash()
	assert.NoError(t, err)
	assert.EqualValues(t, expected[:], actual)

	// Accounts
	entryAccounts, err := e.GetAccounts()
	assert.NoError(t, err)
	assert.Equal(t, len(entryAccounts), len(envelopeAccounts))
	for _, account := range entryAccounts {
		_, exists := envelopeAccounts[account]
		assert.True(t, exists)
	}

	// Ordering Key
	for _, v := range []KinVersion{KinVersion_KIN2, KinVersion_KIN3} {
		e.Version = v

		k, err := e.GetOrderingKey()
		assert.NoError(t, err)

		pt := e.Kind.(*Entry_Stellar).Stellar.PagingToken
		cursor := strconv.FormatUint(pt, 10)

		actual, err := OrderingKeyFromCursor(v, cursor)
		assert.NoError(t, err)
		assert.EqualValues(t, actual, k)
	}
}
