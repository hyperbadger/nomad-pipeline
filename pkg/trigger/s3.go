package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/s3blob"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/awssnssqs"
	"gopkg.in/yaml.v3"
)

// generated using https://mholt.github.io/json-to-go/ using example from
// https://docs.amazonaws.cn/en_us/AmazonS3/latest/userguide/notification-content-structure.html
type S3TriggerEvent struct {
	// this section was seperately added to handle s3:TestEvent messages
	// all of these fields will not be set for all other standard s3 events
	// see https://www.mikulskibartosz.name/what-is-s3-test-event/ for more info
	Service   string    `json:"Service"`
	Event     string    `json:"Event"`
	Time      time.Time `json:"Time"`
	Bucket    string    `json:"Bucket"`
	RequestID string    `json:"RequestId"`
	HostID    string    `json:"HostId"`

	Records []struct {
		EventVersion string    `json:"eventVersion"`
		EventSource  string    `json:"eventSource"`
		AwsRegion    string    `json:"awsRegion"`
		EventTime    time.Time `json:"eventTime"`
		EventName    string    `json:"eventName"`
		UserIdentity struct {
			PrincipalID string `json:"principalId"`
		} `json:"userIdentity"`
		RequestParameters struct {
			SourceIPAddress string `json:"sourceIPAddress"`
		} `json:"requestParameters"`
		ResponseElements struct {
			XAmzRequestID string `json:"x-amz-request-id"`
			XAmzID2       string `json:"x-amz-id-2"`
		} `json:"responseElements"`
		S3 struct {
			S3SchemaVersion string `json:"s3SchemaVersion"`
			ConfigurationID string `json:"configurationId"`
			Bucket          struct {
				Name          string `json:"name"`
				OwnerIdentity struct {
					PrincipalID string `json:"principalId"`
				} `json:"ownerIdentity"`
				Arn string `json:"arn"`
			} `json:"bucket"`
			Object struct {
				Key       string `json:"key"`
				Size      int    `json:"size"`
				ETag      string `json:"eTag"`
				VersionID string `json:"versionId"`
				Sequencer string `json:"sequencer"`
			} `json:"object"`
		} `json:"s3"`
		GlacierEventData struct {
			RestoreEventData struct {
				LifecycleRestorationExpiryTime time.Time `json:"lifecycleRestorationExpiryTime"`
				LifecycleRestoreStorageClass   string    `json:"lifecycleRestoreStorageClass"`
			} `json:"restoreEventData"`
		} `json:"glacierEventData"`
	} `json:"Records"`
}

type S3Trigger struct {
	SQSUrl       string `yaml:"sqs_url"`
	MetaKey      string `yaml:"meta_key"`
	BucketUrl    string `yaml:"bucket_url"`
	SettingsExt  string `yaml:"settings_ext"`
	ObjectFilter string `yaml:"object_filter"`
	AckNoMatch   bool   `yaml:"ack_no_match"`

	Sub    *pubsub.Subscription
	Bucket *blob.Bucket
	regex  *regexp.Regexp
	logger *zap.SugaredLogger
}

func (s3t *S3Trigger) Init(ctx context.Context, logger *zap.SugaredLogger) error {
	s3t.logger = logger
	sub, err := pubsub.OpenSubscription(ctx, s3t.SQSUrl)
	if err != nil {
		return fmt.Errorf("error subscribing to sqs: %w", err)
	}
	s3t.Sub = sub
	if s3t.MetaKey == "" {
		return errors.New("")
	}
	if len(s3t.BucketUrl) > 0 {
		bucket, err := blob.OpenBucket(ctx, s3t.BucketUrl)
		if err != nil {
			return fmt.Errorf("error opening bucket: %w", err)
		}
		s3t.Bucket = bucket
	} else {
		s3t.logger.Warn("bucket_url not set, per object settings won't be used")
	}
	if s3t.ObjectFilter == "" {
		s3t.ObjectFilter = ".*"
	}
	r, err := regexp.Compile(s3t.ObjectFilter)
	if err != nil {
		return fmt.Errorf("error compiling object path regex: %w", err)
	}
	s3t.regex = r
	if s3t.SettingsExt == "" {
		s3t.SettingsExt = ".yaml"
	}
	return nil
}

func (s3t *S3Trigger) Key() string {
	return s3t.ObjectFilter + "-" + s3t.SQSUrl
}

func (s3t *S3Trigger) Run(ctx context.Context, f func(*Dispatch) error, errCh chan<- error) {
	var wg sync.WaitGroup

	for {
		if err := ctx.Err(); err != nil {
			errCh <- err
			break
		}

		msg, err := s3t.Sub.Receive(ctx)
		if err != nil {
			errCh <- err
			break
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			var event S3TriggerEvent
			err := json.Unmarshal(msg.Body, &event)
			if err != nil {
				errCh <- fmt.Errorf("error unmarshaling s3 event: %w", err)
				return
			}

			// see https://www.mikulskibartosz.name/what-is-s3-test-event/ for more info
			// we should just ack the TestEvent and not continue with processing
			if event.Event == "s3:TestEvent" {
				msg.Ack()
				return
			}

			// according to https://stackoverflow.com/a/53845382 we should only recieve a single
			// record in one event
			if r := len(event.Records); r != 1 {
				errCh <- fmt.Errorf("expecting a single record in s3 event, got %v events", r)
				return
			}

			oPath := event.Records[0].S3.Object.Key
			match := s3t.regex.Match([]byte(oPath))
			if !match {
				if s3t.AckNoMatch {
					msg.Ack()
				}
				return
			}

			d := NewDispatch()

			dir := path.Dir(oPath)
			base := path.Base(oPath)
			ext := path.Ext(base)
			fName := strings.TrimSuffix(base, ext)
			sPath := path.Join(dir, fName+s3t.SettingsExt)

			if s3t.Bucket != nil {
				exists, err := s3t.Bucket.Exists(ctx, sPath)
				if err != nil {
					errCh <- fmt.Errorf("error checking if settings file exists, continuing without settings: %w", err)
				}
				sBytes := make([]byte, 0)
				if exists {
					sBytes, err = s3t.Bucket.ReadAll(ctx, sPath)
					if err != nil {
						errCh <- fmt.Errorf("settings file exists but error when reading, continuing without settings: %w", err)
					}
				}
				if len(sBytes) > 0 {
					err = yaml.Unmarshal(sBytes, d)
					if err != nil {
						errCh <- fmt.Errorf("error unmarshalling settings file, continuing without settings: %w", err)
					}
				}
			}

			d.Meta[s3t.MetaKey] = oPath

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

func (s3t *S3Trigger) Shutdown(ctx context.Context) error {
	return s3t.Sub.Shutdown(ctx)
}
