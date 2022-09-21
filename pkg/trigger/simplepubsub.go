package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"gocloud.dev/pubsub"
)

type SimplePubSubTrigger struct {
	PubSubUrl string `yaml:"pubsub_url"`
	Sub       *pubsub.Subscription
}

func (spst *SimplePubSubTrigger) Init(ctx context.Context) error {
	sub, err := pubsub.OpenSubscription(ctx, spst.PubSubUrl)
	if err != nil {
		return fmt.Errorf("error subscribing to sqs: %w", err)
	}
	spst.Sub = sub
	return nil
}

func (spst *SimplePubSubTrigger) Key() string {
	return spst.PubSubUrl
}

func (spst *SimplePubSubTrigger) Run(ctx context.Context, f func(Dispatch) error, errCh chan<- error) {
	var wg sync.WaitGroup

	for {
		if err := ctx.Err(); err != nil {
			errCh <- err
			break
		}

		msg, err := spst.Sub.Receive(ctx)
		if err != nil {
			errCh <- err
			break
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			d := Dispatch{}
			err := json.Unmarshal(msg.Body, &d)
			if err != nil {
				errCh <- fmt.Errorf("error unmarshaling message: %w", err)
				return
			}

			err = f(d)
			if err != nil {
				errCh <- fmt.Errorf("trigger callback failed: %w", err)
				return
			}
			msg.Ack()
		}()
	}

	wg.Wait()
}

func (spst *SimplePubSubTrigger) Shutdown(ctx context.Context) error {
	return spst.Sub.Shutdown(ctx)
}
