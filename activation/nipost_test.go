package activation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/spacemeshos/go-scale/tester"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"

	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/signing"
)

func defaultPoetServiceMock(ctrl *gomock.Controller, id []byte, address string) *MockPoetClient {
	poetClient := NewMockPoetClient(ctrl)
	poetClient.EXPECT().
		Submit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(&types.PoetRound{}, nil)
	poetClient.EXPECT().PoetServiceID(gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context) (types.PoetServiceID, error) {
			return types.PoetServiceID{ServiceID: id}, ctx.Err()
		})
	poetClient.EXPECT().PowParams(gomock.Any()).AnyTimes().Return(&PoetPowParams{}, nil)
	poetClient.EXPECT().Address().AnyTimes().Return(address).AnyTimes()
	return poetClient
}

func defaultLayerClockMock(ctrl *gomock.Controller) *MocklayerClock {
	mclock := NewMocklayerClock(ctrl)
	mclock.EXPECT().LayerToTime(gomock.Any()).AnyTimes().DoAndReturn(
		func(got types.LayerID) time.Time {
			// time.Now() ~= currentLayer
			genesis := time.Now().Add(-time.Duration(postGenesisEpoch.FirstLayer()) * layerDuration)
			return genesis.Add(layerDuration * time.Duration(got))
		},
	)
	return mclock
}

func TestNIPostBuilderWithMocks(t *testing.T) {
	t.Parallel()

	challenge := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 2,
	}

	ctrl := gomock.NewController(t)

	poetProvider := defaultPoetServiceMock(ctrl, []byte("poet"), "http://localhost:9999")
	poetProvider.EXPECT().Proof(gomock.Any(), "").Return(&types.PoetProofMessage{
		PoetProof: types.PoetProof{},
	}, []types.Member{types.Member(challenge.Hash())}, nil)

	poetDb := NewMockpoetDbAPI(ctrl)
	poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil)
	mclock := defaultLayerClockMock(ctrl)

	sig, err := signing.NewEdSigner()
	require.NoError(t, err)

	postClient := NewMockPostClient(ctrl)
	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(&types.Post{}, &types.PostInfo{}, nil)
	postService := NewMockpostService(ctrl)
	postService.EXPECT().Client(sig.NodeID()).Return(postClient, nil)

	nb, err := NewNIPostBuilder(
		poetDb,
		postService,
		t.TempDir(),
		zaptest.NewLogger(t),
		sig,
		PoetConfig{},
		mclock,
		WithPoetClients(poetProvider),
	)
	require.NoError(t, err)

	certifier := NewMockcertifierService(ctrl)
	certifier.EXPECT().GetCertificate(poetProvider.Address()).Return(PoetCert("cert"))

	nipost, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
	require.NoError(t, err)
	require.NotNil(t, nipost)
}

func TestPostSetup(t *testing.T) {
	postProvider := newTestPostManager(t)
	challenge := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 2,
	}

	ctrl := gomock.NewController(t)
	poetProvider := defaultPoetServiceMock(ctrl, []byte("poet"), "http://localhost:9999")
	poetProvider.EXPECT().Proof(gomock.Any(), "").Return(&types.PoetProofMessage{
		PoetProof: types.PoetProof{},
	}, []types.Member{types.Member(challenge.Hash())}, nil)

	poetDb := NewMockpoetDbAPI(ctrl)
	poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil)
	mclock := defaultLayerClockMock(ctrl)

	postClient := NewMockPostClient(ctrl)
	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(&types.Post{}, &types.PostInfo{}, nil)
	postService := NewMockpostService(ctrl)
	postService.EXPECT().Client(postProvider.id).Return(postClient, nil)

	nb, err := NewNIPostBuilder(
		poetDb,
		postService,
		t.TempDir(),
		zaptest.NewLogger(t),
		postProvider.signer,
		PoetConfig{},
		mclock,
		WithPoetClients(poetProvider),
	)
	require.NoError(t, err)

	require.NoError(t, postProvider.PrepareInitializer(postProvider.opts))
	require.NoError(t, postProvider.StartSession(context.Background()))
	t.Cleanup(func() { assert.NoError(t, postProvider.Reset()) })

	certifier := NewMockcertifierService(ctrl)
	certifier.EXPECT().GetCertificate(poetProvider.Address()).Return(PoetCert("cert"))

	nipost, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
	require.NoError(t, err)
	require.NotNil(t, nipost)
}

