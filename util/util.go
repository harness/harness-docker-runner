package util

import (
	"context"
	"fmt"
	"sync"

	"github.com/harness/harness-docker-runner/config"
	"github.com/wings-software/dlite/client"
	"github.com/wings-software/dlite/delegate"
)

var onlyOnce sync.Once

func RegisterDelagetCapacity(Id string) {
	onlyOnce.Do(func() {
		c := config.GetConfig()
		cl := delegate.New(c.DelegateCapacity.ManagerEndpoint, c.DelegateCapacity.AccountID, c.DelegateCapacity.Secret, false)
		err := cl.RegisterCapacity(context.Background(), Id, &client.DelegateCapacity{MaxBuilds: c.DelegateCapacity.MaxBuilds})
		fmt.Println(err)
	})
}
