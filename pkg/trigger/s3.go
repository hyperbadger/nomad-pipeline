package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"gocloud.dev/blob"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/awssnssqs"
	"gopkg.in/yaml.v3"
)

// generated using https://mholt.github.io/json-to-go/ using example from
// https://docs.amazonaws.cn/en_us/AmazonS3/latest/userguide/notification-content-structure.html
type S3TriggerEvent struct {
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
				Size      string `json:"size"`
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
	ObjectPath  string `yaml:"object_path"`
	MetaKey     string `yaml:"meta_key"`
	SQSUrl      string `yaml:"sqs_url"`
	BucketUrl   string `yaml:"bucket_url"`
	SettingsExt string `yaml:"settings_ext"`
	Sub         *pubsub.Subscription
	Bucket      *blob.Bucket
	regex       *regexp.Regexp
}

func (s3t *S3Trigger) Init(ctx context.Context) error {
	sub, err := pubsub.OpenSubscription(ctx, s3t.SQSUrl)
	if err != nil {
		return fmt.Errorf("error subscribing to sqs: %w", err)
	}
	s3t.Sub = sub
	bucket, err := blob.OpenBucket(ctx, s3t.BucketUrl)
	if err != nil {
		return fmt.Errorf("error opening bucket: %w", err)
	}
	s3t.Bucket = bucket
	r, err := regexp.Compile(s3t.ObjectPath)
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
	return s3t.ObjectPath + "-" + s3t.SQSUrl
}

func (s3t *S3Trigger) Run(ctx context.Context, f func(Dispatch) error, errCh chan<- error) {
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

			oPath := event.Records[0].S3.Object.Key
			match := s3t.regex.Match([]byte(oPath))
			if !match {
				return
			}

			d := Dispatch{}

			dir := path.Dir(oPath)
			base := path.Base(oPath)
			ext := path.Ext(base)
			fName := strings.TrimSuffix(base, ext)
			sPath := path.Join(dir, fName+s3t.SettingsExt)

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
				err = yaml.Unmarshal(sBytes, &d)
				if err != nil {
					errCh <- fmt.Errorf("error unmarshalling settings file, continuing without settings: %w", err)
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
