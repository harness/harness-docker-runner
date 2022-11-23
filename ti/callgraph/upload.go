// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package callgraph

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/internal/filesystem"
	"github.com/harness/harness-docker-runner/pipeline"
	"github.com/harness/harness-docker-runner/ti/avro"
	"github.com/harness/harness-docker-runner/ti/client"
	"github.com/mattn/go-zglob"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	cgSchemaType = "callgraph"
	cgDir        = "%s/ti/callgraph/" // path where callgraph files will be generated
)

// Upload method uploads the callgraph.
func Upload(ctx context.Context, stepID string, timeMs int64, out io.Writer, ticlient client.Client) error {
	log := logrus.New()
	log.Out = out

	// TODO: Pass in the config here and use that, right now it will be empty
	// cfg := pipeline.GetState().GetTIConfig()
	cfg := &api.TIConfig{}
	if cfg == nil || cfg.URL == "" {
		return fmt.Errorf("TI config is not provided in setup")
	}

	isManual := cfg.SourceBranch == "" || cfg.TargetBranch == "" || cfg.Sha == ""
	source := cfg.SourceBranch
	if source == "" && !isManual {
		return fmt.Errorf("source branch is not set")
	} else if isManual {
		source = cfg.CommitBranch
		if source == "" {
			return fmt.Errorf("commit branch is not set")
		}
	}
	target := cfg.TargetBranch
	if target == "" && !isManual {
		return fmt.Errorf("target branch is not set")
	} else if isManual {
		target = cfg.CommitBranch
		if target == "" {
			return fmt.Errorf("commit branch is not set")
		}
	}

	encCg, err := encodeCg(fmt.Sprintf(cgDir, pipeline.SharedVolPath), log)
	if err != nil {
		return errors.Wrap(err, "failed to get avro encoded callgraph")
	}

	return ticlient.UploadCg(ctx, stepID, source, target, timeMs, encCg)
}

// encodeCg reads all files of specified format from datadir folder and returns byte array of avro encoded format
func encodeCg(dataDir string, log *logrus.Logger) (
	[]byte, error) {
	var parser Parser
	fs := filesystem.New()

	if dataDir == "" {
		return nil, fmt.Errorf("dataDir not present in request")
	}
	cgFiles, visFiles, err := getCgFiles(dataDir, "json", "csv", log)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch files inside the directory")
	}
	parser = NewCallGraphParser(log, fs)
	cg, err := parser.Parse(cgFiles, visFiles)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse visgraph")
	}
	log.Infoln(fmt.Sprintf("size of nodes: %d, testReln: %d, visReln %d", len(cg.Nodes), len(cg.TestRelations), len(cg.VisRelations)))

	cgMap := cg.ToStringMap()
	cgSer, err := avro.NewCgphSerialzer(cgSchemaType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create serializer")
	}
	encCg, err := cgSer.Serialize(cgMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode callgraph")
	}
	return encCg, nil
}

// get list of all file paths matching a provided regex
func getFiles(path string) ([]string, error) {
	matches, err := zglob.Glob(path)
	if err != nil {
		return []string{}, err
	}
	return matches, err
}

// getCgFiles return list of cg files in given directory
func getCgFiles(dir, ext1, ext2 string, log *logrus.Logger) ([]string, []string, error) { // nolint:gocritic,unparam
	cgFiles, err1 := getFiles(filepath.Join(dir, "**/*."+ext1))
	visFiles, err2 := getFiles(filepath.Join(dir, "**/*."+ext2))
	log.Infoln("cg files: ", cgFiles)
	log.Infoln("vis files: ", visFiles)

	if err1 != nil || err2 != nil {
		log.Errorln(fmt.Sprintf("error in getting files list in dir %s", dir), err1, err2)
	}
	return cgFiles, visFiles, nil
}