func TestNIPostBuilder_BuildNIPost(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	challenge := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 2,
	}
	challenge2 := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 2,
		Sequence:     1,
	}
	challenge3 := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 2,
		Sequence:     2,
	}

	ctrl := gomock.NewController(t)
	poetProver := defaultPoetServiceMock(ctrl, []byte("poet"), "http://localhost:9999")
	poetProver.EXPECT().Proof(gomock.Any(), "").AnyTimes().Return(
		&types.PoetProofMessage{
			PoetProof: types.PoetProof{},
		}, []types.Member{
			types.Member(challenge.Hash()),
			types.Member(challenge2.Hash()),
			types.Member(challenge3.Hash()),
		}, nil,
	)

	poetDb := NewMockpoetDbAPI(ctrl)
	poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil)
	mclock := defaultLayerClockMock(ctrl)

	sig, err := signing.NewEdSigner()
	require.NoError(t, err)

	postClient := NewMockPostClient(ctrl)
	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(&types.Post{}, &types.PostInfo{}, nil).Times(1)
	postService := NewMockpostService(ctrl)
	postService.EXPECT().Client(sig.NodeID()).Return(postClient, nil).AnyTimes()

	nb, err := NewNIPostBuilder(
		poetDb,
		postService,
		dir,
		zaptest.NewLogger(t),
		sig,
		PoetConfig{},
		mclock,
		WithPoetClients(poetProver),
	)
	require.NoError(t, err)

	certifier := NewMockcertifierService(ctrl)
	certifier.EXPECT().GetCertificate(poetProver.Address()).AnyTimes().Return(PoetCert("cert"))
	nipost, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
	require.NoError(t, err)
	require.NotNil(t, nipost)

	poetDb = NewMockpoetDbAPI(ctrl)
	poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil)

	// fail post exec
	nb, err = NewNIPostBuilder(
		poetDb,
		postService,
		dir,
		zaptest.NewLogger(t),
		sig,
		PoetConfig{},
		mclock,
		WithPoetClients(poetProver),
	)
	require.NoError(t, err)

	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(nil, nil, fmt.Errorf("error")).Times(1)

	// check that proof ref is not called again
	nipost, err = nb.BuildNIPost(context.Background(), &challenge2, certifier)
	require.Nil(t, nipost)
	require.Error(t, err)

	// successful post exec
	poetDb = NewMockpoetDbAPI(ctrl)

	nb, err = NewNIPostBuilder(
		poetDb,
		postService,
		dir,
		zaptest.NewLogger(t),
		sig,
		PoetConfig{},
		mclock,
		WithPoetClients(poetProver),
	)
	require.NoError(t, err)
	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(&types.Post{}, &types.PostInfo{}, nil).Times(1)

	// check that proof ref is not called again
	nipost, err = nb.BuildNIPost(context.Background(), &challenge2, certifier)
	require.NoError(t, err)
	require.NotNil(t, nipost)

	// test state not loading if other challenge provided
	poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil)
	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(&types.Post{}, &types.PostInfo{}, nil).Times(1)
	nipost, err = nb.BuildNIPost(context.Background(), &challenge3, certifier)
	require.NoError(t, err)
	require.NotNil(t, nipost)
}

func TestNIPostBuilder_ManyPoETs_SubmittingChallenge_DeadlineReached(t *testing.T) {
	t.Parallel()
	// Arrange
	challenge := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 1,
	}

	proof := &types.PoetProofMessage{PoetProof: types.PoetProof{}}

	ctrl := gomock.NewController(t)
	poetDb := NewMockpoetDbAPI(ctrl)
	poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil)
	mclock := defaultLayerClockMock(ctrl)

	poets := make([]PoetClient, 0, 2)
	{
		poet := NewMockPoetClient(ctrl)
		poet.EXPECT().
			PoetServiceID(gomock.Any()).
			AnyTimes().
			Return(types.PoetServiceID{ServiceID: []byte("poet0")}, nil)
		poet.EXPECT().
			Submit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(
				ctx context.Context,
				_ time.Time,
				_, _ []byte,
				_ types.EdSignature,
				_ types.NodeID,
				_ PoetAuth,
			) (*types.PoetRound, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			})
		poet.EXPECT().Address().AnyTimes().Return("http://localhost:9999")
		poets = append(poets, poet)
	}
	{
		poet := NewMockPoetClient(ctrl)
		poet.EXPECT().
			PoetServiceID(gomock.Any()).
			AnyTimes().
			Return(types.PoetServiceID{ServiceID: []byte("poet1")}, nil)
		poet.EXPECT().
			Submit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.PoetRound{}, nil)
		poet.EXPECT().
			Proof(gomock.Any(), gomock.Any()).
			Return(proof, []types.Member{types.Member(challenge.Hash())}, nil)
		poet.EXPECT().Address().AnyTimes().Return("http://localhost:9998")
		poets = append(poets, poet)
	}

	poetCfg := PoetConfig{
		PhaseShift: layerDuration * layersPerEpoch / 2,
	}

	sig, err := signing.NewEdSigner()
	require.NoError(t, err)

	postClient := NewMockPostClient(ctrl)
	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(&types.Post{}, &types.PostInfo{}, nil)
	postService := NewMockpostService(ctrl)
	postService.EXPECT().Client(sig.NodeID()).Return(postClient, nil)
	nb, err := NewNIPostBuilder(
		poetDb,
		postService,
		t.TempDir(),
		zaptest.NewLogger(t),
		sig,
		poetCfg,
		mclock,
		WithPoetClients(poets...),
	)
	require.NoError(t, err)

	certifier := NewMockcertifierService(ctrl)
	certifier.EXPECT().GetCertificate(poets[0].Address()).Return(PoetCert("cert"))
	certifier.EXPECT().GetCertificate(poets[1].Address()).Return(PoetCert("cert"))

	// Act
	nipost, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
	require.NoError(t, err)

	// Verify
	ref, _ := proof.Ref()
	require.Equal(t, ref[:], nipost.PostMetadata.Challenge)
}

