package util

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

func RetryWithBackOff(ctx context.Context, callback func() error, retryCount int) error {
	sleep := 10 * time.Second
	var err error
	for i := 0; i < retryCount; i++ {
		if i > 0 {
			logrus.Infof("retrying after sleeping %v, txId=%v", sleep, ctx.Value("txId"))
			time.Sleep(sleep)
			sleep *= 2
		}
		err = callback()
		if err == nil {
			break
		}
		logrus.Infof("retrying with error %v, txId=%v", err, ctx.Value("txId"))
	}
	return err
}
