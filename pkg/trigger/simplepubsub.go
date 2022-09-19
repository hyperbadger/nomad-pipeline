package trigger

import (
	"context"
	"fmt"

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

}

func (spst *SimplePubSubTrigger) Shutdown(ctx context.Context) error {
	return nil
}