func TestNIPostBuilder_ManyPoETs_AllFinished(t *testing.T) {
	t.Parallel()

	// Arrange
	challenge := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 2,
	}

	proofWorse := &types.PoetProofMessage{
		PoetProof: types.PoetProof{
			LeafCount: 111,
		},
	}
	proofBetter := &types.PoetProofMessage{
		PoetProof: types.PoetProof{
			LeafCount: 999,
		},
	}

	ctrl := gomock.NewController(t)
	poetDb := NewMockpoetDbAPI(ctrl)
	poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Times(2).Return(nil)
	mclock := defaultLayerClockMock(ctrl)

	poets := make([]PoetClient, 0, 2)
	{
		poet := NewMockPoetClient(ctrl)
		poet.EXPECT().
			Submit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.PoetRound{}, nil)
		poet.EXPECT().PoetServiceID(gomock.Any()).AnyTimes().Return(types.PoetServiceID{ServiceID: []byte("poet0")}, nil)
		poet.EXPECT().Address().AnyTimes().Return("http://localhost:9999").AnyTimes()
		poet.EXPECT().Proof(gomock.Any(), "").Return(proofWorse, []types.Member{types.Member(challenge.Hash())}, nil)
		poets = append(poets, poet)
	}
	{
		poet := defaultPoetServiceMock(ctrl, []byte("poet1"), "http://localhost:9998")
		poet.EXPECT().Proof(gomock.Any(), "").Return(proofBetter, []types.Member{types.Member(challenge.Hash())}, nil)
		poets = append(poets, poet)
	}

	sig, err := signing.NewEdSigner()
	require.NoError(t, err)

	postClient := NewMockPostClient(ctrl)
	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(&types.Post{}, &types.PostInfo{}, nil)
	postService := NewMockpostService(ctrl)
	postService.EXPECT().Client(sig.NodeID()).Return(postClient, nil)

	nb, err := NewNIPostBuilder(
		poetDb,
		postService,
		t.TempDir(),
		zaptest.NewLogger(t),
		sig,
		PoetConfig{},
		mclock,
		WithPoetClients(poets...),
	)
	require.NoError(t, err)

	certifier := NewMockcertifierService(ctrl)
	certifier.EXPECT().GetCertificate(poets[0].Address()).Return(PoetCert("cert"))
	// No certs - fallback to PoW
	certifier.EXPECT().GetCertificate(poets[1].Address()).Return(nil)

	// Act
	nipost, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
	require.NoError(t, err)

	// Verify
	ref, _ := proofBetter.Ref()
	require.Equal(t, ref[:], nipost.PostMetadata.Challenge)
}

