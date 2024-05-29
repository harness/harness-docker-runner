package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/harness/harness-docker-runner/executor"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/logger"
)

// HandleDebug
func HandleDebug() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var s api.DebugRequest
		err := json.NewDecoder(r.Body).Decode(&s)
		if err != nil {
			WriteBadRequest(w, err)
			return
		}

		if s.StepID == "" {
			logger.FromRequest(r).Errorln("step id is not specified")
			WriteError(w, errors.New("step id is not specified"))
			return
		}

		ex := executor.GetExecutor()
		d, err := ex.Get(s.StageRuntimeID)

		if err != nil {
			logger.FromRequest(r).WithError(err).WithField("id", s.StageRuntimeID).Errorln("stage mapping does not exist")
			WriteNotFound(w, err)
			return
		}

		if d != nil {
			d.Engine.Debug(r.Context(), s.StepID, s.Command, s.Last)
		}
	}
}
