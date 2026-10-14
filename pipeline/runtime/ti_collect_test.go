package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/harness/harness-docker-runner/api"
	leapi "github.com/harness/lite-engine/api"
	"github.com/harness/lite-engine/ti/callgraph"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/harness/ti-client/types"
)

func TestCollectTestReportsAndCallgraphPassesParsedTests(t *testing.T) {
	origParse, origUpload := parseAndUploadTestsFn, uploadCallgraphFn
	defer func() { parseAndUploadTestsFn, uploadCallgraphFn = origParse, origUpload }()

	want := []*types.TestCase{{Name: "A", ClassName: "pkg.C", FileName: "C.cs"}, {Name: "B"}}
	parseAndUploadTestsFn = func(_ context.Context, _ leapi.TestReport, _ string, _ string, _ *logrus.Logger, _ time.Time, _ *tiCfg.Cfg, _ *types.TestIntelligenceMetaData, _ map[string]string) ([]*types.TestCase, error) {
		return want, nil
	}
	var got []*types.TestCase
	var gotDir string
	uploadCallgraphFn = func(_ context.Context, _ string, _ int64, _ *logrus.Logger, _ time.Time, _ *tiCfg.Cfg, dir string, _ string, tests []*types.TestCase, _ bool, _ *leapi.StartStepRequest) (*callgraph.Callgraph, error) {
		got, gotDir = tests, dir
		return nil, nil
	}

	r := &api.StartStepRequest{}
	collectTestReportsAndCallgraph(context.Background(), logrus.New(), r, "step", time.Now(), nil, &types.TelemetryData{}, "%s/cg")

	require.Equal(t, want, got)
	require.Equal(t, "%s/cg", gotDir)
}