func TestNIPSTBuilder_PoetUnstable(t *testing.T) {
	t.Parallel()
	challenge := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 1,
	}
	poetCfg := PoetConfig{
		PhaseShift: layerDuration,
	}
	cert := PoetCert("cert")
	sig, err := signing.NewEdSigner()
	require.NoError(t, err)

	t.Run("PoetServiceID fails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		poetDb := NewMockpoetDbAPI(ctrl)
		mclock := defaultLayerClockMock(ctrl)
		poetProver := NewMockPoetClient(ctrl)
		poetProver.EXPECT().PoetServiceID(gomock.Any()).Return(types.PoetServiceID{}, errors.New("test"))
		poetProver.EXPECT().Address().Return("http://localhost:9999")
		postService := NewMockpostService(ctrl)

		nb, err := NewNIPostBuilder(
			poetDb,
			postService,
			t.TempDir(),
			zaptest.NewLogger(t),
			sig,
			poetCfg,
			mclock,
			WithPoetClients(poetProver),
		)
		require.NoError(t, err)

		certifier := NewMockcertifierService(ctrl)
		nipst, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
		require.ErrorIs(t, err, ErrPoetServiceUnstable)
		require.Nil(t, nipst)
	})
	t.Run("Submit fails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		poetDb := NewMockpoetDbAPI(ctrl)
		mclock := defaultLayerClockMock(ctrl)
		poetProver := NewMockPoetClient(ctrl)
		poetProver.EXPECT().PoetServiceID(gomock.Any()).Return(types.PoetServiceID{}, nil)
		poetProver.EXPECT().
			Submit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), sig.NodeID(), PoetAuth{PoetCert: cert}).
			Return(nil, errors.New("test"))
		poetProver.EXPECT().Address().AnyTimes().Return("http://localhost:9999")
		postService := NewMockpostService(ctrl)

		nb, err := NewNIPostBuilder(
			poetDb,
			postService,
			t.TempDir(),
			zaptest.NewLogger(t),
			sig,
			poetCfg,
			mclock,
			WithPoetClients(poetProver),
		)
		require.NoError(t, err)

		certifier := NewMockcertifierService(ctrl)
		certifier.EXPECT().GetCertificate("http://localhost:9999").Return(cert)

		nipst, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
		require.ErrorIs(t, err, ErrPoetServiceUnstable)
		require.Nil(t, nipst)
	})
	t.Run("Submit hangs", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		poetDb := NewMockpoetDbAPI(ctrl)
		mclock := defaultLayerClockMock(ctrl)
		poetProver := NewMockPoetClient(ctrl)
		poetProver.EXPECT().PoetServiceID(gomock.Any()).AnyTimes().Return(types.PoetServiceID{}, nil)
		poetProver.EXPECT().
			Submit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), sig.NodeID(), PoetAuth{PoetCert: cert}).
			DoAndReturn(func(
				ctx context.Context,
				_ time.Time,
				_, _ []byte,
				_ types.EdSignature,
				_ types.NodeID,
				_ PoetAuth,
			) (*types.PoetRound, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			})
		poetProver.EXPECT().Address().AnyTimes().Return("http://localhost:9999")
		postService := NewMockpostService(ctrl)

		nb, err := NewNIPostBuilder(
			poetDb,
			postService,
			t.TempDir(),
			zaptest.NewLogger(t),
			sig,
			poetCfg,
			mclock,
			WithPoetClients(poetProver),
		)
		require.NoError(t, err)

		certifier := NewMockcertifierService(ctrl)
		certifier.EXPECT().GetCertificate("http://localhost:9999").Return(cert)

		nipst, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
		require.ErrorIs(t, err, ErrPoetServiceUnstable)
		require.Nil(t, nipst)
	})
	t.Run("GetProof fails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		poetDb := NewMockpoetDbAPI(ctrl)
		mclock := defaultLayerClockMock(ctrl)
		poetProver := defaultPoetServiceMock(ctrl, []byte("poet"), "http://localhost:9999")
		poetProver.EXPECT().Proof(gomock.Any(), "").Return(nil, nil, errors.New("failed"))
		postService := NewMockpostService(ctrl)

		nb, err := NewNIPostBuilder(
			poetDb,
			postService,
			t.TempDir(),
			zaptest.NewLogger(t),
			sig,
			poetCfg,
			mclock,
			WithPoetClients(poetProver),
		)
		require.NoError(t, err)

		certifier := NewMockcertifierService(ctrl)
		certifier.EXPECT().GetCertificate("http://localhost:9999").Return(cert)

		nipst, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
		require.ErrorIs(t, err, ErrPoetProofNotReceived)
		require.Nil(t, nipst)
	})
	t.Run("Challenge is not included in proof members", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		poetDb := NewMockpoetDbAPI(ctrl)
		poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil)
		mclock := defaultLayerClockMock(ctrl)
		poetProver := defaultPoetServiceMock(ctrl, []byte("poet"), "http://localhost:9999")
		poetProver.EXPECT().
			Proof(gomock.Any(), "").
			Return(&types.PoetProofMessage{PoetProof: types.PoetProof{}}, []types.Member{}, nil)
		postService := NewMockpostService(ctrl)

		nb, err := NewNIPostBuilder(
			poetDb,
			postService,
			t.TempDir(),
			zaptest.NewLogger(t),
			sig,
			poetCfg,
			mclock,
			WithPoetClients(poetProver),
		)
		require.NoError(t, err)

		certifier := NewMockcertifierService(ctrl)
		certifier.EXPECT().GetCertificate("http://localhost:9999").Return(cert)

		nipst, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
		require.ErrorIs(t, err, ErrPoetProofNotReceived)
		require.Nil(t, nipst)
	})
}

