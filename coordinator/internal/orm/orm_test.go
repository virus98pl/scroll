package orm

import (
	"context"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/scroll-tech/go-ethereum/log"
	"io"
	"math/big"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"scroll-tech/common/database"
	"scroll-tech/common/docker"
	"scroll-tech/common/types"
	"scroll-tech/common/utils"

	"scroll-tech/database/migrate"
)

var (
	base *docker.App

	db            *gorm.DB
	proverTaskOrm *ProverTask
)

func TestMain(m *testing.M) {
	t := &testing.T{}
	setupEnv(t)
	defer tearDownEnv(t)
	m.Run()
}

func setupEnv(t *testing.T) {
	base = docker.NewDockerApp()
	base.RunDBImage(t)
	var err error
	db, err = database.InitDB(
		&database.Config{
			DSN:        base.DBConfig.DSN,
			DriverName: base.DBConfig.DriverName,
			MaxOpenNum: base.DBConfig.MaxOpenNum,
			MaxIdleNum: base.DBConfig.MaxIdleNum,
		},
	)
	assert.NoError(t, err)
	sqlDB, err := db.DB()
	assert.NoError(t, err)
	assert.NoError(t, migrate.ResetDB(sqlDB))

	proverTaskOrm = NewProverTask(db)
}

func tearDownEnv(t *testing.T) {
	sqlDB, err := db.DB()
	assert.NoError(t, err)
	sqlDB.Close()
	base.Free()
}

func TestProverTaskOrm(t *testing.T) {
	output := io.Writer(os.Stderr)
	usecolor := (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
	if usecolor {
		output = colorable.NewColorableStderr()
	}
	ostream := log.StreamHandler(output, log.TerminalFormat(usecolor))
	// show the call file and line number
	glogger := log.NewGlogHandler(ostream)
	// Set log level
	glogger.Verbosity(log.Lvl(4))
	log.Root().SetHandler(glogger)

	sqlDB, err := db.DB()
	assert.NoError(t, err)
	assert.NoError(t, migrate.ResetDB(sqlDB))

	reward := big.NewInt(0)
	reward.SetString("18446744073709551616", 10) // 1 << 64, uint64 maximum 1<<64 -1

	taskUUID, err := uuid.NewRandom()
	assert.NoError(t, err)

	proverTask := ProverTask{
		UUID:            taskUUID.String(),
		TaskID:          "test-hash",
		ProverName:      "prover-0",
		ProverPublicKey: "0",
		ProvingStatus:   int16(types.ProverAssigned),
		Reward:          decimal.NewFromBigInt(reward, 0),
		AssignedAt:      utils.NowUTC(),
	}

	err = proverTaskOrm.InsertProverTask(context.Background(), &proverTask)
	assert.NoError(t, err)
	proverTasks, err := proverTaskOrm.GetProverTasksByHashes(context.Background(), []string{"test-hash"})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(proverTasks))
	assert.Equal(t, proverTask.ProverName, proverTasks[0].ProverName)

	// test decimal reward, get reward
	resultReward := proverTasks[0].Reward.BigInt()
	assert.Equal(t, resultReward, reward)
	assert.Equal(t, resultReward.String(), "18446744073709551616")

	taskUUID2, err2 := uuid.NewRandom()
	assert.NoError(t, err2)
	proverTask.UUID = taskUUID2.String()
	proverTask.ProvingStatus = int16(types.ProverProofValid)
	proverTask.AssignedAt = utils.NowUTC()
	err = proverTaskOrm.InsertProverTask(context.Background(), &proverTask)
	assert.Error(t, err)
}

func TestProverTaskOrmUint256(t *testing.T) {
	sqlDB, err := db.DB()
	assert.NoError(t, err)
	assert.NoError(t, migrate.ResetDB(sqlDB))

	// test reward for uint256 maximum 1 << 256 -1 :115792089237316195423570985008687907853269984665640564039457584007913129639935
	rewardUint256 := big.NewInt(0)
	rewardUint256.SetString("115792089237316195423570985008687907853269984665640564039457584007913129639935", 10)
	taskUUID, err := uuid.NewRandom()
	assert.NoError(t, err)
	proverTask := ProverTask{
		UUID:            taskUUID.String(),
		TaskID:          "test-hash",
		ProverName:      "prover-0",
		ProverPublicKey: "0",
		ProvingStatus:   int16(types.ProverAssigned),
		Reward:          decimal.NewFromBigInt(rewardUint256, 0),
		AssignedAt:      utils.NowUTC(),
	}

	err = proverTaskOrm.InsertProverTask(context.Background(), &proverTask)
	assert.NoError(t, err)
	proverTasksUint256, err := proverTaskOrm.GetProverTasksByHashes(context.Background(), []string{"test-hash"})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(proverTasksUint256))
	resultRewardUint256 := proverTasksUint256[0].Reward.BigInt()
	assert.Equal(t, resultRewardUint256, rewardUint256)
	assert.Equal(t, resultRewardUint256.String(), "115792089237316195423570985008687907853269984665640564039457584007913129639935")
}
