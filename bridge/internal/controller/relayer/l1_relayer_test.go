package relayer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"scroll-tech/common/types"
	"scroll-tech/common/utils"

	"scroll-tech/bridge/internal/controller/sender"
	"scroll-tech/bridge/internal/orm"
	"scroll-tech/bridge/internal/orm/migrate"
	bridgeUtils "scroll-tech/bridge/internal/utils"
)

var (
	templateL1Message = []*orm.L1Message{
		{
			QueueIndex: 1,
			MsgHash:    "msg_hash1",
			Height:     1,
			Sender:     "0x596a746661dbed76a84556111c2872249b070e15",
			Value:      "0x19ece",
			GasLimit:   11529940,
			Target:     "0x2c73620b223808297ea734d946813f0dd78eb8f7",
			Calldata:   "testdata",
			Layer1Hash: "hash0",
		},
		{
			QueueIndex: 2,
			MsgHash:    "msg_hash2",
			Height:     2,
			Sender:     "0x596a746661dbed76a84556111c2872249b070e15",
			Value:      "0x19ece",
			GasLimit:   11529940,
			Target:     "0x2c73620b223808297ea734d946813f0dd78eb8f7",
			Calldata:   "testdata",
			Layer1Hash: "hash1",
		},
	}
)

func setupL1RelayerDB(t *testing.T) *gorm.DB {
	db, err := bridgeUtils.InitDB(cfg.DBConfig)
	assert.NoError(t, err)
	sqlDB, err := db.DB()
	assert.NoError(t, err)
	assert.NoError(t, migrate.ResetDB(sqlDB))
	return db
}

// testCreateNewRelayer test create new relayer instance and stop
func testCreateNewL1Relayer(t *testing.T) {
	db := setupL1RelayerDB(t)
	defer bridgeUtils.CloseDB(db)
	relayer, err := NewLayer1Relayer(context.Background(), db, cfg.L2Config.RelayerConfig)
	assert.NoError(t, err)
	assert.NotNil(t, relayer)
}

func testL1RelayerProcessSaveEvents(t *testing.T) {
	db := setupL1RelayerDB(t)
	defer bridgeUtils.CloseDB(db)
	l1MessageOrm := orm.NewL1Message(db)
	l1Cfg := cfg.L1Config
	relayer, err := NewLayer1Relayer(context.Background(), db, l1Cfg.RelayerConfig)
	assert.NoError(t, err)
	assert.NoError(t, l1MessageOrm.SaveL1Messages(context.Background(), templateL1Message))
	relayer.ProcessSavedEvents()
	msg1, err := l1MessageOrm.GetL1MessageByQueueIndex(1)
	assert.NoError(t, err)
	assert.Equal(t, msg1.Status, types.MsgSubmitted)
	msg2, err := l1MessageOrm.GetL1MessageByQueueIndex(2)
	assert.NoError(t, err)
	assert.Equal(t, msg2.Status, types.MsgSubmitted)
}

func testL1RelayerMsgConfirm(t *testing.T) {
	db := setupL1RelayerDB(t)
	defer bridgeUtils.CloseDB(db)
	l1MessageOrm := orm.NewL1Message(db)
	l1Messages := []*orm.L1Message{
		{MsgHash: "msg-1", QueueIndex: 0},
		{MsgHash: "msg-2", QueueIndex: 1},
	}
	err := l1MessageOrm.SaveL1Messages(context.Background(), l1Messages)
	assert.NoError(t, err)
	// Create and set up the Layer1 Relayer.
	l1Cfg := cfg.L1Config
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l1Relayer, err := NewLayer1Relayer(ctx, db, l1Cfg.RelayerConfig)
	assert.NoError(t, err)

	// Simulate message confirmations.
	l1Relayer.messageSender.SendConfirmation(&sender.Confirmation{
		ID:           "msg-1",
		IsSuccessful: true,
	})
	l1Relayer.messageSender.SendConfirmation(&sender.Confirmation{
		ID:           "msg-2",
		IsSuccessful: false,
	})

	// Check the database for the updated status using TryTimes.
	ok := utils.TryTimes(5, func() bool {
		msg1, err1 := l1MessageOrm.GetL1MessageByMsgHash("msg-1")
		msg2, err2 := l1MessageOrm.GetL1MessageByMsgHash("msg-2")
		return err1 == nil && types.MsgStatus(msg1.Status) == types.MsgConfirmed &&
			err2 == nil && types.MsgStatus(msg2.Status) == types.MsgRelayFailed
	})
	assert.True(t, ok)
}

func testL1RelayerGasOracleConfirm(t *testing.T) {
	db := setupL1RelayerDB(t)
	defer bridgeUtils.CloseDB(db)
	l1BlockOrm := orm.NewL1Block(db)

	l1Block := []orm.L1Block{
		{Hash: "gas-oracle-1", Number: 0},
		{Hash: "gas-oracle-2", Number: 1},
	}
	// Insert test data.
	assert.NoError(t, l1BlockOrm.InsertL1Blocks(context.Background(), l1Block))

	// Create and set up the Layer2 Relayer.
	l1Cfg := cfg.L1Config
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l1Relayer, err := NewLayer1Relayer(ctx, db, l1Cfg.RelayerConfig)
	assert.NoError(t, err)

	// Simulate message confirmations.
	l1Relayer.gasOracleSender.SendConfirmation(&sender.Confirmation{
		ID:           "gas-oracle-1",
		IsSuccessful: true,
	})
	l1Relayer.gasOracleSender.SendConfirmation(&sender.Confirmation{
		ID:           "gas-oracle-2",
		IsSuccessful: false,
	})

	// Check the database for the updated status using TryTimes.
	ok := utils.TryTimes(5, func() bool {
		msg1, err1 := l1BlockOrm.GetL1BlockInfos(map[string]interface{}{"hash": "gas-oracle-1"})
		msg2, err2 := l1BlockOrm.GetL1BlockInfos(map[string]interface{}{"hash": "gas-oracle-2"})
		return err1 == nil && len(msg1) == 1 && types.GasOracleStatus(msg1[0].GasOracleStatus) == types.GasOracleImported &&
			err2 == nil && len(msg2) == 1 && types.GasOracleStatus(msg2[0].GasOracleStatus) == types.GasOracleFailed
	})
	assert.True(t, ok)
}