func TestNIPostBuilder_RecertifyPoet(t *testing.T) {
	t.Parallel()
	sig, err := signing.NewEdSigner()
	require.NoError(t, err)
	challenge := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 1,
	}

	ctrl := gomock.NewController(t)
	poetDb := NewMockpoetDbAPI(ctrl)
	poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil)
	mclock := defaultLayerClockMock(ctrl)
	poetProver := NewMockPoetClient(ctrl)
	poetProver.EXPECT().PoetServiceID(gomock.Any()).AnyTimes().Return(types.PoetServiceID{}, nil)
	poetProver.EXPECT().Address().AnyTimes().Return("http://localhost:9999")
	postClient := NewMockPostClient(ctrl)
	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(&types.Post{}, &types.PostInfo{}, nil)
	postService := NewMockpostService(ctrl)
	postService.EXPECT().Client(sig.NodeID()).Return(postClient, nil)

	nb, err := NewNIPostBuilder(
		poetDb,
		postService,
		t.TempDir(),
		zaptest.NewLogger(t),
		sig,
		PoetConfig{PhaseShift: layerDuration},
		mclock,
		WithPoetClients(poetProver),
	)
	require.NoError(t, err)

	certifier := NewMockcertifierService(ctrl)

	invalid := PoetCert("certInvalid")
	getInvalid := certifier.EXPECT().GetCertificate("http://localhost:9999").Return(invalid)
	submitFailed := poetProver.EXPECT().
		Submit(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			sig.NodeID(),
			PoetAuth{PoetCert: invalid},
		).
		After(getInvalid.Call).
		Return(nil, ErrUnathorized)

	valid := PoetCert("certInvalid")
	recertify := certifier.EXPECT().Recertify(gomock.Any(), poetProver).After(submitFailed).Return(valid, nil)
	submitOK := poetProver.EXPECT().
		Submit(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			sig.NodeID(),
			PoetAuth{PoetCert: valid},
		).
		After(recertify).
		Return(&types.PoetRound{}, nil)

	poetProver.EXPECT().
		Proof(gomock.Any(), "").
		After(submitOK).
		Return(&types.PoetProofMessage{}, []types.Member{types.Member(challenge.Hash())}, nil)

	_, err = nb.BuildNIPost(context.Background(), &challenge, certifier)
	require.NoError(t, err)
}

// TestNIPoSTBuilder_StaleChallenge checks if
// it properly detects that the challenge is stale and the poet round has already started.
func TestNIPoSTBuilder_StaleChallenge(t *testing.T) {
	t.Parallel()

	currLayer := types.EpochID(10).FirstLayer()
	genesis := time.Now().Add(-time.Duration(currLayer) * layerDuration)

	sig, err := signing.NewEdSigner()
	require.NoError(t, err)

	// Act & Verify
	t.Run("no requests, poet round started", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		poetDb := NewMockpoetDbAPI(ctrl)
		mclock := NewMocklayerClock(ctrl)
		poetProver := NewMockPoetClient(ctrl)
		poetProver.EXPECT().Address().Return("http://localhost:9999")
		mclock.EXPECT().LayerToTime(gomock.Any()).DoAndReturn(
			func(got types.LayerID) time.Time {
				return genesis.Add(layerDuration * time.Duration(got))
			},
		).AnyTimes()
		postService := NewMockpostService(ctrl)

		nb, err := NewNIPostBuilder(
			poetDb,
			postService,
			t.TempDir(),
			zaptest.NewLogger(t),
			sig,
			PoetConfig{},
			mclock,
			WithPoetClients(poetProver),
		)
		require.NoError(t, err)

		certifier := NewMockcertifierService(ctrl)
		challenge := types.NIPostChallenge{PublishEpoch: currLayer.GetEpoch()}
		nipost, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
		require.ErrorIs(t, err, ErrATXChallengeExpired)
		require.ErrorContains(t, err, "poet round has already started")
		require.Nil(t, nipost)
	})
	t.Run("no response before deadline", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		poetDb := NewMockpoetDbAPI(ctrl)
		mclock := NewMocklayerClock(ctrl)
		poetProver := NewMockPoetClient(ctrl)
		poetProver.EXPECT().Address().Return("http://localhost:9999")
		mclock.EXPECT().LayerToTime(gomock.Any()).DoAndReturn(
			func(got types.LayerID) time.Time {
				return genesis.Add(layerDuration * time.Duration(got))
			}).AnyTimes()
		postService := NewMockpostService(ctrl)

		dir := t.TempDir()
		nb, err := NewNIPostBuilder(
			poetDb,
			postService,
			dir,
			zaptest.NewLogger(t),
			sig,
			PoetConfig{},
			mclock,
			WithPoetClients(poetProver),
		)
		require.NoError(t, err)
		challenge := types.NIPostChallenge{PublishEpoch: currLayer.GetEpoch() - 1}
		state := types.NIPostBuilderState{
			Challenge:    challenge.Hash(),
			PoetRequests: []types.PoetRequest{{}},
		}
		require.NoError(t, saveBuilderState(dir, &state))

		certifier := NewMockcertifierService(ctrl)

		nipost, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
		require.ErrorIs(t, err, ErrATXChallengeExpired)
		require.ErrorContains(t, err, "poet proof for pub epoch")
		require.Nil(t, nipost)
	})
	t.Run("too late for proof generation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		poetDb := NewMockpoetDbAPI(ctrl)
		mclock := NewMocklayerClock(ctrl)
		poetProver := NewMockPoetClient(ctrl)
		poetProver.EXPECT().Address().Return("http://localhost:9999")
		mclock.EXPECT().LayerToTime(gomock.Any()).DoAndReturn(
			func(got types.LayerID) time.Time {
				return genesis.Add(layerDuration * time.Duration(got))
			}).AnyTimes()
		postService := NewMockpostService(ctrl)

		dir := t.TempDir()
		nb, err := NewNIPostBuilder(
			poetDb,
			postService,
			dir,
			zaptest.NewLogger(t),
			sig,
			PoetConfig{},
			mclock,
			WithPoetClients(poetProver),
		)
		require.NoError(t, err)
		challenge := types.NIPostChallenge{PublishEpoch: currLayer.GetEpoch() - 1}
		state := types.NIPostBuilderState{
			Challenge:    challenge.Hash(),
			PoetRequests: []types.PoetRequest{{}},
			PoetProofRef: [32]byte{1, 2, 3},
			NIPost:       &types.NIPost{},
		}
		require.NoError(t, saveBuilderState(dir, &state))

		certifier := NewMockcertifierService(ctrl)

		nipost, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
		require.ErrorIs(t, err, ErrATXChallengeExpired)
		require.ErrorContains(t, err, "deadline to publish ATX for pub epoch")
		require.Nil(t, nipost)
	})
}

// Test if the NIPoSTBuilder continues after being interrupted just after
// a challenge has been submitted to poet.
func TestNIPoSTBuilder_Continues_After_Interrupted(t *testing.T) {
	t.Parallel()

	// Arrange
	challenge := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 1,
	}
	sig, err := signing.NewEdSigner()
	require.NoError(t, err)
	proof := &types.PoetProofMessage{
		PoetProof: types.PoetProof{
			LeafCount: 777,
		},
	}
	cert := PoetCert("cert")

	ctrl := gomock.NewController(t)
	poetDb := NewMockpoetDbAPI(ctrl)
	poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil)
	mclock := defaultLayerClockMock(ctrl)

	buildCtx, cancel := context.WithCancel(context.Background())

	poet := NewMockPoetClient(ctrl)
	poet.EXPECT().PoetServiceID(gomock.Any()).AnyTimes().Return(types.PoetServiceID{ServiceID: []byte("poet0")}, nil)
	poet.EXPECT().
		Submit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), sig.NodeID(), PoetAuth{PoetCert: cert}).
		DoAndReturn(func(
			_ context.Context,
			_ time.Time,
			_, _ []byte,
			_ types.EdSignature,
			_ types.NodeID,
			_ PoetAuth,
		) (*types.PoetRound, error) {
			cancel()
			return &types.PoetRound{}, context.Canceled
		})
	poet.EXPECT().Proof(gomock.Any(), "").Return(proof, []types.Member{types.Member(challenge.Hash())}, nil)
	poet.EXPECT().Address().AnyTimes().Return("http://localhost:9999")

	poetCfg := PoetConfig{
		PhaseShift: layerDuration * layersPerEpoch / 2,
	}

	postClient := NewMockPostClient(ctrl)
	postClient.EXPECT().Proof(gomock.Any(), gomock.Any()).Return(&types.Post{}, &types.PostInfo{}, nil)
	postService := NewMockpostService(ctrl)
	postService.EXPECT().Client(sig.NodeID()).Return(postClient, nil)

	nb, err := NewNIPostBuilder(
		poetDb,
		postService,
		t.TempDir(),
		zaptest.NewLogger(t),
		sig,
		poetCfg,
		mclock,
		WithPoetClients(poet),
	)
	require.NoError(t, err)

	certifier := NewMockcertifierService(ctrl)
	certifier.EXPECT().GetCertificate("http://localhost:9999").Return(cert)

	// Act
	nipost, err := nb.BuildNIPost(buildCtx, &challenge, certifier)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, nipost)

	// allow to submit in the second try
	poet.EXPECT().
		Submit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&types.PoetRound{}, nil)

	certifier.EXPECT().GetCertificate("http://localhost:9999").Return(cert)
	nipost, err = nb.BuildNIPost(context.Background(), &challenge, certifier)
	require.NoError(t, err)

	// Verify
	ref, _ := proof.Ref()
	require.Equal(t, ref[:], nipost.PostMetadata.Challenge)
}

func TestConstructingMerkleProof(t *testing.T) {
	challenge := types.NIPostChallenge{}
	challengeHash := challenge.Hash()

	t.Run("members empty", func(t *testing.T) {
		_, err := constructMerkleProof(challengeHash, []types.Member{})
		require.Error(t, err)
	})
	t.Run("not a member", func(t *testing.T) {
		_, err := constructMerkleProof(challengeHash, []types.Member{{}, {}})
		require.Error(t, err)
	})

	t.Run("is odd member", func(t *testing.T) {
		otherChallenge := types.NIPostChallenge{Sequence: 1}
		members := []types.Member{
			types.Member(challengeHash),
			types.Member(otherChallenge.Hash()),
		}
		proof, err := constructMerkleProof(challengeHash, members)
		require.NoError(t, err)

		root, err := calcRoot(members)
		require.NoError(t, err)

		err = validateMerkleProof(challengeHash[:], proof, root)
		require.NoError(t, err)
	})
	t.Run("is even member", func(t *testing.T) {
		otherChallenge := types.NIPostChallenge{Sequence: 1}
		members := []types.Member{
			types.Member(otherChallenge.Hash()),
			types.Member(challengeHash),
		}
		proof, err := constructMerkleProof(challengeHash, members)
		require.NoError(t, err)

		root, err := calcRoot(members)
		require.NoError(t, err)

		err = validateMerkleProof(challengeHash[:], proof, root)
		require.NoError(t, err)
	})
}

func TestNIPostBuilder_Mainnet_Poet_Workaround(t *testing.T) {
	t.Parallel()

	tt := []struct {
		name  string
		from  string
		to    string
		epoch types.EpochID
	}{
		// no mitigation needed at the moment
	}

	for _, tc := range tt {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			challenge := types.NIPostChallenge{
				PublishEpoch: tc.epoch,
			}

			ctrl := gomock.NewController(t)
			poets := make([]PoetClient, 0, 2)
			{
				poetProvider := NewMockPoetClient(ctrl)
				poetProvider.EXPECT().Address().Return(tc.from)
				poetProvider.EXPECT().
					PoetServiceID(gomock.Any()).
					Return(types.PoetServiceID{ServiceID: []byte("poet-from")}, nil).
					AnyTimes()

				// PoET succeeds to submit
				poetProvider.EXPECT().
					Submit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&types.PoetRound{}, nil)

				// proof is fetched from PoET
				poetProvider.EXPECT().Proof(gomock.Any(), "").Return(&types.PoetProofMessage{
					PoetProof: types.PoetProof{},
				}, []types.Member{types.Member(challenge.Hash())}, nil)
				poets = append(poets, poetProvider)
			}

			{
				// PoET fails submission
				poetProvider := NewMockPoetClient(ctrl)
				poetProvider.EXPECT().Address().Return(tc.to)
				poetProvider.EXPECT().
					PoetServiceID(gomock.Any()).
					Return(types.PoetServiceID{}, errors.New("failed submission"))

				// proof is still fetched from PoET
				poetProvider.EXPECT().
					PoetServiceID(gomock.Any()).
					Return(types.PoetServiceID{ServiceID: []byte("poet-to")}, nil).
					AnyTimes()
				poetProvider.EXPECT().Proof(gomock.Any(), "").Return(&types.PoetProofMessage{
					PoetProof: types.PoetProof{},
				}, []types.Member{types.Member(challenge.Hash())}, nil)

				poets = append(poets, poetProvider)
			}

			poetDb := NewMockpoetDbAPI(ctrl)
			poetDb.EXPECT().ValidateAndStore(gomock.Any(), gomock.Any()).Return(nil).Times(2)
			mclock := NewMocklayerClock(ctrl)
			genesis := time.Now().Add(-time.Duration(1) * layerDuration)
			mclock.EXPECT().LayerToTime(gomock.Any()).AnyTimes().DoAndReturn(
				func(got types.LayerID) time.Time {
					return genesis.Add(layerDuration * time.Duration(got))
				},
			)

			poetCfg := PoetConfig{
				PhaseShift: layerDuration * layersPerEpoch / 2,
			}

			sig, err := signing.NewEdSigner()
			require.NoError(t, err)

			postClient := NewMockPostClient(ctrl)
			postClient.EXPECT().Proof(gomock.Any(), gomock.Any())
			postService := NewMockpostService(ctrl)
			postService.EXPECT().Client(sig.NodeID()).Return(postClient, nil)

			nb, err := NewNIPostBuilder(
				poetDb,
				postService,
				t.TempDir(),
				zaptest.NewLogger(t),
				sig,
				poetCfg,
				mclock,
				WithPoetClients(poets...),
			)
			require.NoError(t, err)

			certifier := NewMockcertifierService(ctrl)
			certifier.EXPECT().CertifyAll(gomock.Any(), gomock.Any()).Return(nil)

			nipost, err := nb.BuildNIPost(context.Background(), &challenge, certifier)
			require.NoError(t, err)
			require.NotNil(t, nipost)
		})
	}
}

func FuzzBuilderStateConsistency(f *testing.F) {
	tester.FuzzConsistency[types.NIPostBuilderState](f)
}

func FuzzBuilderStateSafety(f *testing.F) {
	tester.FuzzSafety[types.NIPostBuilderState](f)
}

func TestRandomDurationInRange(t *testing.T) {
	t.Parallel()
	test := func(min, max time.Duration) {
		for i := 0; i < 100; i++ {
			waitTime := randomDurationInRange(min, max)
			require.LessOrEqual(t, waitTime, max)
			require.GreaterOrEqual(t, waitTime, min)
		}
	}
	t.Run("min = 0", func(t *testing.T) {
		t.Parallel()
		test(0, 7*time.Second)
	})
	t.Run("min != 0", func(t *testing.T) {
		t.Parallel()
		test(5*time.Second, 7*time.Second)
	})
}

func TestCalculatingGetProofWaitTime(t *testing.T) {
	t.Parallel()
	t.Run("past round end", func(t *testing.T) {
		t.Parallel()
		deadline := proofDeadline(time.Now().Add(-time.Hour), time.Hour*12)
		require.Less(t, time.Until(deadline), time.Duration(0))
	})
	t.Run("before round end", func(t *testing.T) {
		t.Parallel()
		cycleGap := 12 * time.Hour
		deadline := proofDeadline(time.Now().Add(time.Hour), cycleGap)

		require.Greater(t, time.Until(deadline), time.Hour+time.Duration(float64(cycleGap)*minPoetGetProofJitter/100))
		require.LessOrEqual(
			t,
			time.Until(deadline),
			time.Hour+time.Duration(float64(cycleGap)*maxPoetGetProofJitter/100),
		)
	})
}

func TestNIPostBuilder_Close(t *testing.T) {
	ctrl := gomock.NewController(t)

	mclock := NewMocklayerClock(ctrl)
	mclock.EXPECT().LayerToTime(gomock.Any()).AnyTimes().DoAndReturn(
		func(got types.LayerID) time.Time {
			// time.Now() ~= currentLayer
			genesis := time.Now().Add(-time.Duration(postGenesisEpoch.FirstLayer()) * layerDuration)
			return genesis.Add(layerDuration * time.Duration(got))
		},
	)

	sig, err := signing.NewEdSigner()
	require.NoError(t, err)

	nb, err := NewNIPostBuilder(
		NewMockpoetDbAPI(ctrl),
		NewMockpostService(ctrl),
		t.TempDir(),
		zaptest.NewLogger(t),
		sig,
		PoetConfig{},
		mclock,
		WithPoetClients(defaultPoetServiceMock(ctrl, []byte("poet"), "http://localhost:9999")),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	challenge := types.NIPostChallenge{
		PublishEpoch: postGenesisEpoch + 2,
	}

	nipost, err := nb.BuildNIPost(ctx, &challenge, NewMockcertifierService(ctrl))
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, nipost)
}
